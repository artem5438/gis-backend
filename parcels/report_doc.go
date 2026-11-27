package parcels

import (
	"fmt"
	"strings"
	"time"
)

type reportLine struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Hint  string `json:"hint,omitempty"`
}

type reportSlice struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Color string  `json:"color"`
}

type reportChart struct {
	Title  string        `json:"title"`
	Kind   string        `json:"kind"`
	Slices []reportSlice `json:"slices"`
}

type reportSection struct {
	Title string       `json:"title"`
	Note  string       `json:"note,omitempty"`
	Lines []reportLine `json:"lines"`
}

type reportDocument struct {
	Title    string         `json:"title"`
	FormedOn string         `json:"formed_on"`
	Author   string         `json:"author"`
	Number   string         `json:"number"`
	Passport []reportLine   `json:"passport"`
	Score    *reportSection `json:"score,omitempty"`
	Quarter  *reportSection `json:"quarter,omitempty"`
	Charts   []reportChart  `json:"charts,omitempty"`
}

type reportChoices struct {
	Score   bool
	Quarter bool
	Charts  bool
}

func composeReport(card parcelCard, district, author string, when time.Time, quarter quarterReport) reportDocument {
	return reportDocument{
		Title:    "Оценка инвестиционного потенциала участка",
		FormedOn: when.Format("02.01.2006"),
		Author:   strings.TrimSpace(author),
		Number:   card.CadastralNumber,
		Passport: passportLines(card, district),
		Score:    scoreSection(card.Score),
		Quarter:  quarterSection(quarter),
		Charts:   quarterCharts(quarter),
	}
}

func passportLines(card parcelCard, district string) []reportLine {
	area := "не указано"
	if card.AreaHa != nil && *card.AreaHa > 0 {
		area = ruFloat(*card.AreaHa, 2) + " га"
	} else if card.AreaM2 != nil && *card.AreaM2 > 0 {
		area = ruFloat(*card.AreaM2, 2) + " кв. м"
	}
	cost := "не указано"
	if card.CadastralCost != nil {
		cost = ruFloat(*card.CadastralCost, 2) + " руб."
	}
	index := "не указано"
	if card.CostIndex != nil {
		index = ruFloat(*card.CostIndex, 2) + " руб./кв. м"
	}
	return []reportLine{
		{Label: "Адрес", Value: textOrEmpty(card.Address)},
		{Label: "Район", Value: textOrEmpty(district)},
		{Label: "Категория земель", Value: textOrEmpty(card.LandCategory)},
		{Label: "Вид разрешённого использования", Value: textOrEmpty(card.PermittedUse)},
		{Label: "Площадь", Value: area},
		{Label: "Кадастровая стоимость", Value: cost},
		{Label: "Удельный показатель кадастровой стоимости", Value: index},
		{Label: "Форма собственности", Value: textOrEmpty(card.OwnershipForm)},
		{Label: "Статус", Value: textOrEmpty(card.Status)},
		{Label: "Кадастровый квартал", Value: textOrEmpty(card.CadastralQuarter)},
	}
}

func scoreSection(score *parcelScore) *reportSection {
	if score == nil || !score.Ready {
		reason := "Оценка не собралась."
		if score != nil && strings.TrimSpace(score.Reason) != "" {
			reason = score.Reason
		}
		return &reportSection{Title: "Балл", Note: reason}
	}
	return &reportSection{
		Title: "Балл",
		Note:  "Балл не является ценой участка в рублях.",
		Lines: []reportLine{
			{Label: "Категория земель", Value: textOrEmpty(score.Category)},
			{Label: "Район", Value: textOrEmpty(score.District)},
			{Label: "Оценка", Value: ruFloat(score.Total, 1), Hint: "Общий балл участка от 0 до 100. Чем выше, тем удачнее место и размер."},
			{Label: "Местоположение", Value: fmt.Sprintf("%s из %s", ruFloat(score.LocationPoints, 1), ruFloat(score.LocationMax, 1)), Hint: "Баллы за место. Чем ближе участок к центру своего района, тем их больше. Максимум 75."},
			{Label: "До центра района", Value: ruMeters(score.DistanceM), Hint: "Как далеко участок от центра своего района."},
			{Label: "Площадь", Value: fmt.Sprintf("%s из %s", ruFloat(score.AreaPoints, 1), ruFloat(score.AreaMax, 1)), Hint: "Баллы за размер. Обычный для этого района размер получает больше. Максимум 25."},
			{Label: "Медиана площади", Value: ruFloat(score.MedianHa, 2) + " га", Hint: "Обычный размер таких же участков в этом районе."},
		},
	}
}

func quarterSection(quarter quarterReport) *reportSection {
	if !quarter.Ready {
		reason := "Сравнить не с чем."
		if strings.TrimSpace(quarter.Reason) != "" {
			reason = quarter.Reason
		}
		return &reportSection{Title: "Квартал", Note: reason}
	}
	ofNeighbors := func(value int) string {
		return fmt.Sprintf("%d из %d", value, quarter.NeighborCount)
	}
	return &reportSection{
		Title: "Квартал",
		Note:  strings.TrimSpace(quarter.Note + " Сравнение по текущему состоянию соседей."),
		Lines: []reportLine{
			{Label: "Соседей", Value: fmt.Sprintf("%d", quarter.NeighborCount)},
			{Label: "Площадь", Value: ruOptional(quarter.AreaHa, " га"), Hint: "Площадь этой карточки."},
			{Label: "Медиана площади", Value: ruOptional(quarter.AreaMedianHa, " га"), Hint: "Середина площадей соседних участков."},
			{Label: "Удельный показатель", Value: ruOptional(quarter.CostIndex, " руб./кв. м"), Hint: "Кадастровая стоимость на квадратный метр у этой карточки."},
			{Label: "Медиана показателя", Value: ruOptional(quarter.CostMedian, " руб./кв. м"), Hint: "Середина удельных показателей соседних участков."},
			{Label: "Та же категория", Value: ofNeighbors(quarter.SameCategory), Hint: "Сколько соседей в той же категории земель."},
			{Label: "То же использование", Value: ofNeighbors(quarter.SameUse), Hint: "Сколько соседей с тем же видом разрешённого использования."},
			{Label: "Оценка", Value: ruOptional(quarter.Score, ""), Hint: "Балл этой карточки, тот же, что на вкладке «Оценка»."},
			{Label: "Медиана оценки", Value: ruOptional(quarter.ScoreMedian, ""), Hint: "Середина баллов соседних участков."},
		},
	}
}

func quarterCharts(quarter quarterReport) []reportChart {
	if !quarter.Ready {
		return nil
	}
	charts := []reportChart{
		{Title: "Соседи по категориям", Kind: "donut", Slices: categoryChart(quarter.Categories)},
		{Title: "Соседи по виду использования", Kind: "horizontal", Slices: useChart(quarter.Uses)},
	}
	if quarter.AreaHa != nil && quarter.AreaMedianHa != nil {
		charts = append(charts, reportChart{
			Title: "Площадь",
			Kind:  "bar",
			Slices: []reportSlice{
				{Name: "Этот участок", Value: *quarter.AreaHa, Color: "#141414"},
				{Name: "Медиана соседей", Value: *quarter.AreaMedianHa, Color: "#7a5a3a"},
			},
		})
	}
	if quarter.CostIndex != nil && quarter.CostMedian != nil {
		charts = append(charts, reportChart{
			Title: "Удельный показатель",
			Kind:  "bar",
			Slices: []reportSlice{
				{Name: "Этот участок", Value: *quarter.CostIndex, Color: "#141414"},
				{Name: "Медиана соседей", Value: *quarter.CostMedian, Color: "#7a5a3a"},
			},
		})
	}
	return charts
}

func categoryChart(rows []quarterSlice) []reportSlice {
	slices := make([]reportSlice, 0, len(rows))
	for _, row := range rows {
		slices = append(slices, reportSlice{Name: row.Name, Value: float64(row.Count), Color: categoryStroke(row.Name)})
	}
	return slices
}

func useChart(rows []quarterSlice) []reportSlice {
	colors := []string{"#141414", "#7a5a3a", "#4d78b0", "#5d8a48", "#a85b55", "#8d6a96", "#8c7a22", "#6b7280"}
	slices := make([]reportSlice, 0, len(rows))
	for index, row := range rows {
		slices = append(slices, reportSlice{Name: row.Name, Value: float64(row.Count), Color: colors[index%len(colors)]})
	}
	return slices
}

func categoryStroke(name string) string {
	text := strings.ToLower(strings.ReplaceAll(name, "ё", "е"))
	switch {
	case strings.Contains(text, "сельскохозяй"):
		return "#8c7a22"
	case strings.Contains(text, "населен"):
		return "#a85b55"
	case strings.Contains(text, "промышлен") || strings.Contains(text, "энергетик") || strings.Contains(text, "транспорт") || strings.Contains(text, "специальн"):
		return "#7a5a3a"
	case strings.Contains(text, "особо охраня"):
		return "#4f6b3d"
	case strings.Contains(text, "лесн"):
		return "#5d8a48"
	case strings.Contains(text, "водн"):
		return "#4d78b0"
	case strings.Contains(text, "запас"):
		return "#8d6a96"
	default:
		return "#6b7280"
	}
}

func textOrEmpty(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "не указано"
	}
	return value
}

func ruOptional(value *float64, unit string) string {
	if value == nil {
		return "не указано"
	}
	text := ruFloat(*value, 2)
	if unit == "" {
		return ruFloat(*value, 1)
	}
	return text + unit
}

func ruMeters(value float64) string {
	if value >= 1000 {
		return ruFloat(value/1000, 2) + " км"
	}
	return ruFloat(value, 0) + " м"
}
