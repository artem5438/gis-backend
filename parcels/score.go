package parcels

import (
	"log"
	"math"
	"sort"

	"gis-backend/db"
	"gis-backend/districts"
	"gis-backend/models"
)

type parcelScore struct {
	Ready          bool    `json:"ready"`
	Reason         string  `json:"reason,omitempty"`
	Category       string  `json:"category,omitempty"`
	District       string  `json:"district,omitempty"`
	Total          float64 `json:"total"`
	LocationPoints float64 `json:"location_points"`
	LocationMax    float64 `json:"location_max"`
	AreaPoints     float64 `json:"area_points"`
	AreaMax        float64 `json:"area_max"`
	DistanceM      float64 `json:"distance_m"`
	AreaHa         float64 `json:"area_ha"`
	MedianHa       float64 `json:"median_ha"`
}

type scoreRow struct {
	ID           uint
	District     string
	LandCategory string
	AreaHa       *float64
	AreaM2       *float64
	Lng          *float64
	Lat          *float64
}

type peerRow struct {
	ID           uint
	LandCategory string
	Hectares     float64
}

func attachScore(card *parcelCard) {
	scored, err := scoreParcel(card.ID)
	if err != nil {
		log.Printf("score: %v", err)
		card.Score = &parcelScore{Ready: false, Reason: "Оценка не собралась."}
		return
	}
	card.Score = &scored
}

// scoreParcel builds the investment score from the saved plot.
// The number is not stored: the next card reads the current catalogue.
func scoreParcel(id uint) (parcelScore, error) {
	var row scoreRow
	err := db.DB.Raw(`
		SELECT id, district, land_category,
			area_ha::float8 AS area_ha, area_m2::float8 AS area_m2,
			CASE WHEN boundary IS NULL THEN NULL ELSE ST_X(ST_Centroid(boundary)) END AS lng,
			CASE WHEN boundary IS NULL THEN NULL ELSE ST_Y(ST_Centroid(boundary)) END AS lat
		FROM parcels
		WHERE id = ?
	`, id).Scan(&row).Error
	if err != nil {
		return parcelScore{}, err
	}
	if row.ID == 0 || row.Lng == nil || row.Lat == nil {
		return parcelScore{Ready: false, Reason: "Оценка не собралась: у участка нет границы."}, nil
	}

	area, ok := hectaresOf(row.AreaHa, row.AreaM2)
	if !ok {
		return parcelScore{Ready: false, Reason: "Оценка не собралась: у участка нет площади."}, nil
	}

	districtName := row.District
	areaContour, known := districts.Named(districtName)
	if !known {
		districtName = districts.Assign(*row.Lng, *row.Lat, nil)
		areaContour, known = districts.Named(districtName)
	}
	if !known {
		return parcelScore{Ready: false, Reason: "Оценка не собралась: участок лежит вне четырёх районов."}, nil
	}

	var peers []peerRow
	err = db.DB.Raw(`
		SELECT id, land_category,
			CASE
				WHEN area_ha > 0 THEN area_ha
				WHEN area_m2 > 0 THEN area_m2 / 10000.0
			END::float8 AS hectares
		FROM parcels
		WHERE district = ?
			AND (area_ha > 0 OR area_m2 > 0)
	`, districtName).Scan(&peers).Error
	if err != nil {
		return parcelScore{}, err
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
		sample = append(sample, area)
	}

	centerLng, centerLat := areaContour.Center()
	distance := metersBetween(*row.Lat, *row.Lng, centerLat, centerLng)
	farthest := 0.0
	for _, corner := range areaContour.Corners() {
		if len(corner) < 2 {
			continue
		}
		edge := metersBetween(centerLat, centerLng, corner[1], corner[0])
		if edge > farthest {
			farthest = edge
		}
	}

	location := locationFactor(distance, farthest)
	areaPart, median := areaFactor(area, sample)
	weightLocation, weightArea := scoreWeights()
	locationPoints := 100 * weightLocation * location
	areaPoints := 100 * weightArea * areaPart
	return parcelScore{
		Ready:          true,
		Category:       category,
		District:       districtName,
		Total:          locationPoints + areaPoints,
		LocationPoints: locationPoints,
		LocationMax:    100 * weightLocation,
		AreaPoints:     areaPoints,
		AreaMax:        100 * weightArea,
		DistanceM:      distance,
		AreaHa:         area,
		MedianHa:       median,
	}, nil
}

func scoreWeights() (float64, float64) {
	var row models.ScoreSetting
	err := db.DB.Order("id").First(&row).Error
	if err != nil || row.WeightLocation < 0 || row.WeightArea < 0 || row.WeightLocation+row.WeightArea == 0 {
		return 0.75, 0.25
	}
	return row.WeightLocation, row.WeightArea
}

func hectaresOf(areaHa, areaM2 *float64) (float64, bool) {
	if areaHa != nil && *areaHa > 0 {
		return *areaHa, true
	}
	if areaM2 != nil && *areaM2 > 0 {
		return *areaM2 / 10000, true
	}
	return 0, false
}

func locationFactor(distance, farthest float64) float64 {
	if farthest <= 0 {
		return 1
	}
	return clamp01(1 - distance/farthest)
}

func areaFactor(value float64, sample []float64) (float64, float64) {
	median := medianOf(sample)
	maxDev := 0.0
	for _, item := range sample {
		dev := math.Abs(item - median)
		if dev > maxDev {
			maxDev = dev
		}
	}
	if maxDev == 0 {
		return 1, median
	}
	return clamp01(1 - math.Abs(value-median)/maxDev), median
}

func medianOf(sample []float64) float64 {
	if len(sample) == 0 {
		return 0
	}
	ordered := append([]float64(nil), sample...)
	sort.Float64s(ordered)
	mid := len(ordered) / 2
	if len(ordered)%2 == 1 {
		return ordered[mid]
	}
	return (ordered[mid-1] + ordered[mid]) / 2
}

func metersBetween(lat1, lng1, lat2, lng2 float64) float64 {
	const earth = 6378137.0
	rad := math.Pi / 180
	φ1 := lat1 * rad
	φ2 := lat2 * rad
	dφ := (lat2 - lat1) * rad
	dλ := (lng2 - lng1) * rad
	h := math.Sin(dφ/2)*math.Sin(dφ/2) + math.Cos(φ1)*math.Cos(φ2)*math.Sin(dλ/2)*math.Sin(dλ/2)
	return 2 * earth * math.Asin(math.Min(1, math.Sqrt(h)))
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
