package nspd

import "testing"

func TestParseLandPlotsKeepsEveryPlot(t *testing.T) {
	body := []byte(`{"type":"FeatureCollection","features":[
		{"type":"Feature","geometry":{"type":"Polygon","coordinates":[[[88.2,69.35],[88.201,69.35],[88.201,69.351],[88.2,69.351],[88.2,69.35]]]},"properties":{"options":{"cad_num":"24:55:0000000:1","land_record_category_type":"Земли населенных пунктов"}}},
		{"type":"Feature","geometry":{"type":"Polygon","coordinates":[[[88.21,69.35],[88.212,69.35],[88.212,69.352],[88.21,69.352],[88.21,69.35]]]},"properties":{"options":{"cad_num":"24:55:0000000:2","land_record_category_type":"Земли лесного фонда"}}},
		{"type":"Feature","geometry":{"type":"Polygon","coordinates":[[[88.2,69.35],[88.201,69.35],[88.201,69.351],[88.2,69.351],[88.2,69.35]]]},"properties":{"options":{"cad_num":"24:55:0000000:1"}}}
	]}`)
	plots, err := ParseLandPlots(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(plots) != 2 {
		t.Fatalf("%d plots", len(plots))
	}
	if plots[0].CadastralNumber != "24:55:0000000:1" || plots[1].LandCategory != "Земли лесного фонда" {
		t.Fatalf("%+v", plots)
	}
	lng, lat, ok := Anchor(plots[0].Geometry)
	if !ok || lng < 88.2 || lng > 88.201 || lat < 69.35 || lat > 69.351 {
		t.Fatalf("anchor %v %v %v", lng, lat, ok)
	}
}

func TestTooBigContour(t *testing.T) {
	if !tooBig(500, []byte(`{"code":400004}`)) {
		t.Fatal("expected too big")
	}
	if tooBig(500, []byte(`{"code":1}`)) || tooBig(200, []byte(`{"code":400004}`)) {
		t.Fatal("unexpected too big")
	}
}
