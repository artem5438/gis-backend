package parcels

import (
	"encoding/json"
	"errors"
	"log"
	"math"
	"net/http"

	"gis-backend/auth"
	"gis-backend/db"
	"gis-backend/models"
	"gis-backend/nspd"
)

type parcelCard struct {
	ID               uint     `json:"id"`
	CadastralNumber  string   `json:"cadastral_number"`
	ObjectKind       string   `json:"object_kind"`
	ParcelKind       string   `json:"parcel_kind"`
	AssignedOn       string   `json:"assigned_on"`
	CadastralQuarter string   `json:"cadastral_quarter"`
	Address          string   `json:"address"`
	AreaHa           *float64 `json:"area_ha"`
	AreaM2           *float64 `json:"area_m2"`
	AreaName         string   `json:"area_name"`
	Status           string   `json:"status"`
	LandCategory     string   `json:"land_category"`
	PermittedUse     string   `json:"permitted_use"`
	OwnershipForm    string   `json:"ownership_form"`
	CadastralCost    *float64 `json:"cadastral_cost"`
	CostIndex        *float64 `json:"cost_index"`
}

type lookupResponse struct {
	Found  bool        `json:"found"`
	Parcel *parcelCard `json:"parcel,omitempty"`
}

// AtPoint asks the public map about one click and stores the land plot.
// A repeated click with the same cadastral number updates that row.
func AtPoint(w http.ResponseWriter, r *http.Request) {
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

	plot, err := nspd.LandPlotAt(*body.Lat, *body.Lng)
	if errors.Is(err, nspd.ErrEmpty) {
		writeJSON(w, http.StatusOK, lookupResponse{Found: false})
		return
	}
	if err != nil {
		log.Printf("nspd point: %v", err)
		writeError(w, http.StatusBadGateway, "Свежие данные не пришли. Сохранённые участки остаются на карте.")
		return
	}

	card, err := savePlot(plot)
	if err != nil {
		log.Printf("save parcel: %v", err)
		writeError(w, http.StatusInternalServerError, "не удалось сохранить участок")
		return
	}
	writeJSON(w, http.StatusOK, lookupResponse{Found: true, Parcel: &card})
}

func savePlot(plot nspd.Plot) (parcelCard, error) {
	card, err := upsertPlot(plot, plot.Geometry)
	if err != nil && len(plot.Geometry) > 0 {
		log.Printf("parcel boundary: %v", err)
		return upsertPlot(plot, nil)
	}
	return card, err
}

func upsertPlot(plot nspd.Plot, geometry json.RawMessage) (parcelCard, error) {
	geo := ""
	if len(geometry) > 0 {
		geo = string(geometry)
	}
	var row parcelCard
	err := db.DB.Raw(`
		INSERT INTO parcels (
			cadastral_number, object_kind, parcel_kind, assigned_on, cadastral_quarter, address,
			area_ha, area_m2, area_name, status, land_category, permitted_use,
			ownership_form, cadastral_cost, cost_index,
			district, direction, in_catalog, boundary, created_at, updated_at
		) VALUES (
			?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?,
			?, ?, ?,
			'', '', false,
			CASE WHEN ? = '' THEN NULL ELSE ST_SetSRID(ST_GeomFromGeoJSON(?), 4326) END,
			NOW(), NOW()
		)
		ON CONFLICT (cadastral_number) DO UPDATE SET
			object_kind = EXCLUDED.object_kind,
			parcel_kind = EXCLUDED.parcel_kind,
			assigned_on = EXCLUDED.assigned_on,
			cadastral_quarter = EXCLUDED.cadastral_quarter,
			address = EXCLUDED.address,
			area_ha = EXCLUDED.area_ha,
			area_m2 = EXCLUDED.area_m2,
			area_name = EXCLUDED.area_name,
			status = EXCLUDED.status,
			land_category = EXCLUDED.land_category,
			permitted_use = EXCLUDED.permitted_use,
			ownership_form = EXCLUDED.ownership_form,
			cadastral_cost = EXCLUDED.cadastral_cost,
			cost_index = EXCLUDED.cost_index,
			boundary = EXCLUDED.boundary,
			updated_at = NOW()
		RETURNING id, cadastral_number, object_kind, parcel_kind, assigned_on, cadastral_quarter, address,
			area_ha::float8 AS area_ha, area_m2::float8 AS area_m2, area_name, status,
			land_category, permitted_use, ownership_form,
			cadastral_cost::float8 AS cadastral_cost, cost_index::float8 AS cost_index
	`,
		plot.CadastralNumber, plot.ObjectKind, plot.ParcelKind, plot.AssignedOn, plot.CadastralQuarter, plot.Address,
		plot.AreaHa, plot.AreaM2, plot.AreaName, plot.Status, plot.LandCategory, plot.PermittedUse,
		plot.OwnershipForm, plot.CadastralCost, plot.CostIndex,
		geo, geo,
	).Scan(&row).Error
	if err != nil {
		return parcelCard{}, err
	}
	if row.ID == 0 {
		return parcelCard{}, errors.New("parcel was not saved")
	}
	return row, nil
}
