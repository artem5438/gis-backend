package nspd

import (
	"encoding/json"
	"math"
	"testing"
)

func TestIntersectsBodyKeepsTheClickInsideASmallRing(t *testing.T) {
	const lat, lng = 69.35, 88.20
	var body struct {
		Categories []struct {
			ID int `json:"id"`
		} `json:"categories"`
		Geom struct {
			Features []struct {
				Geometry struct {
					Coordinates [][][]float64 `json:"coordinates"`
					CRS         struct {
						Properties struct {
							Name string `json:"name"`
						} `json:"properties"`
					} `json:"crs"`
				} `json:"geometry"`
			} `json:"features"`
		} `json:"geom"`
	}
	if err := json.Unmarshal(intersectsBody(lat, lng, 36368), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Categories) != 1 || body.Categories[0].ID != 36368 {
		t.Fatalf("category %#v", body.Categories)
	}
	if body.Geom.Features[0].Geometry.CRS.Properties.Name != "EPSG:4326" {
		t.Fatal(body.Geom.Features[0].Geometry.CRS.Properties.Name)
	}
	ring := body.Geom.Features[0].Geometry.Coordinates[0]
	if len(ring) != 5 || ring[0][0] != ring[4][0] || ring[0][1] != ring[4][1] {
		t.Fatalf("ring %#v", ring)
	}
	if ring[2][0]-ring[0][0] > 0.001 || ring[2][1]-ring[0][1] > 0.001 {
		t.Fatalf("ring is wider than a click: %#v", ring)
	}
	geom := map[string]any{"type": "Polygon", "coordinates": []any{toAny(ring)}}
	if !geometryContains(geom, lng, lat) {
		t.Fatal("click is outside the query ring")
	}
}

func toAny(ring [][]float64) []any {
	points := make([]any, len(ring))
	for i, pair := range ring {
		points[i] = []any{pair[0], pair[1]}
	}
	return points
}

func TestParseLandPlotPrefersThePolygonUnderThePoint(t *testing.T) {
	body := []byte(`{
		"type": "FeatureCollection",
		"features": [
			{
				"type": "Feature",
				"geometry": {"type": "Polygon", "coordinates": [[[0, 0], [1, 0], [1, 1], [0, 1], [0, 0]]]},
				"properties": {"options": {
					"cad_num": "00:00:0000000:1",
					"specified_area": 25000,
					"area": 1,
					"land_record_category_type": "Земли населённых пунктов",
					"permitted_use_established_by_document": "Для индивидуальной жилой застройки"
				}}
			},
			{
				"type": "Feature",
				"geometry": {"type": "Polygon", "coordinates": [[[10, 10], [11, 10], [11, 11], [10, 11], [10, 10]]]},
				"properties": {"options": {"cad_num": "00:00:0000000:2", "area": 100}}
			}
		]
	}`)
	plot, err := ParseLandPlot(body, 10.5, 10.5)
	if err != nil {
		t.Fatal(err)
	}
	if plot.CadastralNumber != "00:00:0000000:2" {
		t.Fatalf("number %s", plot.CadastralNumber)
	}
}

func TestParseLandPlotConvertsMercatorAndArea(t *testing.T) {
	body := []byte(`{
		"features": [{
			"type": "Feature",
			"geometry": {
				"type": "Polygon",
				"crs": {"type": "name", "properties": {"name": "EPSG:3857"}},
				"coordinates": [[
					[9800000, 10500000],
					[9801000, 10500000],
					[9801000, 10501000],
					[9800000, 10501000],
					[9800000, 10500000]
				]]
			},
			"properties": {"options": {
				"cad_num": "24:00:0000000:5",
				"declared_area": 10000,
				"ownership_type": "Частная"
			}}
		}]
	}`)
	plot, err := ParseLandPlot(body, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if plot.AreaHa == nil || math.Abs(*plot.AreaHa-1) > 0.0001 {
		t.Fatalf("area %#v", plot.AreaHa)
	}
	if plot.CadastralNumber != "24:00:0000000:5" {
		t.Fatal(plot.CadastralNumber)
	}
	lng, lat := mercatorToLonLat(9800500, 10500500)
	again, err := ParseLandPlot(body, lat, lng)
	if err != nil {
		t.Fatal(err)
	}
	if !geometryContains(mustGeom(t, again.Geometry), lng, lat) {
		t.Fatal("converted polygon does not contain its center")
	}
}

func TestParseLandPlotReadsThePublicCard(t *testing.T) {
	body := []byte(`{
		"features": [{
			"type": "Feature",
			"geometry": {"type": "Polygon", "coordinates": [[[88.1, 69.3], [88.2, 69.3], [88.2, 69.4], [88.1, 69.4], [88.1, 69.3]]]},
			"properties": {"options": {
				"cad_num": "24:55:0202003:130",
				"land_record_type": "Земельный участок",
				"land_record_subtype": "Землепользование",
				"land_record_reg_date": "2009-12-18T00:00:00Z",
				"quarter_cad_number": "24:55:0202003",
				"readable_address": "г Норильск, ул Таймырская",
				"specified_area": 2009,
				"status": "Учтенный",
				"land_record_category_type": "Земли населенных пунктов",
				"permitted_use_established_by_document": "Для эксплуатации многоквартирного жилого дома",
				"ownership_type": "Частная",
				"cost_value": 4442361.07,
				"cost_index": 2211.23
			}}
		}]
	}`)
	plot, err := ParseLandPlot(body, 69.35, 88.15)
	if err != nil {
		t.Fatal(err)
	}
	if plot.AssignedOn != "18.12.2009" || plot.AreaName != "уточнённая" || plot.AreaM2 == nil || *plot.AreaM2 != 2009 {
		t.Fatalf("%#v", plot)
	}
	if plot.OwnershipForm != "Частная" || plot.CadastralCost == nil || *plot.CadastralCost != 4442361.07 {
		t.Fatalf("%#v", plot)
	}
	if plot.ParcelKind != "Землепользование" || plot.CadastralQuarter != "24:55:0202003" {
		t.Fatalf("%#v", plot)
	}
}

func TestParseLandPlotReadsTheNumberFromDescr(t *testing.T) {
	body := []byte(`{
		"features": [{
			"type": "Feature",
			"geometry": {"type": "Polygon", "coordinates": [[[88.1, 69.3], [88.2, 69.3], [88.2, 69.4], [88.1, 69.4], [88.1, 69.3]]]},
			"properties": {"descr": "24:55:0202003:130", "options": {"specified_area": 2009}}
		}]
	}`)
	plot, err := ParseLandPlot(body, 69.35, 88.15)
	if err != nil {
		t.Fatal(err)
	}
	if plot.CadastralNumber != "24:55:0202003:130" {
		t.Fatal(plot.CadastralNumber)
	}
}

func TestParseLandPlotRejectsServerCode(t *testing.T) {
	_, err := ParseLandPlot([]byte(`{"code":400104,"message":"too big"}`), 69.3, 88.2)
	if err == nil || errorsIsEmpty(err) {
		t.Fatal(err)
	}
}

func TestParseLandPlotEmpty(t *testing.T) {
	_, err := ParseLandPlot([]byte(`{"type":"FeatureCollection","features":[]}`), 69.3, 88.2)
	if !errorsIsEmpty(err) {
		t.Fatal(err)
	}
}

func errorsIsEmpty(err error) bool {
	return err == ErrEmpty
}

func mustGeom(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var geom map[string]any
	if err := json.Unmarshal(raw, &geom); err != nil {
		t.Fatal(err)
	}
	return geom
}
