package parcels

import (
	"encoding/json"
	"errors"
	"log"

	"gis-backend/db"
	"gis-backend/districts"
	"gis-backend/nspd"
)

func saveCatalogPlot(plot nspd.Plot) error {
	err := upsertCatalog(plot, plot.Geometry, districtOf(plot))
	if err != nil && len(plot.Geometry) > 0 {
		log.Printf("parcel boundary: %v", err)
		return upsertCatalog(plot, nil, districtOf(plot))
	}
	return err
}

func districtOf(plot nspd.Plot) string {
	lng, lat, ok := nspd.Anchor(plot.Geometry)
	if !ok {
		return ""
	}
	return districts.Assign(lng, lat, nspd.Vertices(plot.Geometry))
}

func upsertCatalog(plot nspd.Plot, geometry json.RawMessage, district string) error {
	geo := ""
	if len(geometry) > 0 {
		geo = string(geometry)
	}
	var row struct {
		ID uint
	}
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
			?, '', true,
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
			district = EXCLUDED.district,
			in_catalog = true,
			boundary = EXCLUDED.boundary,
			updated_at = NOW()
		RETURNING id
	`,
		plot.CadastralNumber, plot.ObjectKind, plot.ParcelKind, plot.AssignedOn, plot.CadastralQuarter, plot.Address,
		plot.AreaHa, plot.AreaM2, plot.AreaName, plot.Status, plot.LandCategory, plot.PermittedUse,
		plot.OwnershipForm, plot.CadastralCost, plot.CostIndex,
		district,
		geo, geo,
	).Scan(&row).Error
	if err != nil {
		return err
	}
	if row.ID == 0 {
		return errors.New("parcel was not saved")
	}
	return nil
}
