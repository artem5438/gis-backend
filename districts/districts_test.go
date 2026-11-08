package districts

import "testing"

func TestCentresFallIntoTheirOwnContour(t *testing.T) {
	centres := map[string][2]float64{
		"Центральный": {88.201, 69.349},
		"Талнах":      {88.397, 69.486},
		"Кайеркан":    {87.765, 69.35},
		"Оганер":      {88.368, 69.359},
	}
	for name, point := range centres {
		if got := Assign(point[0], point[1], nil); got != name {
			t.Fatalf("%s assigned %q", name, got)
		}
	}
}

func TestTalnakhReachesTheOctoberMine(t *testing.T) {
	// Рудник «Октябрьский», основная площадка: 69°31'32"N 88°20'15"E.
	if got := Assign(88.3375, 69.5256, nil); got != "Талнах" {
		t.Fatal(got)
	}
}

func TestStreetNameDoesNotAssignADistrict(t *testing.T) {
	if got := Assign(88.201, 69.349, nil); got != "Центральный" {
		t.Fatal(got)
	}
	if got := Assign(30, 60, nil); got != "" {
		t.Fatal(got)
	}
}

func TestQuartersCoverTheTile(t *testing.T) {
	tile := Tile{South: 69.34, West: 88.20, North: 69.35, East: 88.22}
	parts := tile.Quarters()
	if len(parts) != 4 {
		t.Fatal(len(parts))
	}
	if parts[0].South != tile.South || parts[3].North != tile.North || parts[1].East != tile.East {
		t.Fatalf("%#v", parts)
	}
}

func TestTilesStayInsideTheDistricts(t *testing.T) {
	tiles := Tiles()
	if len(tiles) < 100 {
		t.Fatalf("tiles %d", len(tiles))
	}
	for _, tile := range tiles {
		if tile.North <= tile.South || tile.East <= tile.West {
			t.Fatalf("%#v", tile)
		}
		if tile.Tiny() {
			t.Fatalf("starting tile is tiny %#v", tile)
		}
	}
}
