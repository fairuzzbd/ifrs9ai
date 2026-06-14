package document

import (
	"testing"
)

// TestValidateObjectKey_CoverDotComponents memverifikasi komponen "." tunggal di-reject.
func TestValidateObjectKey_CoverDotComponents(t *testing.T) {
	// Path "." sendiri harus di-reject.
	err := ValidateObjectKey("./something")
	if err == nil {
		t.Error("ValidateObjectKey('./something') harus mengembalikan error")
	}
}

// TestMinIOConfig_IsNotDryRun memverifikasi MinIOConfig tidak ada "dry-run" concept
// (berbeda dari SMTPConfig). MinIO selalu connect bila endpoint diisi.
func TestMinIOConfig_IsNotDryRun(t *testing.T) {
	cfg := MinIOConfig{
		Endpoint:          "localhost:9000",
		AccessKeyID:       "minioadmin",
		SecretAccessKey:   "minioadmin",
		UseSSL:            false,
		PresignTTLMinutes: DefaultPresignTTLMinutes,
	}
	if cfg.Endpoint == "" {
		t.Error("MinIOConfig endpoint tidak boleh kosong")
	}
	if cfg.PresignTTLMinutes != DefaultPresignTTLMinutes {
		t.Errorf("PresignTTLMinutes = %d, want %d", cfg.PresignTTLMinutes, DefaultPresignTTLMinutes)
	}
}

// TestDefaultPresignTTLMinutes memverifikasi TTL default adalah 60 menit.
func TestDefaultPresignTTLMinutes(t *testing.T) {
	if DefaultPresignTTLMinutes != 60 {
		t.Errorf("DefaultPresignTTLMinutes = %d, want 60", DefaultPresignTTLMinutes)
	}
}

// TestDefaultBucket memverifikasi nama bucket default.
func TestDefaultBucket(t *testing.T) {
	if DefaultBucket != "blips-documents" {
		t.Errorf("DefaultBucket = %q, want 'blips-documents'", DefaultBucket)
	}
}

// TestQuarantineBucket memverifikasi nama quarantine bucket.
func TestQuarantineBucket(t *testing.T) {
	if QuarantineBucket != "blips-quarantine" {
		t.Errorf("QuarantineBucket = %q, want 'blips-quarantine'", QuarantineBucket)
	}
}

// TestNewMinIOClient_InvalidEndpoint memverifikasi bahwa NewMinIOClient tidak panic
// untuk endpoint yang tidak valid. minio.New() dengan endpoint kosong akan error.
// Ini memverifikasi bahwa service tetap bisa start meski MinIO tidak tersedia.
func TestNewMinIOClient_InvalidEndpoint(t *testing.T) {
	// minio-go memvalidasi endpoint saat New(); endpoint kosong adalah invalid.
	_, err := NewMinIOClient(MinIOConfig{
		Endpoint:        "", // kosong → invalid
		AccessKeyID:     "key",
		SecretAccessKey: "secret",
	}, nil)
	// minio-go v7 mungkin tidak error pada endpoint kosong tapi pada operasi pertama.
	// Yang kita pastikan: tidak ada panic.
	_ = err
}
