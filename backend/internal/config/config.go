// Package config memuat konfigurasi aplikasi BLIPS IFRS9 dari environment.
//
// Urutan resolusi: file .env opsional (via godotenv) di-load lebih dulu agar
// nilai-nilainya tersedia di os.Getenv, baru kemudian setiap field dibaca dengan
// fallback default yang aman untuk pengembangan lokal (lihat docker-compose.dev.yml).
package config

import (
	"fmt"
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
	// MinIOUseSSL mengaktifkan TLS ke MinIO (production: true, dev: false).
	MinIOUseSSL bool
	// DocumentPresignTTLMinutes adalah TTL presigned download URL (default: 60 menit).
	DocumentPresignTTLMinutes int
	// BlockPendingDownload menentukan apakah download dokumen yang masih berstatus
	// VirusScanPending diblokir (env: DOCUMENT_BLOCK_PENDING_DOWNLOAD, default: true).
	// Set ke false hanya untuk development/testing. Production WAJIB true.
	BlockPendingDownload bool

	// SMTP config untuk notification service.
	// Semua nilai HARUS dari Vault/KMS di production — TIDAK pernah hardcode.
	// Bila SMTPHost kosong, mailer masuk dry-run mode (dev-safe).
	SMTPHost     string // SMTP_HOST
	SMTPPort     string // SMTP_PORT, default "587"
	SMTPUsername string // SMTP_USERNAME
	SMTPPassword string // SMTP_PASSWORD (dari KMS)
	SMTPFrom     string // SMTP_FROM
	SMTPUseTLS   bool   // SMTP_USE_TLS

	// CORSAllowedOrigins adalah daftar origin yang diizinkan (comma-separated).
	CORSAllowedOrigins string

	// JWTPublicKeyPEM adalah RSA-2048 public key PEM untuk verifikasi JWT Keycloak (DEC-025).
	// Di development, boleh kosong (JWT verification tidak aktif).
	// WAJIB diisi di production/staging via secrets manager — TIDAK pernah hardcode.
	JWTPublicKeyPEM string

	// JWTIssuer adalah expected issuer URL dari Keycloak.
	JWTIssuer string
}

// Load membaca konfigurasi dari environment.
//
// File .env bersifat opsional: bila tidak ada, godotenv mengembalikan error yang
// sengaja diabaikan sehingga konfigurasi tetap dapat di-resolve dari environment
// proses (mis. di container atau CI).
//
// DATABASE_URL: production/staging WAJIB menyediakan via env (Vault/KMS).
// Dev boleh kosong (DB connection skip gracefully di main.go).
// Tidak ada hardcoded credentials — lihat DEC-028.
func Load() *Config {
	// Best-effort: .env opsional, abaikan error bila file tidak tersedia.
	_ = godotenv.Load()

	appEnv := getenv("APP_ENV", "development")

	return &Config{
		ServerPort:                getenv("SERVER_PORT", "8081"),
		AppEnv:                    appEnv,
		DatabaseURL:               resolveDatabaseURL(),
		RedisURL:                  getenv("REDIS_URL", "redis://localhost:6379/0"),
		MinIOEndpoint:             getenv("MINIO_ENDPOINT", "localhost:9000"),
		MinIOAccessKey:            getenv("MINIO_ACCESS_KEY", "minioadmin"),
		MinIOSecretKey:            getenv("MINIO_SECRET_KEY", "minioadmin"),
		MinIOUseSSL:               getenv("MINIO_USE_SSL", "false") == "true",
		DocumentPresignTTLMinutes: getenvInt("DOCUMENT_PRESIGN_TTL_MINUTES", 60),
		BlockPendingDownload:      getenv("DOCUMENT_BLOCK_PENDING_DOWNLOAD", "true") != "false",
		SMTPHost:                  getenv("SMTP_HOST", ""),
		SMTPPort:                  getenv("SMTP_PORT", "587"),
		SMTPUsername:              getenv("SMTP_USERNAME", ""),
		SMTPPassword:              getenv("SMTP_PASSWORD", ""),
		SMTPFrom:                  getenv("SMTP_FROM", "BLIPS IFRS9 <noreply@blips.tugu-re.com>"),
		SMTPUseTLS:                getenv("SMTP_USE_TLS", "false") == "true",
		CORSAllowedOrigins:        getenv("CORS_ALLOWED_ORIGINS", "http://localhost:3001"),
		JWTPublicKeyPEM:           getenv("JWT_PUBLIC_KEY_PEM", ""),
		JWTIssuer:                 getenv("JWT_ISSUER", "http://localhost:8080/realms/blips"),
	}
}

// resolveDatabaseURL mengembalikan DATABASE_URL dari environment.
//
// Tidak ada string kredensial hardcoded di binary (DEC-028, MEDIUM-4/LOW-5).
// Bila kosong di production/staging, main.go wajib fail-fast (DB connection akan gagal).
func resolveDatabaseURL() string {
	return os.Getenv("DATABASE_URL") // kosong string jika tidak di-set; caller bertanggung jawab
}

// getenv mengembalikan nilai environment untuk key, atau fallback bila kosong/tidak diset.
func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// getenvInt mengembalikan nilai integer dari environment, atau fallback.
func getenvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return fallback
	}
	return n
}
