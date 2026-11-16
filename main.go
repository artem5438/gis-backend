package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"gis-backend/applications"
	"gis-backend/auth"
	"gis-backend/config"
	"gis-backend/db"
	"gis-backend/parcels"

	_ "gis-backend/docs"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger"
)

// @title           ГИС Норильска
// @version         1
// @description     Запросы сервера. Вход открывает cookie session_id на сутки. Дальнейшие запросы ходят с этой cookie.
// @host            localhost:8080
// @BasePath        /

func main() {
	config.Load()
	db.Connect()
	db.ConnectRedis()
	db.Migrate()

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(corsMiddleware)

	router.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("http://localhost:8080/swagger/doc.json"),
	))

	router.Get("/api/health", health)
	router.Post("/api/auth/register", auth.Register)
	router.Post("/api/auth/login", auth.Login)
	router.Post("/api/auth/logout", auth.Logout)
	router.Get("/api/auth/me", auth.Me)
	router.Post("/api/auth/source", auth.SaveSource)
	router.Post("/api/applications", applications.Create)
	router.Get("/api/applications/mine", applications.Mine)
	router.Get("/api/applications", applications.Queue)
	router.Post("/api/applications/{id}/decision", applications.Decide)
	router.Get("/api/parcels", parcels.Layer)
	router.Get("/api/analytics", parcels.Analytics)
	router.Post("/api/parcels/at-point", parcels.AtPoint)
	router.Post("/api/parcels/stored", parcels.StoredAtPoint)
	router.Get("/api/catalog/walk", parcels.WalkStatus)
	router.Post("/api/catalog/walk", parcels.StartWalk)

	parcels.StartWalker()

	addr := config.HTTPAddr()
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatal(err)
	}
}

type healthResponse struct {
	Status string `json:"status"`
}

// Health проверяет, что сервер отвечает.
// @Summary      Проверка сервера
// @Tags         сервер
// @Produce      json
// @Success      200  {object}  healthResponse
// @Router       /api/health [get]
func health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(healthResponse{Status: "ok"})
}

func corsMiddleware(next http.Handler) http.Handler {
	allowed := config.CORSOrigins()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		chosen := allowed[0]
		for _, item := range allowed {
			if strings.EqualFold(item, origin) {
				chosen = origin
				break
			}
		}
		w.Header().Set("Access-Control-Allow-Origin", chosen)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
