package applications

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gis-backend/auth"
	"gis-backend/db"
	"gis-backend/models"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

type publicRole struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type publicApplication struct {
	ID           uint        `json:"id"`
	Body         string      `json:"body"`
	Decision     string      `json:"decision"`
	CreatedAt    time.Time   `json:"created_at"`
	FullName     string      `json:"full_name,omitempty"`
	Login        string      `json:"login,omitempty"`
	AssignedRole *publicRole `json:"assigned_role"`
}

func Create(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.RequireUser(w, r)
	if !ok {
		return
	}
	if user.RoleID != nil {
		writeError(w, http.StatusConflict, "роль уже назначена")
		return
	}

	var body struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "некорректный JSON")
		return
	}
	text := strings.TrimSpace(body.Body)
	if utf8.RuneCountInString(text) < 10 || utf8.RuneCountInString(text) > 4000 {
		writeError(w, http.StatusBadRequest, "напишите текст заявки, от 10 символов")
		return
	}

	var pending int64
	if err := db.DB.Model(&models.Application{}).
		Where("user_id = ? AND decision = ?", user.ID, models.DecisionPending).
		Count(&pending).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось проверить заявку")
		return
	}
	if pending > 0 {
		writeError(w, http.StatusConflict, "заявка уже на рассмотрении")
		return
	}

	application := models.Application{
		UserID:   user.ID,
		Body:     text,
		Decision: models.DecisionPending,
	}
	if err := db.DB.Create(&application).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось отправить заявку")
		return
	}
	writeJSON(w, http.StatusCreated, toPublic(application, false))
}

func Mine(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.RequireUser(w, r)
	if !ok {
		return
	}
	var application models.Application
	err := db.DB.Preload("AssignedRole").
		Where("user_id = ?", user.ID).
		Order("created_at desc").
		First(&application).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"application": nil})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось прочитать заявку")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"application": toPublic(application, false)})
}

func Queue(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireModerator(w, r); !ok {
		return
	}
	var applications []models.Application
	err := db.DB.Preload("User").
		Where("decision = ?", models.DecisionPending).
		Order("created_at asc").
		Find(&applications).Error
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось прочитать очередь")
		return
	}
	var roles []models.Role
	if err := db.DB.Order("id").Find(&roles).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось прочитать роли")
		return
	}
	items := make([]publicApplication, 0, len(applications))
	for _, application := range applications {
		items = append(items, toPublic(application, true))
	}
	roleItems := make([]publicRole, 0, len(roles))
	for _, role := range roles {
		roleItems = append(roleItems, publicRole{Code: role.Code, Name: role.Name})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"applications": items,
		"roles":        roleItems,
	})
}

func Decide(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireModerator(w, r); !ok {
		return
	}
	id, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id == 0 {
		writeError(w, http.StatusBadRequest, "некорректная заявка")
		return
	}
	var body struct {
		Decision string `json:"decision"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "некорректный JSON")
		return
	}

	var application models.Application
	err = db.DB.First(&application, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(w, http.StatusNotFound, "заявка не найдена")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось прочитать заявку")
		return
	}
	if application.Decision != models.DecisionPending {
		writeError(w, http.StatusConflict, "заявка уже рассмотрена")
		return
	}

	switch body.Decision {
	case models.DecisionApproved:
		var role models.Role
		if err := db.DB.Where("code = ?", strings.TrimSpace(body.Role)).First(&role).Error; err != nil {
			writeError(w, http.StatusBadRequest, "выберите одну роль")
			return
		}
		err = db.DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&models.User{}).Where("id = ?", application.UserID).Updates(map[string]any{
				"role_id":             role.ID,
				"verification_status": models.StatusVerified,
			}).Error; err != nil {
				return err
			}
			return tx.Model(&application).Updates(map[string]any{
				"decision":         models.DecisionApproved,
				"assigned_role_id": role.ID,
			}).Error
		})
	case models.DecisionRejected:
		err = db.DB.Model(&application).Update("decision", models.DecisionRejected).Error
	default:
		writeError(w, http.StatusBadRequest, "решение: назначить роль или отклонить")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сохранить решение")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"decision": body.Decision})
}

func requireModerator(w http.ResponseWriter, r *http.Request) (models.User, bool) {
	user, ok := auth.RequireUser(w, r)
	if !ok {
		return models.User{}, false
	}
	if user.Role == nil || user.Role.Code != models.RoleModerator {
		writeError(w, http.StatusForbidden, "очередь доступна модератору")
		return models.User{}, false
	}
	return user, true
}

func toPublic(application models.Application, withPerson bool) publicApplication {
	out := publicApplication{
		ID:        application.ID,
		Body:      application.Body,
		Decision:  application.Decision,
		CreatedAt: application.CreatedAt,
	}
	if withPerson {
		out.FullName = application.User.FullName
		out.Login = application.User.Login
	}
	if application.AssignedRole != nil {
		out.AssignedRole = &publicRole{
			Code: application.AssignedRole.Code,
			Name: application.AssignedRole.Name,
		}
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
