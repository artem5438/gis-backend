package nspd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"gis-backend/config"
)

// LandPlotsIn asks the public map for every land plot inside one ring.
// The ring is closed and uses longitude, latitude in EPSG:4326.
// An empty answer is not an error. A contour with too many plots returns ErrTooBig.
func LandPlotsIn(ring [][]float64) ([]Plot, error) {
	categoryID, err := strconv.Atoi(config.NSPDCategoryID())
	if err != nil || categoryID <= 0 {
		return nil, fmt.Errorf("nspd category id")
	}
	endpoint := config.NSPDBaseURL() + "/api/geoportal/v1/intersects?typeIntersect=fullObject"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(contourBody(ring, categoryID)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", config.NSPDBaseURL()+"/map?thematic=PKK")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	body, err := getJSON(req)
	if err != nil {
		return nil, err
	}
	return ParseLandPlots(body)
}

func contourBody(ring [][]float64, categoryID int) []byte {
	payload := map[string]any{
		"categories": []map[string]int{{"id": categoryID}},
		"geom": map[string]any{
			"type": "FeatureCollection",
			"features": []any{
				map[string]any{
					"type":       "Feature",
					"properties": map[string]any{},
					"geometry": map[string]any{
						"type":        "Polygon",
						"coordinates": []any{ring},
						"crs": map[string]any{
							"type": "name",
							"properties": map[string]any{
								"name": "EPSG:4326",
							},
						},
					},
				},
			},
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return []byte("{}")
	}
	return encoded
}

// ParseLandPlots reads every land plot from one public-map answer.
func ParseLandPlots(body []byte) ([]Plot, error) {
	var collection struct {
		Features []json.RawMessage `json:"features"`
		Code     int               `json:"code"`
	}
	if err := json.Unmarshal(body, &collection); err != nil {
		return nil, err
	}
	if collection.Features == nil && collection.Code != 0 {
		return nil, fmt.Errorf("nspd code %d", collection.Code)
	}
	plots := make([]Plot, 0, len(collection.Features))
	seen := map[string]bool{}
	for _, raw := range collection.Features {
		plot, ok, _ := onePlot(raw, 0, 0)
		if !ok || seen[plot.CadastralNumber] {
			continue
		}
		seen[plot.CadastralNumber] = true
		plots = append(plots, plot)
	}
	return plots, nil
}

// Anchor is a point inside the land plot, the average of its outer ring.
func Anchor(geom json.RawMessage) (lng float64, lat float64, ok bool) {
	ring := outerRing(geom)
	if len(ring) == 0 {
		return 0, 0, false
	}
	var sumLng, sumLat float64
	count := 0
	for i, point := range ring {
		if i == len(ring)-1 && point[0] == ring[0][0] && point[1] == ring[0][1] {
			continue
		}
		sumLng += point[0]
		sumLat += point[1]
		count++
	}
	if count == 0 {
		return 0, 0, false
	}
	return sumLng / float64(count), sumLat / float64(count), true
}

// Vertices lists the outer ring of a land plot, longitude then latitude.
func Vertices(geom json.RawMessage) [][]float64 {
	return outerRing(geom)
}

func outerRing(geom json.RawMessage) [][]float64 {
	if len(geom) == 0 {
		return nil
	}
	var raw map[string]any
	if json.Unmarshal(geom, &raw) != nil {
		return nil
	}
	kind, _ := raw["type"].(string)
	coords, _ := raw["coordinates"].([]any)
	switch kind {
	case "Polygon":
		rings := polygonRings(coords)
		if len(rings) > 0 {
			return rings[0]
		}
	case "MultiPolygon":
		for _, item := range coords {
			rings := polygonRings(asArray(item))
			if len(rings) > 0 {
				return rings[0]
			}
		}
	}
	return nil
}
