package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"gis-backend/db"
	"gis-backend/models"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	sessionCookie = "session_id"
	sessionTTL    = 24 * time.Hour
)

type publicUser struct {
	ID                 uint        `json:"id"`
	Login              string      `json:"login"`
	FullName           string      `json:"full_name"`
	VerificationStatus string      `json:"verification_status"`
	FromDatabase       bool        `json:"from_database"`
	Role               *publicRole `json:"role"`
}

type publicRole struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

func Register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Login    string `json:"login"`
		FullName string `json:"full_name"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "некорректный JSON")
		return
	}
	login, fullName, password, ok := cleanAccount(body.Login, body.FullName, body.Password)
	if !ok {
		writeError(w, http.StatusBadRequest, "нужны ФИО, логин от 3 символов и пароль от 8 символов")
		return
	}

	var existing models.User
	err := db.DB.Where("login = ?", login).First(&existing).Error
	if err == nil {
		writeError(w, http.StatusConflict, "такой логин уже занят")
		return
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(w, http.StatusInternalServerError, "не удалось проверить логин")
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сохранить пароль")
		return
	}
	user := models.User{
		Login:              login,
		FullName:           fullName,
		PasswordHash:       string(hashed),
		VerificationStatus: models.StatusVisitor,
		FromDatabase:       true,
	}
	if err := db.DB.Create(&user).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось создать учётную запись")
		return
	}
	if err := startSession(w, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "учётная запись создана, но сессия не открылась")
		return
	}
	writeJSON(w, http.StatusCreated, toPublic(user))
}

func Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "некорректный JSON")
		return
	}
	login := strings.TrimSpace(body.Login)
	password := body.Password
	if login == "" || password == "" {
		writeError(w, http.StatusBadRequest, "нужны логин и пароль")
		return
	}

	var user models.User
	err := db.DB.Preload("Role").Where("login = ?", login).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(w, http.StatusUnauthorized, "неверный логин или пароль")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось проверить вход")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		writeError(w, http.StatusUnauthorized, "неверный логин или пароль")
		return
	}
	if err := startSession(w, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось открыть сессию")
		return
	}
	writeJSON(w, http.StatusOK, toPublic(user))
}

func Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil && cookie.Value != "" {
		_ = db.Redis.Del(r.Context(), sessionKey(cookie.Value)).Err()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func Me(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromRequest(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, toPublic(user))
}

// RequireUser reads the session and the account. On failure it writes the response.
func RequireUser(w http.ResponseWriter, r *http.Request) (models.User, bool) {
	return userFromRequest(w, r)
}

func userFromRequest(w http.ResponseWriter, r *http.Request) (models.User, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		writeError(w, http.StatusUnauthorized, "нужен вход")
		return models.User{}, false
	}
	raw, err := db.Redis.Get(r.Context(), sessionKey(cookie.Value)).Result()
	if errors.Is(err, redis.Nil) {
		writeError(w, http.StatusUnauthorized, "сессия закончилась")
		return models.User{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось прочитать сессию")
		return models.User{}, false
	}
	var user models.User
	err = db.DB.Preload("Role").First(&user, raw).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		_ = db.Redis.Del(r.Context(), sessionKey(cookie.Value)).Err()
		writeError(w, http.StatusUnauthorized, "нужен вход")
		return models.User{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось прочитать учётную запись")
		return models.User{}, false
	}
	return user, true
}

func startSession(w http.ResponseWriter, userID uint) error {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return err
	}
	id := hex.EncodeToString(buf)
	if err := db.Redis.Set(context.Background(), sessionKey(id), userID, sessionTTL).Err(); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    id,
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func sessionKey(id string) string {
	return "session:" + id
}

func cleanAccount(login, fullName, password string) (string, string, string, bool) {
	login = strings.TrimSpace(login)
	fullName = strings.TrimSpace(fullName)
	if strings.Contains(login, " ") || utf8.RuneCountInString(login) < 3 || utf8.RuneCountInString(login) > 100 {
		return "", "", "", false
	}
	if utf8.RuneCountInString(fullName) < 2 || utf8.RuneCountInString(fullName) > 300 {
		return "", "", "", false
	}
	if utf8.RuneCountInString(password) < 8 || utf8.RuneCountInString(password) > 72 {
		return "", "", "", false
	}
	return login, fullName, password, true
}

// SaveSource remembers whether the map reads a saved parcel or asks the public map.
func SaveSource(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireUser(w, r)
	if !ok {
		return
	}
	var body struct {
		FromDatabase *bool `json:"from_database"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.FromDatabase == nil {
		writeError(w, http.StatusBadRequest, "нужно положение переключателя")
		return
	}
	if err := db.DB.Model(&user).Update("from_database", *body.FromDatabase).Error; err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сохранить переключатель")
		return
	}
	user.FromDatabase = *body.FromDatabase
	writeJSON(w, http.StatusOK, toPublic(user))
}

func toPublic(user models.User) publicUser {
	out := publicUser{
		ID:                 user.ID,
		Login:              user.Login,
		FullName:           user.FullName,
		VerificationStatus: user.VerificationStatus,
		FromDatabase:       user.FromDatabase,
	}
	if user.Role != nil {
		out.Role = &publicRole{Code: user.Role.Code, Name: user.Role.Name}
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
