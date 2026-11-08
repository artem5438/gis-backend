package parcels

import (
	"encoding/json"
	"testing"

	"gis-backend/db"
	"gis-backend/nspd"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestCatalogKeepsDistrictWhenTheClickUpdatesTheRow(t *testing.T) {
	conn, err := gorm.Open(postgres.Open("host=localhost user=admin password=secret dbname=norilsk_gis port=5432 sslmode=disable"), &gorm.Config{})
	if err != nil {
		t.Skip(err)
	}
	sqlDB, err := conn.DB()
	if err != nil {
		t.Skip(err)
	}
	if err = sqlDB.Ping(); err != nil {
		t.Skip(err)
	}
	db.DB = conn

	area := 1.5
	meters := 15000.0
	plot := nspd.Plot{
		CadastralNumber: "00:00:0000000:6",
		AreaHa:          &area,
		AreaM2:          &meters,
		Status:          "Учтенный",
		LandCategory:    "Земли населенных пунктов",
		PermittedUse:    "Для каталога",
		Geometry:        json.RawMessage(`{"type":"Polygon","coordinates":[[[88.2,69.35],[88.201,69.35],[88.201,69.351],[88.2,69.351],[88.2,69.35]]]}`),
	}
	if err = saveCatalogPlot(plot); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.DB.Exec("DELETE FROM parcels WHERE cadastral_number = ?", plot.CadastralNumber).Error
	})
	district, inCatalog := readPlace(t, plot.CadastralNumber)
	if district != "Центральный" || !inCatalog {
		t.Fatalf("district %q catalog %v", district, inCatalog)
	}

	plot.Status = "Снят с учета"
	if _, err = savePlot(plot); err != nil {
		t.Fatal(err)
	}
	district, inCatalog = readPlace(t, plot.CadastralNumber)
	if district != "Центральный" || !inCatalog {
		t.Fatalf("click cleared district %q catalog %v", district, inCatalog)
	}

	var covers bool
	if err = db.DB.Raw(`SELECT ST_Covers(boundary, ST_SetSRID(ST_MakePoint(88.2004, 69.3504), 4326)) FROM parcels WHERE cadastral_number = ?`, plot.CadastralNumber).Scan(&covers).Error; err != nil || !covers {
		t.Fatalf("test parcel does not cover the point: %v %v", covers, err)
	}
	card, found, err := readStoredParcel(69.3504, 88.2004)
	if err != nil || !found || card.CadastralNumber == "" {
		t.Fatalf("stored card found=%v err=%v %+v", found, err, card)
	}
	miss, found, err := readStoredParcel(60, 30)
	if err != nil || found || miss.ID != 0 {
		t.Fatalf("outside point found=%v err=%v", found, err)
	}
}

func readPlace(t *testing.T, number string) (string, bool) {
	t.Helper()
	var row struct {
		District  string
		InCatalog bool
	}
	err := db.DB.Raw("SELECT district, in_catalog FROM parcels WHERE cadastral_number = ?", number).Scan(&row).Error
	if err != nil {
		t.Fatal(err)
	}
	return row.District, row.InCatalog
}
