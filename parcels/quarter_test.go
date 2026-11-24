package parcels

import (
	"strings"
	"testing"
)

func TestNeighborBasisUsesTheQuarterWhenItFitsAYard(t *testing.T) {
	basis, radius := neighborBasis("24:55:010101", 6)
	if basis != "quarter" || radius {
		t.Fatalf("basis %s radius %v", basis, radius)
	}
	basis, radius = neighborBasis("24:55:010101", quarterMin)
	if basis != "quarter" || radius {
		t.Fatalf("lower edge %s %v", basis, radius)
	}
	basis, radius = neighborBasis("24:55:010101", quarterMax)
	if basis != "quarter" || radius {
		t.Fatalf("upper edge %s %v", basis, radius)
	}
}

func TestNeighborBasisFallsBackWhenTheQuarterIsAPoorYard(t *testing.T) {
	cases := []struct {
		quarter string
		count   int
	}{
		{"24:55:010101", quarterMin - 1},
		{"24:55:010101", quarterMax + 1},
		{"", 10},
	}
	for _, item := range cases {
		basis, radius := neighborBasis(item.quarter, item.count)
		if basis != "radius" || !radius {
			t.Fatalf("%q %d -> %s %v", item.quarter, item.count, basis, radius)
		}
	}
}

func TestAssembleQuarterComparesThePlotWithNeighborMedians(t *testing.T) {
	neighbors := []quarterPlace{
		{Category: "Земли населённых пунктов", Use: "Для дома", Hectares: 1, HasArea: true, Cost: 10, HasCost: true, Score: 10, HasScore: true},
		{Category: "Земли населенных пунктов", Use: "Для дома", Hectares: 2, HasArea: true, Cost: 20, HasCost: true, Score: 20, HasScore: true},
		{Category: "Земли населенных пунктов", Use: "Для дома", Hectares: 3, HasArea: true, Cost: 30, HasCost: true, Score: 30, HasScore: true},
		{Category: "Земли промышленности", Use: "Для склада", Hectares: 4, HasArea: true, Cost: 40, HasCost: true, Score: 40, HasScore: true},
		{Category: "Земли промышленности", Use: "", Hectares: 10, HasArea: true, Cost: 100, HasCost: true, Score: 80, HasScore: true},
	}
	report := assembleQuarter(quarterInput{
		Quarter:      "24:55:010101",
		QuarterCount: len(neighbors),
		HasBoundary:  true,
		Basis:        "quarter",
		Subject:      quarterPlace{Category: "Земли населённых пунктов", Use: "Для дома", Hectares: 100, HasArea: true, Cost: 5, HasCost: true, Score: 50, HasScore: true},
		Neighbors:    neighbors,
	})
	if !report.Ready || report.NeighborCount != 5 || report.SameCategory != 3 || report.SameUse != 3 {
		t.Fatalf("%#v", report)
	}
	if report.AreaMedianHa == nil || *report.AreaMedianHa != 3 {
		t.Fatalf("area median %#v", report.AreaMedianHa)
	}
	if report.CostMedian == nil || *report.CostMedian != 30 {
		t.Fatalf("cost median %#v", report.CostMedian)
	}
	if report.ScoreMedian == nil || *report.ScoreMedian != 30 {
		t.Fatalf("score median %#v", report.ScoreMedian)
	}
	if report.AreaHa == nil || *report.AreaHa != 100 {
		t.Fatalf("subject area %#v", report.AreaHa)
	}
	if len(report.Categories) != 2 || report.Categories[0].Name != "Земли населенных пунктов" || report.Categories[0].Count != 3 {
		t.Fatalf("categories %#v", report.Categories)
	}
	if report.Uses[0].Name != "Для дома" || report.Uses[0].Count != 3 {
		t.Fatalf("uses %#v", report.Uses)
	}
}

func TestAssembleQuarterRefusesAThinGroup(t *testing.T) {
	report := assembleQuarter(quarterInput{
		HasBoundary: true,
		Basis:       "radius",
		Neighbors:   []quarterPlace{{}, {}, {}, {}},
	})
	if report.Ready || report.Reason == "" {
		t.Fatalf("%#v", report)
	}
}

func TestAssembleQuarterRefusesRadiusWithoutABoundary(t *testing.T) {
	report := assembleQuarter(quarterInput{Basis: "radius", HasBoundary: false})
	if report.Ready || report.Reason == "" {
		t.Fatalf("%#v", report)
	}
}

func TestUseSlicesFoldTheLongTail(t *testing.T) {
	labels := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"}
	slices := useSlices(labels)
	if len(slices) != quarterUseLimit || slices[len(slices)-1].Name != "Остальные" || slices[len(slices)-1].Count != 2 {
		t.Fatalf("%#v", slices)
	}
}

func TestQuarterNoteNamesTheFallback(t *testing.T) {
	wide := quarterNote("24:55:1", quarterMax+5, 6, "radius")
	if !strings.Contains(wide, "для двора это много") || !strings.Contains(wide, "80") {
		t.Fatal(wide)
	}
	missing := quarterNote("", 0, 7, "radius")
	if !strings.Contains(missing, "не указан") {
		t.Fatal(missing)
	}
}
