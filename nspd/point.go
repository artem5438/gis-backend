package nspd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gis-backend/config"
)

// ErrEmpty means the public map answered and there is no land plot at the point.
var ErrEmpty = errors.New("nspd: no land plot")

// Plot is one land plot taken from the public cadastral map.
// Geometry is GeoJSON in EPSG:4326, or nil when the plot has no boundary.
type Plot struct {
	CadastralNumber  string
	ObjectKind       string
	ParcelKind       string
	AssignedOn       string
	CadastralQuarter string
	Address          string
	AreaHa           *float64
	AreaM2           *float64
	AreaName         string
	Status           string
	LandCategory     string
	PermittedUse     string
	OwnershipForm    string
	CadastralCost    *float64
	CostIndex        *float64
	Geometry         json.RawMessage
}

// LandPlotAt asks the public map which land plot covers one click.
// The map answers with the plot at that place, so another click finds another plot.
func LandPlotAt(lat, lng float64) (Plot, error) {
	categoryID, err := strconv.Atoi(config.NSPDCategoryID())
	if err != nil || categoryID <= 0 {
		return Plot{}, fmt.Errorf("nspd category id")
	}
	payload := intersectsBody(lat, lng, categoryID)
	endpoint := config.NSPDBaseURL() + "/api/geoportal/v1/intersects?typeIntersect=fullObject"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return Plot{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", config.NSPDBaseURL()+"/map?thematic=PKK")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	body, err := getJSON(req)
	if err != nil {
		return Plot{}, err
	}
	return ParseLandPlot(body, lat, lng)
}

// intersectsBody is one small square around the click, a few meters wide.
// Category 36368 is every land plot, and the answer names the plot under the square.
func intersectsBody(lat, lng float64, categoryID int) []byte {
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
						"coordinates": []any{pointRing(lat, lng)},
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

func pointRing(lat, lng float64) [][]float64 {
	const meters = 4.0
	dLat := meters / 111320
	cos := math.Cos(lat * math.Pi / 180)
	if cos < 0.01 {
		cos = 0.01
	}
	dLng := meters / (111320 * cos)
	return [][]float64{
		{lng - dLng, lat - dLat},
		{lng + dLng, lat - dLat},
		{lng + dLng, lat + dLat},
		{lng - dLng, lat + dLat},
		{lng - dLng, lat - dLat},
	}
}

func getJSON(req *http.Request) ([]byte, error) {
	client := httpClient()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	payload, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	_ = resp.Body.Close()
	if readErr != nil {
		return nil, readErr
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		time.Sleep(time.Second)
		resp, err = client.Do(req.Clone(req.Context()))
		if err != nil {
			return nil, err
		}
		payload, readErr = io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrEmpty
	}
	if resp.StatusCode != http.StatusOK {
		snippet := payload
		if len(snippet) > 180 {
			snippet = snippet[:180]
		}
		return nil, fmt.Errorf("nspd status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	if len(payload) == 0 || payload[0] == '<' {
		return nil, fmt.Errorf("nspd returned no json")
	}
	return payload, nil
}

// ParseLandPlot reads a GeoJSON feature collection and keeps the plot under the point.
func ParseLandPlot(body []byte, lat, lng float64) (Plot, error) {
	var collection struct {
		Features []json.RawMessage `json:"features"`
		Code     int               `json:"code"`
	}
	if err := json.Unmarshal(body, &collection); err != nil {
		return Plot{}, err
	}
	if collection.Features == nil && collection.Code != 0 {
		return Plot{}, fmt.Errorf("nspd code %d", collection.Code)
	}
	var chosen Plot
	found := false
	contains := false
	for _, raw := range collection.Features {
		plot, ok, hit := onePlot(raw, lat, lng)
		if !ok {
			continue
		}
		if !found || (hit && !contains) {
			chosen = plot
			found = true
			contains = hit
		}
		if contains && hit {
			break
		}
	}
	if !found {
		return Plot{}, ErrEmpty
	}
	return chosen, nil
}

func onePlot(raw json.RawMessage, lat, lng float64) (Plot, bool, bool) {
	var feature struct {
		Geometry   map[string]any `json:"geometry"`
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(raw, &feature); err != nil || feature.Properties == nil {
		return Plot{}, false, false
	}
	opts := feature.Properties
	if nested, ok := feature.Properties["options"].(map[string]any); ok {
		opts = nested
	}
	number := cadastralNumber(text(opts, "cad_num"))
	if number == "" {
		number = cadastralNumber(text(feature.Properties, "descr"))
	}
	if number == "" {
		number = cadastralNumber(text(feature.Properties, "externalKey"))
	}
	if number == "" {
		return Plot{}, false, false
	}
	plot := Plot{
		CadastralNumber:  clip(number, 40),
		ObjectKind:       clip(text(opts, "land_record_type"), 200),
		ParcelKind:       clip(text(opts, "land_record_subtype"), 200),
		AssignedOn:       assignedOn(text(opts, "land_record_reg_date")),
		CadastralQuarter: clip(cadastralNumber(text(opts, "quarter_cad_number")), 40),
		Address:          clip(text(opts, "readable_address"), 1000),
		Status:           clip(text(opts, "status"), 80),
		LandCategory:     clip(text(opts, "land_record_category_type"), 300),
		PermittedUse:     clip(text(opts, "permitted_use_established_by_document"), 500),
		OwnershipForm:    clip(text(opts, "ownership_type"), 80),
		CadastralCost:    positive(opts, "cost_value"),
		CostIndex:        positive(opts, "cost_index"),
	}
	if meters, name := landArea(opts); meters != nil {
		hectares := *meters / 10000
		plot.AreaHa = &hectares
		plot.AreaM2 = meters
		plot.AreaName = name
	}
	hit := false
	if feature.Geometry != nil {
		kind, _ := feature.Geometry["type"].(string)
		if kind == "Polygon" || kind == "MultiPolygon" {
			normalizeGeometry(feature.Geometry)
			hit = geometryContains(feature.Geometry, lng, lat)
			if encoded, err := json.Marshal(feature.Geometry); err == nil {
				plot.Geometry = encoded
			}
		}
	}
	return plot, true, hit
}

func landArea(opts map[string]any) (*float64, string) {
	for _, item := range []struct{ key, name string }{
		{"specified_area", "уточнённая"},
		{"declared_area", "декларированная"},
		{"area", ""},
	} {
		meters, ok := asFloat(opts[item.key])
		if ok && meters > 0 {
			value := meters
			return &value, item.name
		}
	}
	return nil, ""
}

func positive(opts map[string]any, key string) *float64 {
	value, ok := asFloat(opts[key])
	if !ok || value <= 0 {
		return nil
	}
	return &value
}

func assignedOn(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.Format("02.01.2006")
		}
	}
	return clip(value, 40)
}

func text(opts map[string]any, key string) string {
	value, _ := opts[key].(string)
	return strings.TrimSpace(value)
}

func cadastralNumber(value string) string {
	value = strings.TrimSpace(value)
	if strings.Count(value, ":") < 2 {
		return ""
	}
	for _, r := range value {
		if r != ':' && (r < '0' || r > '9') {
			return ""
		}
	}
	return value
}

func clip(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

func normalizeGeometry(geom map[string]any) {
	name := crsName(geom)
	delete(geom, "crs")
	if name == "EPSG:4326" {
		return
	}
	if name != "EPSG:3857" && name != "EPSG:900913" && !coordsLookProjected(geom["coordinates"]) {
		return
	}
	transformCoords(geom["coordinates"], mercatorToLonLat)
}

func crsName(geom map[string]any) string {
	crs, _ := geom["crs"].(map[string]any)
	props, _ := crs["properties"].(map[string]any)
	name, _ := props["name"].(string)
	return strings.ToUpper(strings.TrimSpace(name))
}

func coordsLookProjected(value any) bool {
	items, ok := value.([]any)
	if !ok {
		return false
	}
	if len(items) >= 2 {
		if x, okx := asFloat(items[0]); okx {
			if y, oky := asFloat(items[1]); oky {
				return math.Abs(x) > 180 || math.Abs(y) > 90
			}
		}
	}
	for _, item := range items {
		if coordsLookProjected(item) {
			return true
		}
	}
	return false
}

func transformCoords(value any, convert func(x, y float64) (float64, float64)) {
	items, ok := value.([]any)
	if !ok {
		return
	}
	if len(items) >= 2 {
		if x, okx := asFloat(items[0]); okx {
			if y, oky := asFloat(items[1]); oky {
				nx, ny := convert(x, y)
				items[0], items[1] = nx, ny
				return
			}
		}
	}
	for _, item := range items {
		transformCoords(item, convert)
	}
}

func mercatorToLonLat(x, y float64) (float64, float64) {
	const origin = 20037508.342789244
	lng := x / origin * 180
	lat := math.Atan(math.Sinh(y/origin*math.Pi)) * 180 / math.Pi
	return lng, lat
}

func geometryContains(geom map[string]any, lng, lat float64) bool {
	kind, _ := geom["type"].(string)
	coords, _ := geom["coordinates"].([]any)
	switch kind {
	case "Polygon":
		rings := polygonRings(coords)
		return len(rings) > 0 && ringContains(rings[0], lng, lat)
	case "MultiPolygon":
		for _, item := range coords {
			rings := polygonRings(asArray(item))
			if len(rings) > 0 && ringContains(rings[0], lng, lat) {
				return true
			}
		}
	}
	return false
}

func polygonRings(coords []any) [][][]float64 {
	rings := make([][][]float64, 0, len(coords))
	for _, item := range coords {
		ring := make([][]float64, 0)
		for _, pos := range asArray(item) {
			pair := asArray(pos)
			if len(pair) < 2 {
				continue
			}
			x, okx := asFloat(pair[0])
			y, oky := asFloat(pair[1])
			if okx && oky {
				ring = append(ring, []float64{x, y})
			}
		}
		if len(ring) >= 3 {
			rings = append(rings, ring)
		}
	}
	return rings
}

func ringContains(ring [][]float64, lng, lat float64) bool {
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

func asArray(value any) []any {
	items, _ := value.([]any)
	return items
}

func asFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, !math.IsNaN(typed)
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(typed), ",", "."), 64)
		return parsed, err == nil && !math.IsNaN(parsed)
	}
	return 0, false
}
