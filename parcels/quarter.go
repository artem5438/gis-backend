package parcels

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"gis-backend/auth"
	"gis-backend/db"
	"gis-backend/districts"
	"gis-backend/models"

	"github.com/go-chi/chi/v5"
)

const (
	quarterMin      = 5
	quarterMax      = 40
	neighborRadiusM = 80
	quarterUseLimit = 8
)

var errParcelMissing = errors.New("parcel missing")

type quarterSlice struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type quarterReport struct {
	Ready         bool           `json:"ready"`
	Reason        string         `json:"reason,omitempty"`
	Basis         string         `json:"basis,omitempty"`
	Note          string         `json:"note,omitempty"`
	Quarter       string         `json:"quarter,omitempty"`
	NeighborCount int            `json:"neighbor_count"`
	AreaHa        *float64       `json:"area_ha"`
	AreaMedianHa  *float64       `json:"area_median_ha"`
	CostIndex     *float64       `json:"cost_index"`
	CostMedian    *float64       `json:"cost_median"`
	SameCategory  int            `json:"same_category"`
	SameUse       int            `json:"same_use"`
	Score         *float64       `json:"score"`
	ScoreMedian   *float64       `json:"score_median"`
	Categories    []quarterSlice `json:"categories"`
	Uses          []quarterSlice `json:"uses"`
}

type quarterRow struct {
	ID           uint
	Quarter      string
	LandCategory string
	PermittedUse string
	District     string
	Hectares     *float64
	CostIndex    *float64
	Lng          *float64
	Lat          *float64
}

type quarterPlace struct {
	Category string
	Use      string
	Hectares float64
	HasArea  bool
	Cost     float64
	HasCost  bool
	Score    float64
	HasScore bool
}

type quarterInput struct {
	Quarter      string
	QuarterCount int
	HasBoundary  bool
	Basis        string
	Subject      quarterPlace
	Neighbors    []quarterPlace
}

// Quarter compares one saved plot with the parcels around it.
// The group is the cadastral quarter, or parcels within 80 metres when that quarter is too small or too large.
// @Summary      Квартал участка
// @Tags         участки
// @Produce      json
// @Param        id   path      int  true  "Идентификатор участка"
// @Success      200  {object}  quarterReport
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Router       /api/parcels/{id}/quarter [get]
func Quarter(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.RequireUser(w, r)
	if !ok {
		return
	}
	if user.VerificationStatus != models.StatusVerified {
		writeError(w, http.StatusForbidden, "кадастровая карта доступна после проверки")
		return
	}
	parsed, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil || parsed == 0 {
		writeError(w, http.StatusNotFound, "участок не найден")
		return
	}
	report, err := quarterFor(uint(parsed))
	if errors.Is(err, errParcelMissing) {
		writeError(w, http.StatusNotFound, "участок не найден")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось собрать квартал")
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func quarterFor(id uint) (quarterReport, error) {
	subject, err := readQuarterSubject(id)
	if err != nil {
		return quarterReport{}, err
	}
	if subject.ID == 0 {
		return quarterReport{}, errParcelMissing
	}

	quarter := strings.TrimSpace(subject.Quarter)
	quarterCount := 0
	if quarter != "" {
		quarterCount, err = countQuarterNeighbors(id, quarter)
		if err != nil {
			return quarterReport{}, err
		}
	}

	hasBoundary := subject.Lng != nil && subject.Lat != nil
	basis, useRadius := neighborBasis(quarter, quarterCount)
	var rows []quarterRow
	if !useRadius {
		rows, err = readQuarterNeighbors(`
			WHERE id <> ? AND cadastral_quarter = ?
		`, id, quarter)
		if err != nil {
			return quarterReport{}, err
		}
	}
	if useRadius {
		if !hasBoundary {
			rows = nil
		} else {
			// The degree box is only wide enough to cover 80 metres at this latitude.
			// The gist index can use it; the metre check then keeps the real neighbours.
			rows, err = readQuarterNeighbors(`
				WHERE id <> ?
					AND boundary IS NOT NULL
					AND boundary && ST_Expand((SELECT boundary FROM parcels WHERE id = ?), 0.0022)
					AND ST_DWithin(
						boundary::geography,
						(SELECT boundary::geography FROM parcels WHERE id = ? AND boundary IS NOT NULL),
						?
					)
			`, id, id, id, neighborRadiusM)
			if err != nil {
				return quarterReport{}, err
			}
		}
	}

	weightLocation, weightArea := scoreWeights()
	cache := map[string][]peerRow{}
	subjectPlace, err := placeWithScore(subject, cache, weightLocation, weightArea)
	if err != nil {
		return quarterReport{}, err
	}
	neighbors := make([]quarterPlace, 0, len(rows))
	for _, row := range rows {
		place, err := placeWithScore(row, cache, weightLocation, weightArea)
		if err != nil {
			return quarterReport{}, err
		}
		neighbors = append(neighbors, place)
	}
	return assembleQuarter(quarterInput{
		Quarter:      quarter,
		QuarterCount: quarterCount,
		HasBoundary:  hasBoundary,
		Basis:        basis,
		Subject:      subjectPlace,
		Neighbors:    neighbors,
	}), nil
}

func neighborBasis(quarter string, quarterCount int) (string, bool) {
	if quarter != "" && quarterCount >= quarterMin && quarterCount <= quarterMax {
		return "quarter", false
	}
	return "radius", true
}

func assembleQuarter(in quarterInput) quarterReport {
	if in.Basis == "radius" && !in.HasBoundary {
		return emptyQuarter("Сравнить не с чем: для поиска рядом нужна граница участка.")
	}
	if len(in.Neighbors) < quarterMin {
		return emptyQuarter("Сравнить не с чем: рядом меньше пяти участков.")
	}

	subjectCategory := CategoryTitle(in.Subject.Category)
	subjectUse := useLabel(in.Subject.Use)
	categories := map[string]int{}
	uses := make([]string, 0, len(in.Neighbors))
	areas := make([]float64, 0, len(in.Neighbors))
	costs := make([]float64, 0, len(in.Neighbors))
	scores := make([]float64, 0, len(in.Neighbors))
	sameCategory := 0
	sameUse := 0
	for _, neighbor := range in.Neighbors {
		title := CategoryTitle(neighbor.Category)
		categories[title]++
		uses = append(uses, useLabel(neighbor.Use))
		if title == subjectCategory {
			sameCategory++
		}
		if useLabel(neighbor.Use) == subjectUse {
			sameUse++
		}
		if neighbor.HasArea {
			areas = append(areas, neighbor.Hectares)
		}
		if neighbor.HasCost {
			costs = append(costs, neighbor.Cost)
		}
		if neighbor.HasScore {
			scores = append(scores, neighbor.Score)
		}
	}

	report := quarterReport{
		Ready:         true,
		Basis:         in.Basis,
		Note:          quarterNote(in.Quarter, in.QuarterCount, len(in.Neighbors), in.Basis),
		Quarter:       in.Quarter,
		NeighborCount: len(in.Neighbors),
		SameCategory:  sameCategory,
		SameUse:       sameUse,
		Categories:    categorySlices(categories),
		Uses:          useSlices(uses),
	}
	if in.Subject.HasArea {
		report.AreaHa = floatPtr(in.Subject.Hectares)
	}
	report.AreaMedianHa = medianPtr(areas)
	if in.Subject.HasCost {
		report.CostIndex = floatPtr(in.Subject.Cost)
	}
	report.CostMedian = medianPtr(costs)
	if in.Subject.HasScore {
		report.Score = floatPtr(in.Subject.Score)
	}
	report.ScoreMedian = medianPtr(scores)
	return report
}

func quarterNote(quarter string, quarterCount, shown int, basis string) string {
	if basis == "quarter" {
		return fmt.Sprintf("Кадастровый квартал %s. Соседей %d.", quarter, shown)
	}
	if quarter == "" {
		return fmt.Sprintf("Кадастровый квартал не указан. Сравнение по участкам в радиусе %d м: %d.", neighborRadiusM, shown)
	}
	if quarterCount > quarterMax {
		return fmt.Sprintf("В кадастровом квартале %s соседей %d, для двора это много. Сравнение по участкам в радиусе %d м: %d.", quarter, quarterCount, neighborRadiusM, shown)
	}
	return fmt.Sprintf("В кадастровом квартале %s соседей %d. Сравнение по участкам в радиусе %d м: %d.", quarter, quarterCount, neighborRadiusM, shown)
}

func emptyQuarter(reason string) quarterReport {
	return quarterReport{
		Ready:      false,
		Reason:     reason,
		Categories: []quarterSlice{},
		Uses:       []quarterSlice{},
	}
}

func categorySlices(counts map[string]int) []quarterSlice {
	slices := make([]quarterSlice, 0, len(counts))
	for _, title := range LegendTitles {
		if counts[title] == 0 {
			continue
		}
		slices = append(slices, quarterSlice{Name: title, Count: counts[title]})
	}
	return slices
}

func useSlices(labels []string) []quarterSlice {
	counts := map[string]int{}
	for _, label := range labels {
		counts[label]++
	}
	slices := make([]quarterSlice, 0, len(counts))
	for name, count := range counts {
		slices = append(slices, quarterSlice{Name: name, Count: count})
	}
	sort.Slice(slices, func(i, j int) bool {
		if slices[i].Count == slices[j].Count {
			return slices[i].Name < slices[j].Name
		}
		return slices[i].Count > slices[j].Count
	})
	if len(slices) <= quarterUseLimit {
		return slices
	}
	rest := 0
	for _, item := range slices[quarterUseLimit-1:] {
		rest += item.Count
	}
	return append(slices[:quarterUseLimit-1], quarterSlice{Name: "Остальные", Count: rest})
}

func useLabel(value string) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return "не указано"
	}
	return text
}

func medianPtr(sample []float64) *float64 {
	if len(sample) == 0 {
		return nil
	}
	return floatPtr(medianOf(sample))
}

func floatPtr(value float64) *float64 {
	return &value
}

func countQuarterNeighbors(id uint, quarter string) (int, error) {
	var count int
	err := db.DB.Raw(`
		SELECT count(*) FROM parcels
		WHERE id <> ? AND cadastral_quarter = ?
	`, id, quarter).Scan(&count).Error
	return count, err
}

func readQuarterSubject(id uint) (quarterRow, error) {
	var row quarterRow
	err := db.DB.Raw(`
		SELECT id, cadastral_quarter AS quarter, land_category, permitted_use, district,
			CASE
				WHEN area_ha > 0 THEN area_ha
				WHEN area_m2 > 0 THEN area_m2 / 10000.0
			END::float8 AS hectares,
			cost_index::float8 AS cost_index,
			CASE WHEN boundary IS NULL THEN NULL ELSE ST_X(ST_Centroid(boundary)) END AS lng,
			CASE WHEN boundary IS NULL THEN NULL ELSE ST_Y(ST_Centroid(boundary)) END AS lat
		FROM parcels
		WHERE id = ?
	`, id).Scan(&row).Error
	return row, err
}

func readQuarterNeighbors(where string, args ...any) ([]quarterRow, error) {
	var rows []quarterRow
	err := db.DB.Raw(`
		SELECT id, land_category, permitted_use, district,
			CASE
				WHEN area_ha > 0 THEN area_ha
				WHEN area_m2 > 0 THEN area_m2 / 10000.0
			END::float8 AS hectares,
			cost_index::float8 AS cost_index,
			CASE WHEN boundary IS NULL THEN NULL ELSE ST_X(ST_Centroid(boundary)) END AS lng,
			CASE WHEN boundary IS NULL THEN NULL ELSE ST_Y(ST_Centroid(boundary)) END AS lat
		FROM parcels
	`+where, args...).Scan(&rows).Error
	return rows, err
}

func placeWithScore(row quarterRow, cache map[string][]peerRow, weightLocation, weightArea float64) (quarterPlace, error) {
	place := quarterPlace{Category: row.LandCategory, Use: row.PermittedUse}
	if row.Hectares != nil && *row.Hectares > 0 {
		place.Hectares = *row.Hectares
		place.HasArea = true
	}
	if row.CostIndex != nil && *row.CostIndex > 0 {
		place.Cost = *row.CostIndex
		place.HasCost = true
	}
	score, ok, err := scoreQuarterRow(row, cache, weightLocation, weightArea)
	if err != nil {
		return quarterPlace{}, err
	}
	if ok {
		place.Score = score
		place.HasScore = true
	}
	return place, nil
}

func scoreQuarterRow(row quarterRow, cache map[string][]peerRow, weightLocation, weightArea float64) (float64, bool, error) {
	if row.Lng == nil || row.Lat == nil || row.Hectares == nil || *row.Hectares <= 0 {
		return 0, false, nil
	}
	districtName := row.District
	contour, known := districts.Named(districtName)
	if !known {
		districtName = districts.Assign(*row.Lng, *row.Lat, nil)
		contour, known = districts.Named(districtName)
	}
	if !known {
		return 0, false, nil
	}
	peers, err := districtPeers(cache, districtName)
	if err != nil {
		return 0, false, err
	}
	category := CategoryTitle(row.LandCategory)
	sample := make([]float64, 0, len(peers))
	included := false
	for _, peer := range peers {
		if CategoryTitle(peer.LandCategory) != category || peer.Hectares <= 0 {
			continue
		}
		sample = append(sample, peer.Hectares)
		if peer.ID == row.ID {
			included = true
		}
	}
	if !included {
		sample = append(sample, *row.Hectares)
	}
	centerLng, centerLat := contour.Center()
	distance := metersBetween(*row.Lat, *row.Lng, centerLat, centerLng)
	farthest := 0.0
	for _, corner := range contour.Corners() {
		if len(corner) < 2 {
			continue
		}
		edge := metersBetween(centerLat, centerLng, corner[1], corner[0])
		if edge > farthest {
			farthest = edge
		}
	}
	location := locationFactor(distance, farthest)
	areaPart, _ := areaFactor(*row.Hectares, sample)
	return 100*weightLocation*location + 100*weightArea*areaPart, true, nil
}

func districtPeers(cache map[string][]peerRow, districtName string) ([]peerRow, error) {
	if rows, ok := cache[districtName]; ok {
		return rows, nil
	}
	var rows []peerRow
	err := db.DB.Raw(`
		SELECT id, land_category,
			CASE
				WHEN area_ha > 0 THEN area_ha
				WHEN area_m2 > 0 THEN area_m2 / 10000.0
			END::float8 AS hectares
		FROM parcels
		WHERE district = ?
			AND (area_ha > 0 OR area_m2 > 0)
	`, districtName).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []peerRow{}
	}
	cache[districtName] = rows
	return rows, nil
}
