package parcels

import (
	"encoding/json"
	"strings"
	"time"

	"gis-backend/db"
	"gis-backend/models"
)

type savedReport struct {
	ID        uint            `json:"id"`
	UserID    uint            `json:"-" gorm:"column:user_id"`
	Format    string          `json:"format"`
	Number    string          `json:"number" gorm:"column:cadastral_number"`
	Score     bool            `json:"score" gorm:"column:include_score"`
	Quarter   bool            `json:"quarter" gorm:"column:include_quarter"`
	Charts    bool            `json:"charts" gorm:"column:include_charts"`
	CreatedAt time.Time       `json:"created_at"`
	Body      json.RawMessage `json:"body,omitempty" gorm:"-"`
}

func loadReportParcel(id uint, number string) (parcelCard, string, error) {
	var row parcelCard
	var district string
	query := `
		SELECT id, cadastral_number, object_kind, parcel_kind, assigned_on, cadastral_quarter, address,
			area_ha::float8 AS area_ha, area_m2::float8 AS area_m2, area_name, status,
			land_category, permitted_use, ownership_form,
			cadastral_cost::float8 AS cadastral_cost, cost_index::float8 AS cost_index
		FROM parcels
		WHERE `
	districtQuery := `SELECT district FROM parcels WHERE `
	var arg any
	if id != 0 {
		query += `id = ?`
		districtQuery += `id = ?`
		arg = id
	} else {
		query += `cadastral_number = ?`
		districtQuery += `cadastral_number = ?`
		arg = strings.TrimSpace(number)
	}
	err := db.DB.Raw(query, arg).Scan(&row).Error
	if err != nil {
		return parcelCard{}, "", err
	}
	if row.ID == 0 {
		return parcelCard{}, "", errParcelMissing
	}
	if err = db.DB.Raw(districtQuery, arg).Scan(&district).Error; err != nil {
		return parcelCard{}, "", err
	}
	attachScore(&row)
	return row, district, nil
}

func buildReport(id uint, number, author string, when time.Time) (reportDocument, parcelCard, error) {
	card, district, err := loadReportParcel(id, number)
	if err != nil {
		return reportDocument{}, parcelCard{}, err
	}
	quarter, err := quarterFor(card.ID)
	if err != nil {
		return reportDocument{}, parcelCard{}, err
	}
	return composeReport(card, district, author, when, quarter), card, nil
}

func insertReport(user models.User, format string, card parcelCard, choices reportChoices, doc reportDocument) (savedReport, error) {
	body, err := json.Marshal(doc)
	if err != nil {
		return savedReport{}, err
	}
	var row savedReport
	err = db.DB.Raw(`
		INSERT INTO parcel_reports (
			user_id, format, cadastral_number, include_score, include_quarter, include_charts, body
		) VALUES (?, ?, ?, ?, ?, ?, ?::jsonb)
		RETURNING id, format, cadastral_number, include_score, include_quarter, include_charts, created_at
	`, user.ID, format, card.CadastralNumber, choices.Score, choices.Quarter, choices.Charts && choices.Quarter, string(body)).Scan(&row).Error
	return row, err
}

func listReports(userID uint) ([]savedReport, error) {
	var rows []savedReport
	err := db.DB.Raw(`
		SELECT id, format, cadastral_number, include_score, include_quarter, include_charts, created_at
		FROM parcel_reports
		WHERE user_id = ?
		ORDER BY id DESC
	`, userID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []savedReport{}
	}
	return rows, nil
}

func reportByID(userID, id uint) (savedReport, reportDocument, error) {
	var row savedReport
	var raw string
	err := db.DB.Raw(`
		SELECT id, user_id, format, cadastral_number, include_score, include_quarter, include_charts, created_at, body::text
		FROM parcel_reports
		WHERE id = ?
	`, id).Row().Scan(&row.ID, &row.UserID, &row.Format, &row.Number, &row.Score, &row.Quarter, &row.Charts, &row.CreatedAt, &raw)
	if err != nil {
		return savedReport{}, reportDocument{}, errParcelMissing
	}
	if row.ID == 0 || row.UserID != userID {
		return savedReport{}, reportDocument{}, errParcelMissing
	}
	var doc reportDocument
	if err = json.Unmarshal([]byte(raw), &doc); err != nil {
		return savedReport{}, reportDocument{}, err
	}
	row.Body = json.RawMessage(raw)
	return row, doc, nil
}

func applyChartKinds(doc *reportDocument, kinds map[string]string) {
	allowed := map[string]bool{"bar": true, "horizontal": true, "pie": true, "donut": true}
	for index := range doc.Charts {
		kind := kinds[doc.Charts[index].Title]
		if allowed[kind] {
			doc.Charts[index].Kind = kind
		}
		if doc.Charts[index].Kind == "" {
			doc.Charts[index].Kind = "bar"
		}
	}
}

func updateReport(userID, id uint, format string, choices reportChoices, kinds map[string]string) (savedReport, error) {
	row, doc, err := reportByID(userID, id)
	if err != nil {
		return savedReport{}, err
	}
	applyChartKinds(&doc, kinds)
	body, err := json.Marshal(doc)
	if err != nil {
		return savedReport{}, err
	}
	err = db.DB.Raw(`
		UPDATE parcel_reports
		SET format = ?, include_score = ?, include_quarter = ?, include_charts = ?, body = ?::jsonb
		WHERE id = ? AND user_id = ?
		RETURNING id, format, cadastral_number, include_score, include_quarter, include_charts, created_at
	`, format, choices.Score, choices.Quarter, choices.Charts, string(body), id, userID).Scan(&row).Error
	return row, err
}

func (row savedReport) choices() reportChoices {
	return reportChoices{Score: row.Score, Quarter: row.Quarter, Charts: row.Charts && row.Quarter}
}
