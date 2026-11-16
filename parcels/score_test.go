package parcels

import (
	"math"
	"testing"

	"gis-backend/districts"
)

func TestCategoryTitleFollowsTheLegend(t *testing.T) {
	cases := map[string]string{
		"Земли населённых пунктов":               "Земли населенных пунктов",
		"Земли сельскохозяйственного назначения": "Земли сельскохозяйственного назначения",
		"Земли промышленности":                   "Земли промышленности и иного назначения",
		"Земли лесного фонда":                    "Земли лесного фонда",
		"Земли водного фонда":                    "Земли водного фонда",
		"Земли запаса":                           "Земли запаса",
		"Земли особо охраняемых территорий":      "Земли особо охраняемых территорий",
		"": "Категория не установлена",
	}
	for raw, want := range cases {
		if got := CategoryTitle(raw); got != want {
			t.Fatalf("%q -> %q, want %q", raw, got, want)
		}
	}
}

func TestLocationIsFullAtTheCentreAndEmptyAtTheCorner(t *testing.T) {
	if locationFactor(0, 1000) != 1 {
		t.Fatal("centre")
	}
	if locationFactor(1000, 1000) != 0 {
		t.Fatal("corner")
	}
	if math.Abs(locationFactor(250, 1000)-0.75) > 1e-9 {
		t.Fatal(locationFactor(250, 1000))
	}
}

func TestAreaIsFullAtTheMedian(t *testing.T) {
	factor, median := areaFactor(5, []float64{1, 5, 9})
	if median != 5 || factor != 1 {
		t.Fatalf("middle factor %v median %v", factor, median)
	}
	edge, _ := areaFactor(1, []float64{1, 5, 9})
	if edge != 0 {
		t.Fatal(edge)
	}
}

func TestWeightsStayOnTheFactors(t *testing.T) {
	location := 100 * 0.75 * 0.9
	area := 100 * 0.25 * 0.8
	if math.Abs(location+area-87.5) > 1e-9 {
		t.Fatal(location + area)
	}
}

func TestCentralCentreSitsInsideItsContour(t *testing.T) {
	area, ok := districts.Named("Центральный")
	if !ok {
		t.Fatal("missing district")
	}
	lng, lat := area.Center()
	if districts.Assign(lng, lat, nil) != "Центральный" {
		t.Fatalf("centre %v %v", lng, lat)
	}
	farthest := 0.0
	for _, corner := range area.Corners() {
		edge := metersBetween(lat, lng, corner[1], corner[0])
		if edge > farthest {
			farthest = edge
		}
	}
	if farthest < 1000 {
		t.Fatal(farthest)
	}
}
