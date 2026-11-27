package parcels

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestComposeReportKeepsThePassportAndTheHints(t *testing.T) {
	area := 1.5
	card := parcelCard{CadastralNumber: "24:55:1:1", Address: "Ленинский проспект", LandCategory: "Земли населенных пунктов", AreaHa: &area}
	score := parcelScore{Ready: true, Total: 75, LocationPoints: 50, LocationMax: 75, AreaPoints: 25, AreaMax: 25, DistanceM: 1200, MedianHa: 2, Category: "Земли населенных пунктов", District: "Центральный"}
	card.Score = &score
	doc := composeReport(card, "Центральный", "Модератор", time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC), quarterReport{Ready: false, Reason: "Сравнить не с чем."})
	if doc.Number != "24:55:1:1" || doc.Author != "Модератор" || doc.FormedOn != "27.11.2026" {
		t.Fatalf("%#v", doc)
	}
	if doc.Passport[0].Value != "Ленинский проспект" || doc.Passport[1].Value != "Центральный" {
		t.Fatalf("passport %#v", doc.Passport)
	}
	if doc.Score == nil || !strings.Contains(doc.Score.Lines[2].Hint, "от 0 до 100") || !strings.Contains(doc.Score.Note, "не является ценой") {
		t.Fatalf("score %#v", doc.Score)
	}
	if doc.Quarter == nil || doc.Quarter.Note != "Сравнить не с чем." || len(doc.Charts) != 0 {
		t.Fatalf("quarter %#v charts %#v", doc.Quarter, doc.Charts)
	}
}

func TestRenderReportWritesPdfAndDocx(t *testing.T) {
	doc := reportDocument{
		Title: "Оценка инвестиционного потенциала участка", FormedOn: "27.11.2026", Author: "Модератор", Number: "24:55:1:1",
		Passport: []reportLine{{Label: "Адрес", Value: "Ленинский проспект"}},
		Quarter:  &reportSection{Title: "Квартал", Note: "Соседи рядом.", Lines: []reportLine{{Label: "Соседей", Value: "2", Hint: "Площадь этой карточки."}}},
		Charts:   []reportChart{{Title: "Площадь", Slices: []reportSlice{{Name: "Этот участок", Value: 2, Color: "#141414"}, {Name: "Медиана соседей", Value: 1, Color: "#7a5a3a"}}}},
	}
	choices := reportChoices{Quarter: true, Charts: true}
	pdf, err := renderPDF(doc, choices)
	if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatalf("pdf %v %d", err, len(pdf))
	}
	docx, err := renderDocx(doc, choices)
	if err != nil || !bytes.HasPrefix(docx, []byte("PK")) {
		t.Fatalf("docx %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, file := range reader.File {
		if file.Name == "word/document.xml" {
			found = true
		}
	}
	if !found {
		t.Fatal("document.xml missing")
	}
}

func TestChartBoxKeepsACircleSquare(t *testing.T) {
	width, height := chartBox(280, 280)
	if width != height || width > 80 {
		t.Fatalf("circle %v %v", width, height)
	}
	wide, short := chartBox(640, 220)
	if wide <= short || wide > 110 {
		t.Fatalf("bars %v %v", wide, short)
	}
}

func TestRuFloatGroupsThousands(t *testing.T) {
	if ruFloat(1234.5, 2) != "1 234,5" || ruFloat(75, 1) != "75" {
		t.Fatalf("%s %s", ruFloat(1234.5, 2), ruFloat(75, 1))
	}
}
