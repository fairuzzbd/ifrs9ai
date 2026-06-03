package workflow

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Config is the deserialized value of sys.config rows with key
// WORKFLOW_CONFIG_{ENTITY_TYPE_UPPER}. Fields must match the JSON stored by
// migration 0007.
//
// No if-else per entity in engine code — engine reads Eyes to determine path.
type Config struct {
	EntityType          string            `json:"entityType"`
	Eyes                int               `json:"eyes"` // 4 or 6 only
	Retractable         bool              `json:"retractable"`
	RequiredPermissions map[string]string `json:"requiredPermissions"` // action → permission
	StepUpRequired      map[string]bool   `json:"stepUpRequired"`
	SoDRules            SoDRulesConfig    `json:"sodRules"`
}

// SoDRulesConfig holds SoD rule flags from config. Hardcoded enforcement rules
// (DEC-017) always apply regardless of config; config is only read for
// approver2NotAnyPrevious which is entity-specific.
type SoDRulesConfig struct {
	ReviewerNotMaker           bool `json:"reviewerNotMaker"`
	ApproverNotMakerOrReviewer bool `json:"approverNotMakerOrReviewer"`
	Approver2NotAnyPrevious    bool `json:"approver2NotAnyPrevious"`
}

// Validate enforces hardcoded invariants that cannot be overridden (DEC-017).
func (c *Config) Validate() error {
	if c.Eyes != 4 && c.Eyes != 6 {
		return fmt.Errorf("Config.Eyes must be 4 or 6, got %d", c.Eyes)
	}
	if c.EntityType == "" {
		return fmt.Errorf("Config.EntityType must not be empty")
	}
	// DEC-017: these SoD rules cannot be disabled.
	if !c.SoDRules.ReviewerNotMaker {
		return fmt.Errorf("Config.SoDRules.ReviewerNotMaker must be true (DEC-017)")
	}
	if !c.SoDRules.ApproverNotMakerOrReviewer {
		return fmt.Errorf("Config.SoDRules.ApproverNotMakerOrReviewer must be true (DEC-017)")
	}
	return nil
}

// RequiredPermission returns the permission string for a given action key.
// Returns "" if not configured (caller should treat as FORBIDDEN).
func (c *Config) RequiredPermission(action string) string {
	return c.RequiredPermissions[strings.ToLower(action)]
}

// NeedsStepUp returns true if the given action requires step-up MFA.
func (c *Config) NeedsStepUp(action string) bool {
	return c.StepUpRequired[strings.ToLower(action)]
}

// ApproveTarget returns the state that APPROVE action leads to.
// 4-eyes → APPROVED, 6-eyes → PENDING_APPROVAL_2.
func (c *Config) ApproveTarget() State {
	if c.Eyes == 6 {
		return StatePendingApproval2
	}
	return StateApproved
}

// configKey returns the sys.config key for a resource name.
func configKey(resource string) string {
	upper := strings.ToUpper(strings.ReplaceAll(resource, "-", "_"))
	return "WORKFLOW_CONFIG_" + upper
}

// ConfigLoader abstracts reading Config rows from storage.
// Implementations: DBConfigLoader (production), InMemoryConfigLoader (tests).
type ConfigLoader interface {
	Load(entityType string) (*Config, error)
}

// CachedConfigLoader wraps another ConfigLoader with a short-lived cache (5 min).
// Thread-safe.
type CachedConfigLoader struct {
	mu    sync.RWMutex
	inner ConfigLoader
	ttl   time.Duration
	cache map[string]*cachedEntry
}

type cachedEntry struct {
	cfg       *Config
	expiresAt time.Time
}

// NewCachedConfigLoader creates a CachedConfigLoader with 5-minute TTL.
func NewCachedConfigLoader(inner ConfigLoader) *CachedConfigLoader {
	return &CachedConfigLoader{
		inner: inner,
		ttl:   5 * time.Minute,
		cache: make(map[string]*cachedEntry),
	}
}

// NewCachedConfigLoaderWithTTL creates a CachedConfigLoader with custom TTL (for testing).
func NewCachedConfigLoaderWithTTL(inner ConfigLoader, ttl time.Duration) *CachedConfigLoader {
	return &CachedConfigLoader{
		inner: inner,
		ttl:   ttl,
		cache: make(map[string]*cachedEntry),
	}
}

// Load returns the Config for entityType, using cache if fresh.
func (c *CachedConfigLoader) Load(entityType string) (*Config, error) {
	key := strings.ToUpper(entityType)

	c.mu.RLock()
	if e, ok := c.cache[key]; ok && time.Now().Before(e.expiresAt) {
		c.mu.RUnlock()
		return e.cfg, nil
	}
	c.mu.RUnlock()

	cfg, err := c.inner.Load(entityType)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.cache[key] = &cachedEntry{cfg: cfg, expiresAt: time.Now().Add(c.ttl)}
	c.mu.Unlock()

	return cfg, nil
}

// Invalidate removes an entry from the cache (call after sys.config update).
func (c *CachedConfigLoader) Invalidate(entityType string) {
	c.mu.Lock()
	delete(c.cache, strings.ToUpper(entityType))
	c.mu.Unlock()
}

// InMemoryConfigLoader is for unit tests — no DB required.
type InMemoryConfigLoader struct {
	configs map[string]*Config
}

// NewInMemoryConfigLoader creates a test loader from a map of entityType → config.
func NewInMemoryConfigLoader(configs map[string]*Config) *InMemoryConfigLoader {
	upper := make(map[string]*Config, len(configs))
	for k, v := range configs {
		upper[strings.ToUpper(k)] = v
	}
	return &InMemoryConfigLoader{configs: upper}
}

// Load returns the config for entityType or an error if not found.
func (l *InMemoryConfigLoader) Load(entityType string) (*Config, error) {
	if cfg, ok := l.configs[strings.ToUpper(entityType)]; ok {
		return cfg, nil
	}
	return nil, fmt.Errorf("Config not found for entity type %q", entityType)
}

// ParseWorkflowConfig parses a JSON string into a Config and validates it.
func ParseWorkflowConfig(raw string) (*Config, error) {
	var cfg Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, fmt.Errorf("parse Config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate Config: %w", err)
	}
	return &cfg, nil
}

// DefaultConfigs returns the in-memory equivalent of the sys.config seed
// from migration 0007. Used when no DB is available (tests, startup).
func DefaultConfigs() map[string]*Config {
	return map[string]*Config{
		"PENEMPATAN": {
			EntityType:  "PENEMPATAN",
			Eyes:        4,
			Retractable: false,
			RequiredPermissions: map[string]string{
				"submit":  "penempatan.submit",
				"review":  "penempatan.review",
				"approve": "penempatan.approve",
				"reject":  "penempatan.reject",
			},
			StepUpRequired: map[string]bool{"approve": false},
			SoDRules: SoDRulesConfig{
				ReviewerNotMaker:           true,
				ApproverNotMakerOrReviewer: true,
				Approver2NotAnyPrevious:    false,
			},
		},
		"JURNAL": {
			EntityType:  "JURNAL",
			Eyes:        4,
			Retractable: false,
			RequiredPermissions: map[string]string{
				"submit":  "jurnal.submit",
				"review":  "jurnal.review",
				"approve": "jurnal.approve",
				"reject":  "jurnal.reject",
			},
			StepUpRequired: map[string]bool{"approve": false},
			SoDRules: SoDRulesConfig{
				ReviewerNotMaker:           true,
				ApproverNotMakerOrReviewer: true,
				Approver2NotAnyPrevious:    false,
			},
		},
		"PERIODE": {
			EntityType:  "PERIODE",
			Eyes:        4,
			Retractable: false,
			RequiredPermissions: map[string]string{
				"submit":  "periode.softclose",
				"review":  "periode.softclose",
				"approve": "periode.hardclose",
				"reject":  "periode.reject",
			},
			StepUpRequired: map[string]bool{"approve": true},
			SoDRules: SoDRulesConfig{
				ReviewerNotMaker:           true,
				ApproverNotMakerOrReviewer: true,
				Approver2NotAnyPrevious:    false,
			},
		},
		"UPLOAD_BATCH": {
			EntityType:  "UPLOAD_BATCH",
			Eyes:        4,
			Retractable: true,
			RequiredPermissions: map[string]string{
				"submit":  "upload_batch.submit",
				"review":  "upload_batch.review",
				"approve": "upload_batch.approve",
				"reject":  "upload_batch.reject",
			},
			StepUpRequired: map[string]bool{"approve": false},
			SoDRules: SoDRulesConfig{
				ReviewerNotMaker:           true,
				ApproverNotMakerOrReviewer: true,
				Approver2NotAnyPrevious:    false,
			},
		},
		"KLASIFIKASI": {
			EntityType:  "KLASIFIKASI",
			Eyes:        6,
			Retractable: false,
			RequiredPermissions: map[string]string{
				"submit":   "klasifikasi.submit",
				"review":   "klasifikasi.review",
				"approve":  "klasifikasi.approve",
				"approve2": "klasifikasi.approve",
				"reject":   "klasifikasi.reject",
			},
			StepUpRequired: map[string]bool{"approve": false, "approve2": true},
			SoDRules: SoDRulesConfig{
				ReviewerNotMaker:           true,
				ApproverNotMakerOrReviewer: true,
				Approver2NotAnyPrevious:    true,
			},
		},
		"ECL_PARAMETER": {
			EntityType:  "ECL_PARAMETER",
			Eyes:        6,
			Retractable: false,
			RequiredPermissions: map[string]string{
				"submit":   "ecl_parameter.submit",
				"review":   "ecl_parameter.review",
				"approve":  "ecl_parameter.approve",
				"approve2": "ecl_parameter.approve",
				"reject":   "ecl_parameter.reject",
			},
			StepUpRequired: map[string]bool{"approve": true, "approve2": true},
			SoDRules: SoDRulesConfig{
				ReviewerNotMaker:           true,
				ApproverNotMakerOrReviewer: true,
				Approver2NotAnyPrevious:    true,
			},
		},
		// BOBOT_SKENARIO — 6-eyes ECL parameter (DEC-010, DEC-017).
		// Scenario weights (G/N/B) default 0.25/0.50/0.25; ALCO can override but
		// sum MUST equal 1.0. Both approve steps require step-up MFA (DEC-027).
		// SoD: approver2 ≠ maker ∧ ≠ reviewer ∧ ≠ approver1.
		// Source of truth: WORKFLOW_CONFIG_BOBOT_SKENARIO seeded in migration 0008.
		"BOBOT_SKENARIO": {
			EntityType:  "BOBOT_SKENARIO",
			Eyes:        6,
			Retractable: false,
			RequiredPermissions: map[string]string{
				"submit":   "ecl_parameter.submit",
				"review":   "ecl_parameter.review",
				"approve":  "ecl_parameter.approve",
				"approve2": "ecl_parameter.approve",
				"reject":   "ecl_parameter.reject",
			},
			StepUpRequired: map[string]bool{"approve": true, "approve2": true},
			SoDRules: SoDRulesConfig{
				ReviewerNotMaker:           true,
				ApproverNotMakerOrReviewer: true,
				Approver2NotAnyPrevious:    true,
			},
		},
	}
}
