// Package config memuat konfigurasi aplikasi BLIPS IFRS9 dari environment.
//
// Urutan resolusi: file .env opsional (via godotenv) di-load lebih dulu agar
// nilai-nilainya tersedia di os.Getenv, baru kemudian setiap field dibaca dengan
// fallback default yang aman untuk pengembangan lokal (lihat docker-compose.dev.yml).
package config

import (
	"os"

	"github.com/joho/godotenv"
)

// Config menampung seluruh konfigurasi runtime yang dibaca dari environment.
type Config struct {
	// ServerPort adalah port HTTP yang di-bind oleh API server.
	ServerPort string
	// AppEnv menandakan lingkungan aktif: development, staging, atau production.
	AppEnv string

	// DatabaseURL adalah DSN PostgreSQL (sslmode menyesuaikan lingkungan).
	DatabaseURL string
	// RedisURL adalah URL koneksi Redis (Asynq + cache).
	RedisURL string

	// MinIOEndpoint, MinIOAccessKey, MinIOSecretKey untuk object storage S3-compatible.
	MinIOEndpoint  string
	MinIOAccessKey string
	MinIOSecretKey string

	// CORSAllowedOrigins adalah daftar origin yang diizinkan (comma-separated).
	CORSAllowedOrigins string

	// JWTSecret hanya placeholder Phase 0. Produksi memakai verifikasi RSA-2048
	// dari Keycloak (DEC-006/DEC-025) dan nilai ini wajib di-supply via secrets manager.
	JWTSecret string
}

// Load membaca konfigurasi dari environment.
//
// File .env bersifat opsional: bila tidak ada, godotenv mengembalikan error yang
// sengaja diabaikan sehingga konfigurasi tetap dapat di-resolve dari environment
// proses (mis. di container atau CI).
func Load() *Config {
	// Best-effort: .env opsional, abaikan error bila file tidak tersedia.
	_ = godotenv.Load()

	return &Config{
		ServerPort:         getenv("SERVER_PORT", "8080"),
		AppEnv:             getenv("APP_ENV", "development"),
		DatabaseURL:        getenv("DATABASE_URL", "postgres://blips_admin:change_me_in_production@localhost:5432/blips_db?sslmode=disable"),
		RedisURL:           getenv("REDIS_URL", "redis://localhost:6379/0"),
		MinIOEndpoint:      getenv("MINIO_ENDPOINT", "localhost:9000"),
		MinIOAccessKey:     getenv("MINIO_ACCESS_KEY", "minioadmin"),
		MinIOSecretKey:     getenv("MINIO_SECRET_KEY", "minioadmin"),
		CORSAllowedOrigins: getenv("CORS_ALLOWED_ORIGINS", "http://localhost:3000"),
		JWTSecret:          getenv("JWT_SECRET", "dev-only-insecure-jwt-secret-change-me"),
	}
}

// getenv mengembalikan nilai environment untuk key, atau fallback bila kosong/tidak diset.
func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
