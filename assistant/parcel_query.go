package assistant

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"gis-backend/db"
)

const (
	compareAreaDesc = "area_desc"
	compareAreaAsc  = "area_asc"
	compareCostDesc = "cost_desc"
	compareCostAsc  = "cost_asc"
)

type parcelQuery struct {
	District string
	Kind     string
}

type parcelHit struct {
	CadastralNumber string
	District        string
	LandCategory    string
	PermittedUse    string
	Hectares        *float64
	CadastralCost   *float64
	Lat             *float64
	Lng             *float64
}

type ParcelSpot struct {
	Number string  `json:"number"`
	Lat    float64 `json:"lat"`
	Lng    float64 `json:"lng"`
}

func matchParcelQuestion(question string) (parcelQuery, bool) {
	text := strings.ToLower(strings.ReplaceAll(question, "ё", "е"))
	district, ok := oneDistrict(text)
	if !ok {
		return parcelQuery{}, false
	}
	kind, ok := oneCompare(text)
	if !ok {
		return parcelQuery{}, false
	}
	return parcelQuery{District: district, Kind: kind}, true
}

func oneDistrict(text string) (string, bool) {
	found := ""
	for _, item := range []struct{ needle, name string }{
		{"талнах", "Талнах"},
		{"кайеркан", "Кайеркан"},
		{"оганер", "Оганер"},
		{"центральн", "Центральный"},
	} {
		if strings.Contains(text, item.needle) {
			if found != "" {
				return "", false
			}
			found = item.name
		}
	}
	if found == "" {
		return "", false
	}
	return found, true
}

func oneCompare(text string) (string, bool) {
	areaMax := hasAny(text, "самый больш", "самая больш", "самого больш", "наибольш", "крупнейш")
	areaMin := hasAny(text, "самый маленьк", "самая маленьк", "самого маленьк", "самый мал", "наименьш")
	costMax := hasAny(text, "самый дорог", "самая дорог", "самого дорог", "наиболее дорог")
	costMin := hasAny(text, "самый дешев", "самая дешев", "самого дешев", "наиболее дешев", "наименее дорог")
	if strings.Contains(text, "максимальн") && strings.Contains(text, "площад") {
		areaMax = true
	}
	if strings.Contains(text, "минимальн") && strings.Contains(text, "площад") {
		areaMin = true
	}
	if strings.Contains(text, "максимальн") && (strings.Contains(text, "стоим") || strings.Contains(text, "дорог")) {
		costMax = true
	}
	if strings.Contains(text, "минимальн") && (strings.Contains(text, "стоим") || strings.Contains(text, "дешев")) {
		costMin = true
	}
	kind := ""
	count := 0
	for _, item := range []struct {
		on   bool
		name string
	}{
		{areaMax, compareAreaDesc},
		{areaMin, compareAreaAsc},
		{costMax, compareCostDesc},
		{costMin, compareCostAsc},
	} {
		if !item.on {
			continue
		}
		count++
		kind = item.name
	}
	if count != 1 {
		return "", false
	}
	return kind, true
}

func hasAny(text string, parts ...string) bool {
	for _, part := range parts {
		if strings.Contains(text, part) {
			return true
		}
	}
	return false
}

func parcelAnswer(ctx context.Context, query parcelQuery) (string, *ParcelSpot, error) {
	row, ok, err := extremeParcel(ctx, query)
	if err != nil {
		return "", nil, err
	}
	if !ok {
		if query.Kind == compareCostDesc || query.Kind == compareCostAsc {
			return fmt.Sprintf("В районе %s нет участка с указанной кадастровой стоимостью.", query.District), nil, nil
		}
		return fmt.Sprintf("В районе %s нет участка с указанной площадью.", query.District), nil, nil
	}
	return describeParcel(query.Kind, row), spotOf(row), nil
}

func extremeParcel(ctx context.Context, query parcelQuery) (parcelHit, bool, error) {
	measure := "hectares"
	direction := "DESC"
	switch query.Kind {
	case compareAreaAsc:
		direction = "ASC"
	case compareCostDesc:
		measure = "cadastral_cost"
	case compareCostAsc:
		measure = "cadastral_cost"
		direction = "ASC"
	}
	var row parcelHit
	err := db.DB.WithContext(ctx).Raw(`
		WITH sized AS (
			SELECT cadastral_number, district, land_category, permitted_use,
				cadastral_cost::float8 AS cadastral_cost,
				CASE
					WHEN area_ha > 0 THEN area_ha::float8
					WHEN area_m2 > 0 THEN area_m2::float8 / 10000
					ELSE NULL
				END AS hectares,
				CASE WHEN boundary IS NULL THEN NULL ELSE ST_Y(ST_Centroid(boundary))::float8 END AS lat,
				CASE WHEN boundary IS NULL THEN NULL ELSE ST_X(ST_Centroid(boundary))::float8 END AS lng
			FROM parcels
			WHERE district = ?
		)
		SELECT cadastral_number, district, land_category, permitted_use, hectares, cadastral_cost, lat, lng
		FROM sized
		WHERE `+measure+` > 0
		ORDER BY `+measure+` `+direction+`, cadastral_number
		LIMIT 1
	`, query.District).Scan(&row).Error
	if err != nil {
		return parcelHit{}, false, err
	}
	if row.CadastralNumber == "" {
		return parcelHit{}, false, nil
	}
	return row, true, nil
}

func describeParcel(kind string, row parcelHit) string {
	lead := map[string]string{
		compareAreaDesc: "Самый большой по площади участок",
		compareAreaAsc:  "Самый маленький по площади участок",
		compareCostDesc: "Участок с самой высокой кадастровой стоимостью",
		compareCostAsc:  "Участок с самой низкой кадастровой стоимостью",
	}[kind]
	parts := []string{fmt.Sprintf("%s в районе %s — %s.", lead, row.District, row.CadastralNumber)}
	if row.Hectares != nil && *row.Hectares > 0 {
		parts = append(parts, "Площадь "+formatHa(*row.Hectares)+".")
	} else {
		parts = append(parts, "Площадь не указана.")
	}
	if category := strings.TrimSpace(row.LandCategory); category != "" {
		parts = append(parts, "Категория: "+category+".")
	}
	if use := strings.TrimSpace(row.PermittedUse); use != "" {
		parts = append(parts, "Разрешённое использование: "+use+".")
	}
	if row.CadastralCost != nil && *row.CadastralCost > 0 {
		parts = append(parts, "Кадастровая стоимость "+formatRub(*row.CadastralCost)+".")
	} else {
		parts = append(parts, "Кадастровая стоимость не указана.")
	}
	return strings.Join(parts, " ")
}

func spotOf(row parcelHit) *ParcelSpot {
	if row.Lat == nil || row.Lng == nil || *row.Lat == 0 || *row.Lng == 0 {
		return nil
	}
	return &ParcelSpot{Number: row.CadastralNumber, Lat: *row.Lat, Lng: *row.Lng}
}

func formatHa(value float64) string {
	text := strconv.FormatFloat(value, 'f', 2, 64)
	text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	return strings.ReplaceAll(text, ".", ",") + " га"
}

func formatRub(value float64) string {
	rounded := math.Round(value*100) / 100
	whole := int64(math.Trunc(rounded))
	if whole < 0 {
		whole = -whole
	}
	body := strconv.FormatInt(whole, 10)
	groups := make([]string, 0, (len(body)+2)/3)
	for len(body) > 3 {
		groups = append([]string{body[len(body)-3:]}, groups...)
		body = body[:len(body)-3]
	}
	groups = append([]string{body}, groups...)
	text := strings.Join(groups, " ")
	kopecks := int64(math.Round((rounded - float64(int64(rounded))) * 100))
	if kopecks < 0 {
		kopecks = -kopecks
	}
	if kopecks != 0 {
		text += "," + fmt.Sprintf("%02d", kopecks)
	}
	return text + " ₽"
}
