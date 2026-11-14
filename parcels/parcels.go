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
	Geometry   json.RawMessage `json:"geometry"`
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
func drawWindow(values url.Values) (*drawBox, float64, error) {
	keys := []string{"south", "west", "north", "east"}
	present := 0
	for _, key := range keys {
		if values.Get(key) != "" {
			present++
		}
	}
	if present == 0 {
		return nil, 0, nil
	}
	if present != len(keys) {
		return nil, 0, errDrawWindow
	}
	nums := make([]float64, len(keys))
	for i, key := range keys {
		value, err := strconv.ParseFloat(values.Get(key), 64)
		if err != nil {
			return nil, 0, errDrawWindow
		}
		nums[i] = value
	}
	box := drawBox{south: nums[0], west: nums[1], north: nums[2], east: nums[3]}
	if box.south < -90 || box.north > 90 || box.south >= box.north || box.west < -180 || box.east > 180 || box.west >= box.east {
		return nil, 0, errDrawWindow
	}
	simplify := 0.0
	if raw := values.Get("simplify"); raw != "" {
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil || value < 0 {
			return nil, 0, errDrawWindow
		}
		if value > 0.02 {
			value = 0.02
		}
		simplify = value
	}
	return &box, simplify, nil
}

var errDrawWindow = errString("не удалось прочитать окно карты")

type errString string

func (e errString) Error() string { return string(e) }

// Layer returns saved parcel boundaries. Only a verified person may read it.
// Without a window the whole city is returned. With a window only parcels in that rectangle come back.
func Layer(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.RequireUser(w, r)
	if !ok {
		return
	}
	if user.VerificationStatus != models.StatusVerified {
		writeError(w, http.StatusForbidden, "кадастровая карта доступна после проверки")
		return
	}
	box, simplify, err := drawWindow(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	query := `
		SELECT id, cadastral_number, land_category,
			ST_AsGeoJSON(
				COALESCE(
					CASE WHEN ? > 0 THEN ST_SimplifyPreserveTopology(boundary, ?) ELSE boundary END,
					boundary
				)
			)::text AS geojson
		FROM parcels
		WHERE boundary IS NOT NULL`
	args := []any{simplify, simplify}
	if box != nil {
		query += ` AND ST_Intersects(boundary, ST_MakeEnvelope(?, ?, ?, ?, 4326))`
		args = append(args, box.west, box.south, box.east, box.north)
	}
	query += ` ORDER BY id`

	var rows []parcelRow
	if err = db.DB.Raw(query, args...).Scan(&rows).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось прочитать участки")
		return
	}

	features := make([]feature, 0, len(rows))
	for _, row := range rows {
		if row.GeoJSON == "" {
			continue
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
	writeJSON(w, status, map[string]string{"error": message})
}
