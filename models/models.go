package models

import "time"

const (
	RoleEntrepreneur   = "entrepreneur"
	RoleARN            = "arn"
	RoleUPRIU          = "upriu"
	RoleAdministration = "administration"
	RoleModerator      = "moderator"

	StatusVisitor   = "visitor"
	DecisionPending = "pending"
)

// Role is an application role, not a PostgreSQL login.
type Role struct {
	ID   uint   `gorm:"primaryKey"`
	Code string `gorm:"uniqueIndex;not null;size:40"`
	Name string `gorm:"not null;size:200"`
}

// User holds the account. Personal columns exist now; the site fills only the full name and password hash.
type User struct {
	ID                 uint   `gorm:"primaryKey"`
	FullName           string `gorm:"uniqueIndex;not null;size:300"`
	PasswordHash       string `gorm:"not null;size:255"`
	Phone              string `gorm:"size:40"`
	Email              string `gorm:"size:200"`
	VerificationStatus string `gorm:"not null;size:40;default:visitor"`
	RoleID             *uint
	Role               *Role
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Application keeps the verification form. The site does not fill these columns yet.
type Application struct {
	ID             uint `gorm:"primaryKey"`
	UserID         uint `gorm:"not null;index"`
	User           User
	FullName       string `gorm:"size:300"`
	Phone          string `gorm:"size:40"`
	Email          string `gorm:"size:200"`
	Organization   string `gorm:"size:300"`
	INN            string `gorm:"size:12"`
	Position       string `gorm:"size:200"`
	Body           string `gorm:"type:text"`
	FileKey        string `gorm:"size:500"`
	Decision       string `gorm:"not null;size:40;default:pending"`
	AssignedRoleID *uint
	AssignedRole   *Role
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Parcel is one cadastral land plot. Boundary is a PostGIS geometry in EPSG:4326.
type Parcel struct {
	ID              uint   `gorm:"primaryKey"`
	CadastralNumber string `gorm:"uniqueIndex;not null;size:40"`
	AreaHa          *float64
	LandCategory    string  `gorm:"size:300"`
	PermittedUse    string  `gorm:"size:500"`
	District        string  `gorm:"size:40"`
	Direction       string  `gorm:"size:40"`
	Boundary        *string `gorm:"type:geometry(Geometry,4326)"`
	InCatalog       bool    `gorm:"not null;default:false"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ScoreSetting stores weights and the working area range for one direction.
// Rows are inserted when the score itself is implemented.
type ScoreSetting struct {
	ID                 uint   `gorm:"primaryKey"`
	Direction          string `gorm:"uniqueIndex;not null;size:40"`
	AreaMinHa          float64
	AreaMaxHa          float64
	WeightLandCategory float64
	WeightArea         float64
	WeightPermittedUse float64
}
