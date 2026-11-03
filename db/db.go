package db

import (
	"fmt"
	"log"
	"os"

	"gis-backend/config"
	"gis-backend/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func Connect() {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=Asia/Krasnoyarsk",
		config.DBHost(),
		config.DBUser(),
		config.DBPassword(),
		config.DBName(),
		config.DBPort(),
	)

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Printf("database connection failed: %v", err)
		os.Exit(1)
	}
	log.Print("database connected")
}

func Migrate() {
	if err := DB.Exec("CREATE EXTENSION IF NOT EXISTS postgis").Error; err != nil {
		log.Printf("postgis extension failed: %v", err)
		os.Exit(1)
	}

	err := DB.AutoMigrate(
		&models.Role{},
		&models.User{},
		&models.Application{},
		&models.Parcel{},
		&models.ScoreSetting{},
	)
	if err != nil {
		log.Printf("database migration failed: %v", err)
		os.Exit(1)
	}
	if err := ensureParcelBoundary(); err != nil {
		log.Printf("parcel boundary column failed: %v", err)
		os.Exit(1)
	}
	if err := seedRoles(); err != nil {
		log.Printf("role seed failed: %v", err)
		os.Exit(1)
	}
	log.Print("database migrated")
}

func ensureParcelBoundary() error {
	var udt string
	err := DB.Raw(`
		SELECT COALESCE(udt_name, '')
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'parcels' AND column_name = 'boundary'
	`).Scan(&udt).Error
	if err != nil {
		return err
	}
	switch udt {
	case "geometry":
		// Column already has the PostGIS type.
	case "":
		if err := DB.Exec(`ALTER TABLE parcels ADD COLUMN boundary geometry(Geometry, 4326)`).Error; err != nil {
			return err
		}
	default:
		if err := DB.Exec(`ALTER TABLE parcels ALTER COLUMN boundary TYPE geometry(Geometry, 4326) USING NULL`).Error; err != nil {
			return err
		}
	}
	return DB.Exec(`CREATE INDEX IF NOT EXISTS parcels_boundary_gix ON parcels USING GIST (boundary)`).Error
}

func seedRoles() error {
	roles := []models.Role{
		{Code: models.RoleEntrepreneur, Name: "Предприниматель"},
		{Code: models.RoleARN, Name: "Сотрудник АНО «Агентство развития Норильска»"},
		{Code: models.RoleUPRIU, Name: "Сотрудник Управления потребительского рынка и услуг"},
		{Code: models.RoleAdministration, Name: "Сотрудник администрации города"},
		{Code: models.RoleModerator, Name: "Модератор"},
	}
	for _, role := range roles {
		err := DB.Where(models.Role{Code: role.Code}).
			Attrs(models.Role{Name: role.Name}).
			FirstOrCreate(&role).Error
		if err != nil {
			return err
		}
	}
	return nil
}
