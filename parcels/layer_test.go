package parcels

import (
	"net/url"
	"testing"
)

func TestDrawWindowReadsTheWholeCityWhenTheQueryIsEmpty(t *testing.T) {
	box, simplify, squares, err := drawWindow(url.Values{})
	if err != nil || box != nil || simplify != 0 || squares {
		t.Fatalf("empty query = %v %v %v %v", box, simplify, squares, err)
	}
}

func TestDrawWindowRejectsAPartialRectangle(t *testing.T) {
	_, _, _, err := drawWindow(url.Values{"south": {"69"}, "west": {"88"}})
	if err == nil {
		t.Fatal("partial window was accepted")
	}
}

func TestDrawWindowKeepsANorilskRectangle(t *testing.T) {
	box, simplify, squares, err := drawWindow(url.Values{
		"south":    {"69.34"},
		"west":     {"88.10"},
		"north":    {"69.36"},
		"east":     {"88.20"},
		"simplify": {"0.0002"},
	})
	if err != nil || box == nil {
		t.Fatal(err)
	}
	if box.south != 69.34 || box.east != 88.20 || simplify != 0.0002 || squares {
		t.Fatalf("window = %+v %v %v", box, simplify, squares)
	}
}

func TestDrawWindowCapsAHugeSimplify(t *testing.T) {
	_, simplify, squares, err := drawWindow(url.Values{
		"south":    {"69"},
		"west":     {"88"},
		"north":    {"70"},
		"east":     {"89"},
		"simplify": {"5"},
	})
	if err != nil || simplify != 0.02 || squares {
		t.Fatalf("simplify = %v %v %v", simplify, squares, err)
	}
}

func TestDrawWindowSquaresTheWholeCity(t *testing.T) {
	box, _, squares, err := drawWindow(url.Values{"squares": {"1"}})
	if err != nil || box != nil || !squares {
		t.Fatalf("whole city squares = %v %v %v", box, squares, err)
	}
}

func TestDrawWindowAsksForSquares(t *testing.T) {
	_, _, squares, err := drawWindow(url.Values{
		"south":   {"69.30"},
		"west":    {"88.10"},
		"north":   {"69.40"},
		"east":    {"88.30"},
		"squares": {"1"},
	})
	if err != nil || !squares {
		t.Fatalf("squares = %v %v", squares, err)
	}
}
