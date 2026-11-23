package assistant

import (
	"strings"
	"testing"

	"gis-backend/models"
)

func TestModeratorEditsUnownedDocument(t *testing.T) {
	user := models.User{ID: 1, Role: &models.Role{Code: models.RoleModerator}}
	if !canEdit(user, nil) {
		t.Fatal("moderator should edit a document without an author")
	}
}

func TestAuthorEditsOnlyOwnDocument(t *testing.T) {
	own := uint(4)
	other := uint(5)
	user := models.User{ID: 4, Role: &models.Role{Code: models.RoleEntrepreneur}}
	if !canEdit(user, &own) {
		t.Fatal("author should edit the document")
	}
	if canEdit(user, &other) || canEdit(user, nil) {
		t.Fatal("author should not edit a foreign document")
	}
}

func TestCleanTitle(t *testing.T) {
	title, err := cleanTitle("  Правила  ")
	if err != nil || title != "Правила" {
		t.Fatalf("got %q %v", title, err)
	}
	if _, err := cleanTitle("   "); err == nil {
		t.Fatal("expected an empty title to fail")
	}
	if _, err := cleanTitle(strings.Repeat("а", 201)); err == nil {
		t.Fatal("expected a long title to fail")
	}
}
