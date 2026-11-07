package parcels

import (
	"encoding/json"
	"testing"

	"gis-backend/db"
	"gis-backend/nspd"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestUpsertKeepsTheSameRow(t *testing.T) {
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

	area := 1.25
	meters := 12500.0
	plot := nspd.Plot{
		CadastralNumber: "00:00:0000000:8",
		AreaHa:          &area,
		AreaM2:          &meters,
		AreaName:        "уточнённая",
		Status:          "Учтенный",
		OwnershipForm:   "Частная",
		LandCategory:    "Земли населённых пунктов",
		PermittedUse:    "Для проверки",
		Geometry:        json.RawMessage(`{"type":"Polygon","coordinates":[[[88.2,69.35],[88.201,69.35],[88.201,69.351],[88.2,69.351],[88.2,69.35]]]}`),
	}
	first, err := savePlot(plot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.DB.Exec("DELETE FROM parcels WHERE cadastral_number = ?", plot.CadastralNumber).Error
	})
	area = 2.5
	plot.AreaHa = &area
	plot.Status = "Снят с учета"
	plot.PermittedUse = "Повторный щелчок"
	second, err := savePlot(plot)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("id changed %d -> %d", first.ID, second.ID)
	}
	if second.AreaHa == nil || *second.AreaHa != 2.5 {
		t.Fatalf("area %#v", second.AreaHa)
	}
	if second.Status != "Снят с учета" || second.OwnershipForm != "Частная" {
		t.Fatalf("%+v", second)
	}
}
