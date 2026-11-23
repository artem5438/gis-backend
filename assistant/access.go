package assistant

import (
	"fmt"
	"path"
	"strings"
	"unicode/utf8"

	"gis-backend/models"
)

func canEdit(user models.User, ownerID *uint) bool {
	if user.Role != nil && user.Role.Code == models.RoleModerator {
		return true
	}
	return ownerID != nil && *ownerID == user.ID
}

func cleanTitle(value string) (string, error) {
	title := strings.TrimSpace(value)
	if title == "" {
		return "", fmt.Errorf("нужно название")
	}
	if utf8.RuneCountInString(title) > 200 {
		return "", fmt.Errorf("название длиннее 200 знаков")
	}
	return title, nil
}

func titleFromFilename(name string) string {
	title := strings.TrimSpace(path.Base(name))
	if title == "" || title == "." || title == "/" {
		title = "документ"
	}
	runes := []rune(title)
	if len(runes) > 200 {
		title = string(runes[:200])
	}
	return title
}
