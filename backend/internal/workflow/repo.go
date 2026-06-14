package workflow

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Repository defines the storage contract for workflow instances and signatures.
// Two implementations: DBRepository (production) + InMemoryRepository (unit tests).
type Repository interface {
	// GetByEntityID returns the workflow instance for a given entity, or nil if none.
	GetByEntityID(ctx context.Context, entityType string, entityID uuid.UUID) (*Instance, error)

	// GetByID returns a workflow instance by its own UUID.
	GetByID(ctx context.Context, id uuid.UUID) (*Instance, error)

	// UpdateState updates current_state, actor fields, timestamps, and increments row_version.
	// Must be called within a transaction (tx != nil).
	UpdateState(ctx context.Context, tx *sql.Tx, update StateUpdate) error

	// InsertSignature appends a SignatureRecord (never UPDATE or DELETE).
	// Must be called within a transaction (tx != nil).
	InsertSignature(ctx context.Context, tx *sql.Tx, sig *SignatureRecord) error

	// ListSignatures returns all signature records for a workflow, ordered by signed_at ASC.
	ListSignatures(ctx context.Context, workflowID uuid.UUID) ([]SignatureRecord, error)

	// BeginTx starts a serializable transaction. Caller must Commit or Rollback.
	BeginTx(ctx context.Context) (*sql.Tx, error)
}

// StateUpdate captures the fields to mutate in a single workflow transition.
type StateUpdate struct {
	WorkflowID uuid.UUID
	NewState   State
	UpdatedBy  uuid.UUID
	RowVersion int64 // expected current row_version (optimistic lock applied at DB level too)
	// Conditionally-set actor IDs (nil = no change)
	ReviewerID    *uuid.UUID
	Approver1ID   *uuid.UUID
	Approver2ID   *uuid.UUID
	RejectedBy    *uuid.UUID
	RejectComment *string
	RejectStep    *string
	// Timestamps — set the one corresponding to the action
	SubmittedAt *time.Time
	ReviewedAt  *time.Time
	Approved1At *time.Time
	Approved2At *time.Time
	RejectedAt  *time.Time
}

// -----------------------------------------------------------------------
// DB implementation
// -----------------------------------------------------------------------

// DBRepository implements Repository backed by PostgreSQL via database/sql.
type DBRepository struct {
	db *sql.DB
}

// NewDBRepository creates a production repository.
func NewDBRepository(db *sql.DB) *DBRepository {
	return &DBRepository{db: db}
}

// BeginTx starts a serializable transaction.
func (r *DBRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, fmt.Errorf("workflow repo: begin tx: %w", err)
	}
	return tx, nil
}

// GetByEntityID loads a workflow_instance by (entity_type, entity_id).
func (r *DBRepository) GetByEntityID(ctx context.Context, entityType string, entityID uuid.UUID) (*Instance, error) {
	return getInstanceWhere(ctx, r.db, "entity_type = $1 AND entity_id = $2 AND deleted_at IS NULL", entityType, entityID)
}

// GetByID loads a workflow_instance by its own id.
func (r *DBRepository) GetByID(ctx context.Context, id uuid.UUID) (*Instance, error) {
	return getInstanceWhere(ctx, r.db, "id = $1 AND deleted_at IS NULL", id)
}

func getInstanceWhere(ctx context.Context, db *sql.DB, where string, args ...any) (*Instance, error) {
	query := `
		SELECT id, entity_type, entity_id, entity_schema,
		       workflow_config_key, eyes, current_state,
		       maker_id, reviewer_id, approver1_id, approver2_id, rejected_by,
		       submitted_at, reviewed_at, approved1_at, approved2_at, rejected_at,
		       reject_comment, reject_step,
		       created_at, created_by, updated_at, updated_by,
		       row_version, tenant_id
		FROM sys.workflow_instance
		WHERE ` + where

	var inst Instance
	var reviewerID, approver1ID, approver2ID, rejectedBy sql.NullString
	err := db.QueryRowContext(ctx, query, args...).Scan(
		&inst.ID, &inst.EntityType, &inst.EntityID, &inst.EntitySchema,
		&inst.WorkflowConfigKey, &inst.Eyes, &inst.CurrentState,
		&inst.MakerID, &reviewerID, &approver1ID, &approver2ID, &rejectedBy,
		&inst.SubmittedAt, &inst.ReviewedAt, &inst.Approved1At, &inst.Approved2At, &inst.RejectedAt,
		&inst.RejectComment, &inst.RejectStep,
		&inst.CreatedAt, &inst.CreatedBy, &inst.UpdatedAt, &inst.UpdatedBy,
		&inst.RowVersion, &inst.TenantID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("workflow repo: get instance: %w", err)
	}

	if reviewerID.Valid {
		id, err := uuid.Parse(reviewerID.String)
		if err != nil {
			return nil, fmt.Errorf("workflow repo: parse reviewer_id %q: %w", reviewerID.String, err)
		}
		inst.ReviewerID = &id
	}
	if approver1ID.Valid {
		id, err := uuid.Parse(approver1ID.String)
		if err != nil {
			return nil, fmt.Errorf("workflow repo: parse approver1_id %q: %w", approver1ID.String, err)
		}
		inst.Approver1ID = &id
	}
	if approver2ID.Valid {
		id, err := uuid.Parse(approver2ID.String)
		if err != nil {
			return nil, fmt.Errorf("workflow repo: parse approver2_id %q: %w", approver2ID.String, err)
		}
		inst.Approver2ID = &id
	}
	if rejectedBy.Valid {
		id, err := uuid.Parse(rejectedBy.String)
		if err != nil {
			return nil, fmt.Errorf("workflow repo: parse rejected_by %q: %w", rejectedBy.String, err)
		}
		inst.RejectedBy = &id
	}

	return &inst, nil
}

// UpdateState updates workflow_instance state in a transaction with optimistic lock.
// The DB trigger fn_wf_protect_signing_timestamps additionally protects timestamps.
func (r *DBRepository) UpdateState(ctx context.Context, tx *sql.Tx, u StateUpdate) error {
	now := time.Now()
	_, err := tx.ExecContext(ctx, `
		UPDATE sys.workflow_instance SET
			current_state   = $1,
			reviewer_id     = COALESCE($2, reviewer_id),
			approver1_id    = COALESCE($3, approver1_id),
			approver2_id    = COALESCE($4, approver2_id),
			rejected_by     = COALESCE($5, rejected_by),
			reject_comment  = COALESCE($6, reject_comment),
			reject_step     = COALESCE($7, reject_step),
			submitted_at    = COALESCE($8, submitted_at),
			reviewed_at     = COALESCE($9, reviewed_at),
			approved1_at    = COALESCE($10, approved1_at),
			approved2_at    = COALESCE($11, approved2_at),
			rejected_at     = COALESCE($12, rejected_at),
			updated_at      = $13,
			updated_by      = $14
		WHERE id = $15 AND row_version = $16 AND deleted_at IS NULL`,
		string(u.NewState),
		uuidPtrToNullable(u.ReviewerID),
		uuidPtrToNullable(u.Approver1ID),
		uuidPtrToNullable(u.Approver2ID),
		uuidPtrToNullable(u.RejectedBy),
		u.RejectComment,
		u.RejectStep,
		u.SubmittedAt,
		u.ReviewedAt,
		u.Approved1At,
		u.Approved2At,
		u.RejectedAt,
		now,
		u.UpdatedBy,
		u.WorkflowID,
		u.RowVersion,
	)
	return err
}

// InsertSignature inserts a new signature record (append-only).
func (r *DBRepository) InsertSignature(ctx context.Context, tx *sql.Tx, sig *SignatureRecord) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO sys.workflow_signature
			(id, workflow_id, action, user_id, role_at_time, signed_at,
			 signature_hash, signature_method, comment, tenant_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		sig.ID, sig.WorkflowID, string(sig.Action),
		sig.UserID, sig.RoleAtTime, sig.SignedAt,
		sig.SignatureHash, string(sig.SignatureMethod),
		sig.Comment, sig.TenantID,
	)
	if err != nil {
		return fmt.Errorf("workflow repo: insert signature: %w", err)
	}
	return nil
}

// ListSignatures returns all signatures for a workflow ordered by signed_at ASC.
func (r *DBRepository) ListSignatures(ctx context.Context, workflowID uuid.UUID) ([]SignatureRecord, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, workflow_id, action, user_id, role_at_time, signed_at,
		       signature_hash, signature_method, comment, tenant_id
		FROM sys.workflow_signature
		WHERE workflow_id = $1
		ORDER BY signed_at ASC`, workflowID)
	if err != nil {
		return nil, fmt.Errorf("workflow repo: list signatures: %w", err)
	}
	defer rows.Close()

	var sigs []SignatureRecord
	for rows.Next() {
		var s SignatureRecord
		var comment sql.NullString
		if err := rows.Scan(
			&s.ID, &s.WorkflowID, &s.Action, &s.UserID, &s.RoleAtTime, &s.SignedAt,
			&s.SignatureHash, &s.SignatureMethod, &comment, &s.TenantID,
		); err != nil {
			return nil, fmt.Errorf("workflow repo: scan signature: %w", err)
		}
		if comment.Valid {
			s.Comment = &comment.String
		}
		sigs = append(sigs, s)
	}
	return sigs, rows.Err()
}

// uuidPtrToNullable converts *uuid.UUID to a scannable value for COALESCE($n, col).
func uuidPtrToNullable(u *uuid.UUID) any {
	if u == nil {
		return nil
	}
	return *u
}

// -----------------------------------------------------------------------
// DBConfigLoader — reads Config from sys.config
// -----------------------------------------------------------------------

// DBConfigLoader implements ConfigLoader by querying sys.config.
type DBConfigLoader struct {
	db *sql.DB
}

// NewDBConfigLoader creates a production ConfigLoader.
func NewDBConfigLoader(db *sql.DB) *DBConfigLoader {
	return &DBConfigLoader{db: db}
}

// Load reads and parses the WORKFLOW_CONFIG_{ENTITY} row from sys.config.
func (l *DBConfigLoader) Load(entityType string) (*Config, error) {
	key := configKey(entityType)
	var raw string
	err := l.db.QueryRowContext(
		context.Background(), // config reads are not request-scoped
		`SELECT config_value FROM sys.config WHERE config_key = $1`, key,
	).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("Config not found for entity type %q (key: %s)", entityType, key)
	}
	if err != nil {
		return nil, fmt.Errorf("DBConfigLoader: query sys.config: %w", err)
	}
	return ParseWorkflowConfig(raw)
}

// -----------------------------------------------------------------------
// InMemoryRepository — for unit tests (no DB required)
// -----------------------------------------------------------------------

// InMemoryRepository is a thread-unsafe in-memory repository for unit tests.
// Mark integration tests that require actual DB separately.
type InMemoryRepository struct {
	instances  map[string]*Instance // key = entityType+":"+entityID.String()
	signatures []SignatureRecord
}

// NewInMemoryRepository creates an empty in-memory repository.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		instances: make(map[string]*Instance),
	}
}

// Seed adds an instance to the in-memory store.
func (r *InMemoryRepository) Seed(inst *Instance) {
	r.instances[instKey(inst.EntityType, inst.EntityID)] = inst
}

func instKey(entityType string, entityID uuid.UUID) string {
	return entityType + ":" + entityID.String()
}

func (r *InMemoryRepository) GetByEntityID(_ context.Context, entityType string, entityID uuid.UUID) (*Instance, error) {
	if inst, ok := r.instances[instKey(entityType, entityID)]; ok {
		return inst, nil
	}
	return nil, nil
}

func (r *InMemoryRepository) GetByID(_ context.Context, id uuid.UUID) (*Instance, error) {
	for _, inst := range r.instances {
		if inst.ID == id {
			return inst, nil
		}
	}
	return nil, nil
}

func (r *InMemoryRepository) UpdateState(_ context.Context, _ *sql.Tx, u StateUpdate) error {
	for k := range r.instances {
		inst := r.instances[k]
		if inst.ID == u.WorkflowID {
			if inst.RowVersion != u.RowVersion {
				return fmt.Errorf("optimistic lock: expected row_version %d, got %d", u.RowVersion, inst.RowVersion)
			}
			inst.CurrentState = u.NewState
			if u.ReviewerID != nil {
				inst.ReviewerID = u.ReviewerID
			}
			if u.Approver1ID != nil {
				inst.Approver1ID = u.Approver1ID
			}
			if u.Approver2ID != nil {
				inst.Approver2ID = u.Approver2ID
			}
			if u.RejectedBy != nil {
				inst.RejectedBy = u.RejectedBy
			}
			if u.RejectComment != nil {
				inst.RejectComment = u.RejectComment
			}
			if u.RejectStep != nil {
				inst.RejectStep = u.RejectStep
			}
			inst.RowVersion++
			inst.UpdatedBy = u.UpdatedBy
			r.instances[k] = inst
			return nil
		}
	}
	return fmt.Errorf("workflow instance %s not found", u.WorkflowID)
}

func (r *InMemoryRepository) InsertSignature(_ context.Context, _ *sql.Tx, sig *SignatureRecord) error {
	r.signatures = append(r.signatures, *sig)
	return nil
}

func (r *InMemoryRepository) ListSignatures(_ context.Context, workflowID uuid.UUID) ([]SignatureRecord, error) {
	var sigs []SignatureRecord
	for i := range r.signatures {
		if r.signatures[i].WorkflowID == workflowID {
			sigs = append(sigs, r.signatures[i])
		}
	}
	return sigs, nil
}

func (r *InMemoryRepository) BeginTx(_ context.Context) (*sql.Tx, error) {
	// In-memory: no actual transaction — return nil (UpdateState/InsertSignature accept nil tx).
	return nil, nil
}

// GetSignatures returns all recorded signatures (test helper).
func (r *InMemoryRepository) GetSignatures() []SignatureRecord {
	return r.signatures
}
