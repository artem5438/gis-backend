package parcels

import (
	"net/url"
	"testing"
)

func TestDrawWindowReadsTheWholeCityWhenTheQueryIsEmpty(t *testing.T) {
	box, simplify, err := drawWindow(url.Values{})
	if err != nil || box != nil || simplify != 0 {
		t.Fatalf("empty query = %v %v %v", box, simplify, err)
	}
}

func TestDrawWindowRejectsAPartialRectangle(t *testing.T) {
	_, _, err := drawWindow(url.Values{"south": {"69"}, "west": {"88"}})
	if err == nil {
		t.Fatal("partial window was accepted")
	}
}

func TestDrawWindowKeepsANorilskRectangle(t *testing.T) {
	box, simplify, err := drawWindow(url.Values{
		"south":    {"69.34"},
		"west":     {"88.10"},
		"north":    {"69.36"},
		"east":     {"88.20"},
		"simplify": {"0.0002"},
	})
	if err != nil || box == nil {
		t.Fatal(err)
	}
	if box.south != 69.34 || box.east != 88.20 || simplify != 0.0002 {
		t.Fatalf("window = %+v %v", box, simplify)
	}
}

func TestDrawWindowCapsAHugeSimplify(t *testing.T) {
	_, simplify, err := drawWindow(url.Values{
		"south":    {"69"},
		"west":     {"88"},
		"north":    {"70"},
		"east":     {"89"},
		"simplify": {"5"},
	})
	if err != nil || simplify != 0.02 {
		t.Fatalf("simplify = %v %v", simplify, err)
	}
}
