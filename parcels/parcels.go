package parcels

import (
	"encoding/json"
	"net/http"

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

// Layer returns saved parcel boundaries. Only a verified person may read it.
func Layer(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.RequireUser(w, r)
	if !ok {
		return
	}
	if user.VerificationStatus != models.StatusVerified {
		writeError(w, http.StatusForbidden, "кадастровая карта доступна после проверки")
		return
	}

	var rows []parcelRow
	err := db.DB.Raw(`
		SELECT id, cadastral_number, land_category, ST_AsGeoJSON(boundary)::text AS geojson
		FROM parcels
		WHERE boundary IS NOT NULL
		ORDER BY id
	`).Scan(&rows).Error
	if err != nil {
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
