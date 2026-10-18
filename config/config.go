package config

import (
	"os"
	"strings"
)

func Load() {
	data, err := os.ReadFile(".env")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, val)
	}
}

func getEnv(key, fallback string) string {
	if val := strings.TrimSpace(os.Getenv(key)); val != "" {
		return val
	}
	return fallback
}

func HTTPAddr() string {
	return getEnv("HTTP_ADDR", ":8080")
}

func CORSOrigin() string {
	return getEnv("CORS_ORIGIN", "http://localhost:5173,http://localhost")
}

func CORSOrigins() []string {
	parts := strings.Split(CORSOrigin(), ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		origins = append(origins, trimmed)
	}
	if len(origins) == 0 {
		return []string{"http://localhost:5173"}
	}
	return origins
}

func DBHost() string     { return getEnv("DB_HOST", "localhost") }
func DBPort() string     { return getEnv("DB_PORT", "5432") }
func DBUser() string     { return getEnv("DB_USER", "admin") }
func DBPassword() string { return getEnv("DB_PASSWORD", "secret") }
func DBName() string     { return getEnv("DB_NAME", "norilsk_gis") }

func RedisAddr() string {
	return getEnv("REDIS_ADDR", "localhost:6379")
}

func MinioEndpoint() string {
	return getEnv("MINIO_ENDPOINT", "localhost:9000")
}

func MinioAccessKey() string {
	return getEnv("MINIO_ACCESS_KEY", "minioadmin")
}

func MinioSecretKey() string {
	return getEnv("MINIO_SECRET_KEY", "minioadmin123")
}

func MinioBucket() string {
	return getEnv("MINIO_BUCKET", "norilsk-media")
}

func MinioUseSSL() bool {
	return strings.EqualFold(getEnv("MINIO_USE_SSL", "false"), "true")
}
