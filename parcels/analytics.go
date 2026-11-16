package parcels

import (
	"net/http"

	"gis-backend/auth"
	"gis-backend/db"
	"gis-backend/districts"
)

type analyticsCell struct {
	District string  `json:"district"`
	Category string  `json:"category"`
	Count    int     `json:"count"`
	AreaHa   float64 `json:"area_ha"`
}

type analyticsResponse struct {
	Cells []analyticsCell `json:"cells"`
}

type catalogArea struct {
	District     string
	LandCategory string
	Hectares     float64
}

// Analytics returns how many saved catalogue plots and how much area they cover.
// The figures are the same for every person who already has a role.
// @Summary      Аналитика каталога
// @Tags         участки
// @Produce      json
// @Success      200  {object}  analyticsResponse
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Router       /api/analytics [get]
func Analytics(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.RequireUser(w, r)
	if !ok {
		return
	}
	if user.RoleID == nil {
		writeError(w, http.StatusForbidden, "аналитика доступна после назначения роли")
		return
	}

	var rows []catalogArea
	err := db.DB.Raw(`
		SELECT district, land_category,
			CASE
				WHEN area_ha > 0 THEN area_ha::float8
				WHEN area_m2 > 0 THEN area_m2::float8 / 10000.0
				ELSE 0
			END AS hectares
		FROM parcels
		WHERE in_catalog = true AND district <> ''
	`).Scan(&rows).Error
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось собрать аналитику")
		return
	}
	writeJSON(w, http.StatusOK, analyticsResponse{Cells: summarize(rows)})
}

func summarize(rows []catalogArea) []analyticsCell {
	type key struct {
		district string
		category string
	}
	totals := map[key]analyticsCell{}
	for _, row := range rows {
		if _, ok := districts.Named(row.District); !ok {
			continue
		}
		category := CategoryTitle(row.LandCategory)
		item := totals[key{row.District, category}]
		item.District = row.District
		item.Category = category
		item.Count++
		if row.Hectares > 0 {
			item.AreaHa += row.Hectares
		}
		totals[key{row.District, category}] = item
	}

	cells := make([]analyticsCell, 0, len(districts.Areas)*len(LegendTitles))
	for _, area := range districts.Areas {
		for _, category := range LegendTitles {
			item := totals[key{area.Name, category}]
			item.District = area.Name
			item.Category = category
			cells = append(cells, item)
		}
	}
	return cells
}
