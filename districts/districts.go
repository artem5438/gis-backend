package districts

import "math"

// Area is one of the four Norilsk districts.
// The ring is our own contour around the known centre, longitude then latitude.
type Area struct {
	ID   string
	Name string
	Ring [][]float64
}

// Areas are the contours used to assign a district.
// A street name does not decide the district.
var Areas = []Area{
	// 16 ноября 2026. Контуры только расширены: река между Норильском и Талнахом и дорога на Кайеркан входят в обход.
	{ID: "central", Name: "Центральный", Ring: box(69.220, 87.980, 69.420, 88.340)},
	{ID: "talnakh", Name: "Талнах", Ring: box(69.410, 88.180, 69.640, 88.620)},
	{ID: "kayerkan", Name: "Кайеркан", Ring: box(69.200, 87.050, 69.430, 88.050)},
	{ID: "oganer", Name: "Оганер", Ring: box(69.250, 88.290, 69.450, 88.580)},
}

// Tile is one small rectangle inside a district, in degrees.
type Tile struct {
	South float64 `json:"s"`
	West  float64 `json:"w"`
	North float64 `json:"n"`
	East  float64 `json:"e"`
	Try   int     `json:"try"`
}

// Tiles cuts every district into rectangles of about four hundred metres.
func Tiles() []Tile {
	const dLat = 0.0036
	const dLng = 0.010
	var tiles []Tile
	for _, area := range Areas {
		south, west, north, east := bounds(area.Ring)
		for lat := south; lat < north-1e-9; lat += dLat {
			nextLat := lat + dLat
			if nextLat > north {
				nextLat = north
			}
			for lng := west; lng < east-1e-9; lng += dLng {
				nextLng := lng + dLng
				if nextLng > east {
					nextLng = east
				}
				tiles = append(tiles, Tile{South: lat, West: lng, North: nextLat, East: nextLng})
			}
		}
	}
	return tiles
}

// Ring is the closed contour of a tile.
func (tile Tile) Ring() [][]float64 {
	return box(tile.South, tile.West, tile.North, tile.East)
}

// Quarters splits a tile into four smaller ones.
func (tile Tile) Quarters() []Tile {
	midLat := (tile.South + tile.North) / 2
	midLng := (tile.West + tile.East) / 2
	return []Tile{
		{South: tile.South, West: tile.West, North: midLat, East: midLng},
		{South: tile.South, West: midLng, North: midLat, East: tile.East},
		{South: midLat, West: tile.West, North: tile.North, East: midLng},
		{South: midLat, West: midLng, North: tile.North, East: tile.East},
	}
}

// Tiny reports a rectangle that is already about twenty metres across.
func (tile Tile) Tiny() bool {
	mid := (tile.South + tile.North) / 2
	cos := math.Cos(mid * math.Pi / 180)
	if cos < 0.01 {
		cos = 0.01
	}
	height := (tile.North - tile.South) * 111320
	width := (tile.East - tile.West) * 111320 * cos
	return height < 20 && width < 20
}

// Assign picks the district whose contour holds the land plot.
// The anchor is tried first. If it falls outside, the district with the most vertices wins.
func Assign(lng, lat float64, vertices [][]float64) string {
	if name := nameAt(lng, lat); name != "" {
		return name
	}
	counts := map[string]int{}
	best := ""
	bestCount := 0
	for _, point := range vertices {
		if len(point) < 2 {
			continue
		}
		name := nameAt(point[0], point[1])
		if name == "" {
			continue
		}
		counts[name]++
		if counts[name] > bestCount {
			best = name
			bestCount = counts[name]
		}
	}
	return best
}

// Named returns the district contour with this name.
func Named(name string) (Area, bool) {
	for _, area := range Areas {
		if area.Name == name {
			return area, true
		}
	}
	return Area{}, false
}

// Center is the middle of the district contour.
func (area Area) Center() (lng, lat float64) {
	south, west, north, east := bounds(area.Ring)
	return (west + east) / 2, (south + north) / 2
}

// Corners are the four corners of the district contour, longitude then latitude.
func (area Area) Corners() [][]float64 {
	south, west, north, east := bounds(area.Ring)
	return [][]float64{
		{west, south},
		{east, south},
		{east, north},
		{west, north},
	}
}

func nameAt(lng, lat float64) string {
	for _, area := range Areas {
		if contains(area.Ring, lng, lat) {
			return area.Name
		}
	}
	return ""
}

func box(south, west, north, east float64) [][]float64 {
	return [][]float64{
		{west, south},
		{east, south},
		{east, north},
		{west, north},
		{west, south},
	}
}

func bounds(ring [][]float64) (south, west, north, east float64) {
	south, west = ring[0][1], ring[0][0]
	north, east = south, west
	for _, point := range ring {
		if point[0] < west {
			west = point[0]
		}
		if point[0] > east {
			east = point[0]
		}
		if point[1] < south {
			south = point[1]
		}
		if point[1] > north {
			north = point[1]
		}
	}
	return south, west, north, east
}

func contains(ring [][]float64, lng, lat float64) bool {
	inside := false
	j := len(ring) - 1
	for i := 0; i < len(ring); i++ {
		xi, yi := ring[i][0], ring[i][1]
		xj, yj := ring[j][0], ring[j][1]
		if (yi > lat) != (yj > lat) && lng < (xj-xi)*(lat-yi)/(yj-yi)+xi {
			inside = !inside
		}
		j = i
	}
	return inside
}
