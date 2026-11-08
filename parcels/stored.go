package parcels

import (
	"encoding/json"
	"math"
	"net/http"

	"gis-backend/auth"
	"gis-backend/db"
	"gis-backend/models"
)

// StoredAtPoint opens a card from a parcel that is already saved.
// It does not ask the public map.
func StoredAtPoint(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.RequireUser(w, r)
	if !ok {
		return
	}
	if user.VerificationStatus != models.StatusVerified {
		writeError(w, http.StatusForbidden, "кадастровая карта доступна после проверки")
		return
	}

	var body struct {
		Lat *float64 `json:"lat"`
		Lng *float64 `json:"lng"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Lat == nil || body.Lng == nil {
		writeError(w, http.StatusBadRequest, "нужны широта и долгота")
		return
	}
	if math.IsNaN(*body.Lat) || math.IsNaN(*body.Lng) || *body.Lat < -90 || *body.Lat > 90 || *body.Lng < -180 || *body.Lng > 180 {
		writeError(w, http.StatusBadRequest, "нужны широта и долгота")
		return
	}

	card, found, err := readStoredParcel(*body.Lat, *body.Lng)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось прочитать участок")
		return
	}
	if !found {
		writeJSON(w, http.StatusOK, lookupResponse{Found: false})
		return
	}
	writeJSON(w, http.StatusOK, lookupResponse{Found: true, Parcel: &card})
}

func readStoredParcel(lat, lng float64) (parcelCard, bool, error) {
	var row parcelCard
	err := db.DB.Raw(`
		SELECT id, cadastral_number, object_kind, parcel_kind, assigned_on, cadastral_quarter, address,
			area_ha::float8 AS area_ha, area_m2::float8 AS area_m2, area_name, status,
			land_category, permitted_use, ownership_form,
			cadastral_cost::float8 AS cadastral_cost, cost_index::float8 AS cost_index
		FROM parcels
		WHERE boundary IS NOT NULL
			AND ST_Covers(boundary, ST_SetSRID(ST_MakePoint(?, ?), 4326))
		ORDER BY COALESCE(area_m2, 1000000000000)
		LIMIT 1
	`, lng, lat).Scan(&row).Error
	if err != nil {
		return parcelCard{}, false, err
	}
	if row.ID == 0 {
		return parcelCard{}, false, nil
	}
	return row, true, nil
}
