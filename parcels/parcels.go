package parcels

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"gis-backend/auth"
	"gis-backend/db"
	"gis-backend/models"
)

type featureCollection struct {
	Type     string    `json:"type"`
	Features []feature `json:"features"`
}

type feature struct {
	Type       string          `json:"type"`
	ID         uint            `json:"id"`
	Geometry   json.RawMessage `json:"geometry" swaggertype:"object"`
	Properties map[string]any  `json:"properties"`
}

type parcelRow struct {
	ID              uint
	CadastralNumber string
	LandCategory    string `gorm:"column:land_category"`
	GeoJSON         string `gorm:"column:geojson"`
}

type drawBox struct {
	south, west, north, east float64
}

// drawWindow reads the map window. An empty query means the whole saved city.
func drawWindow(values url.Values) (*drawBox, float64, bool, error) {
	keys := []string{"south", "west", "north", "east"}
	present := 0
	for _, key := range keys {
		if values.Get(key) != "" {
			present++
		}
	}
	squares := values.Get("squares") == "1"
	if present == 0 {
		return nil, 0, squares, nil
	}
	if present != len(keys) {
		return nil, 0, false, errDrawWindow
	}
	nums := make([]float64, len(keys))
	for i, key := range keys {
		value, err := strconv.ParseFloat(values.Get(key), 64)
		if err != nil {
			return nil, 0, false, errDrawWindow
		}
		nums[i] = value
	}
	box := drawBox{south: nums[0], west: nums[1], north: nums[2], east: nums[3]}
	if box.south < -90 || box.north > 90 || box.south >= box.north || box.west < -180 || box.east > 180 || box.west >= box.east {
		return nil, 0, false, errDrawWindow
	}
	simplify := 0.0
	if raw := values.Get("simplify"); raw != "" {
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil || value < 0 {
			return nil, 0, false, errDrawWindow
		}
		if value > 0.02 {
			value = 0.02
		}
		simplify = value
	}
	return &box, simplify, squares, nil
}

func squareQuery(box *drawBox) (string, []any) {
	const trim = 0.00005
	const smooth = 0.00018
	where := "boundary IS NOT NULL"
	args := []any{trim}
	if box != nil {
		where += " AND ST_Intersects(boundary, ST_MakeEnvelope(?, ?, ?, ?, 4326))"
		args = append(args, box.west, box.south, box.east, box.north)
	}
	args = append(args, smooth)
	query := `
		WITH clipped AS (
			SELECT COALESCE(land_category, '') AS land_category,
				ST_MakeValid(ST_SimplifyPreserveTopology(boundary, ?)) AS geom
			FROM parcels
			WHERE ` + where + `
		),
		merged AS (
			SELECT land_category, ST_UnaryUnion(ST_Collect(geom)) AS geom
			FROM clipped
			WHERE geom IS NOT NULL AND NOT ST_IsEmpty(geom)
			GROUP BY land_category
		),
		parts AS (
			SELECT land_category, (ST_Dump(ST_SimplifyPreserveTopology(geom, ?))).geom AS geom
			FROM merged
			WHERE geom IS NOT NULL AND NOT ST_IsEmpty(geom)
		)
		SELECT
			0 AS id,
			'' AS cadastral_number,
			land_category,
			ST_AsGeoJSON(geom)::text AS geojson
		FROM parts
		WHERE geom IS NOT NULL AND NOT ST_IsEmpty(geom)`
	return query, args
}

var errDrawWindow = errString("не удалось прочитать окно карты")

type errString string

func (e errString) Error() string { return string(e) }

type errorResponse struct {
	Error string `json:"error"`
}

type pointRequest struct {
	Lat *float64 `json:"lat"`
	Lng *float64 `json:"lng"`
}

// Layer returns saved parcel boundaries. Only a verified person may read it.
// Without a window the whole city is returned. With a window only parcels in that rectangle come back.
// @Summary      Слой участков
// @Description  Без краёв окна отдаёт весь город. С краями отдаёт только участки внутри прямоугольника. Это прорисовка «Экран» и «Близко».
// @Tags         участки
// @Produce      json
// @Param        south     query  number  false  "Юг окна"
// @Param        west      query  number  false  "Запад окна"
// @Param        north     query  number  false  "Север окна"
// @Param        east      query  number  false  "Восток окна"
// @Param        simplify  query  number  false  "Упрощение линии, в градусах"
// @Param        squares   query  int     false  "1 — склеенные упрощённые контуры"
// @Success      200  {object}  featureCollection
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Router       /api/parcels [get]
func Layer(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.RequireUser(w, r)
	if !ok {
		return
	}
	if user.VerificationStatus != models.StatusVerified {
		writeError(w, http.StatusForbidden, "кадастровая карта доступна после проверки")
		return
	}
	box, simplify, squares, err := drawWindow(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var query string
	var args []any
	if squares {
		query, args = squareQuery(box)
	} else {
		query = `
		SELECT id, cadastral_number, land_category,
			ST_AsGeoJSON(
				COALESCE(
					CASE WHEN ? > 0 THEN ST_SimplifyPreserveTopology(boundary, ?) ELSE boundary END,
					boundary
				)
			)::text AS geojson
		FROM parcels
		WHERE boundary IS NOT NULL`
		args = []any{simplify, simplify}
		if box != nil {
			query += ` AND ST_Intersects(boundary, ST_MakeEnvelope(?, ?, ?, ?, 4326))`
			args = append(args, box.west, box.south, box.east, box.north)
		}
		query += ` ORDER BY id`
	}

	var rows []parcelRow
	if err = db.DB.Raw(query, args...).Scan(&rows).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось прочитать участки")
		return
	}

	features := make([]feature, 0, len(rows))
	for i, row := range rows {
		if row.GeoJSON == "" {
			continue
		}
		if squares {
			row.ID = uint(i + 1)
		}
		features = append(features, feature{
			Type:     "Feature",
			ID:       row.ID,
			Geometry: json.RawMessage(row.GeoJSON),
			Properties: map[string]any{
				"cadastral_number": row.CadastralNumber,
				"land_category":    row.LandCategory,
			},
		})
	}
	writeJSON(w, http.StatusOK, featureCollection{
		Type:     "FeatureCollection",
		Features: features,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
