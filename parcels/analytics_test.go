package parcels

import "testing"

func TestSummarizeKeepsEveryDistrictAndCategory(t *testing.T) {
	cells := summarize([]catalogArea{
		{District: "Талнах", LandCategory: "Земли населённых пунктов", Hectares: 1},
		{District: "Талнах", LandCategory: "Земли населенных пунктов", Hectares: 0.5},
		{District: "Кайеркан", LandCategory: "", Hectares: 2},
		{District: "Чужой", LandCategory: "Земли запаса", Hectares: 9},
	})
	if len(cells) != 4*8 {
		t.Fatalf("cells %d", len(cells))
	}
	var talnakhTown, kayerkanUnset, foreign int
	var talnakhArea float64
	for _, cell := range cells {
		if cell.District == "Чужой" {
			foreign += cell.Count
		}
		if cell.District == "Талнах" && cell.Category == "Земли населенных пунктов" {
			talnakhTown = cell.Count
			talnakhArea = cell.AreaHa
		}
		if cell.District == "Кайеркан" && cell.Category == "Категория не установлена" {
			kayerkanUnset = cell.Count
		}
		if cell.District == "Оганер" && cell.Count != 0 {
			t.Fatalf("oganer %#v", cell)
		}
	}
	if talnakhTown != 2 || talnakhArea != 1.5 || kayerkanUnset != 1 || foreign != 0 {
		t.Fatalf("talnakh %d area %v unset %d foreign %d", talnakhTown, talnakhArea, kayerkanUnset, foreign)
	}
}
