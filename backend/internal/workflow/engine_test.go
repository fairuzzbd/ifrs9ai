package workflow

import (
	"testing"
	"time"

	"github.com/google/uuid"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

func makeUUID(seed string) uuid.UUID {
	id, _ := uuid.Parse(seed)
	return id
}

var (
	makerID    = makeUUID("00000000-0000-0000-0000-000000000001")
	reviewerID = makeUUID("00000000-0000-0000-0000-000000000002")
	approverID = makeUUID("00000000-0000-0000-0000-000000000003")
	approver2ID = makeUUID("00000000-0000-0000-0000-000000000004")
	entityID   = makeUUID("aaaaaaaa-0000-0000-0000-000000000001")
)

func cfg4Eyes(retractable bool) *WorkflowConfig {
	return &WorkflowConfig{
		EntityType:  "PENEMPATAN",
		Eyes:        4,
		Retractable: retractable,
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
	}
}

func cfg6Eyes() *WorkflowConfig {
	return &WorkflowConfig{
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
	}
}

func cfg6EyesStepUpApprove() *WorkflowConfig {
	cfg := cfg6Eyes()
	cfg.StepUpRequired["approve"] = true
	return cfg
}

func draftInstance(cfg *WorkflowConfig) *Instance {
	return &Instance{
		ID:                uuid.New(),
		EntityType:        cfg.EntityType,
		EntityID:          entityID,
		EntitySchema:      "test",
		WorkflowConfigKey: configKey(cfg.EntityType),
		Eyes:              cfg.Eyes,
		CurrentState:      StateDraft,
		MakerID:           makerID,
		RowVersion:        1,
		CreatedBy:         makerID,
		TenantID:          "TUGURE",
	}
}

func buildEngine(cfg *WorkflowConfig) *Engine {
	loader := NewInMemoryConfigLoader(map[string]*WorkflowConfig{
		cfg.EntityType: cfg,
	})
	return NewEngine(loader)
}

func defaultActionRequest() ActionRequest {
	return ActionRequest{
		SignatureMethod: SignatureMethodJWTStandard,
	}
}

func withRowVersion(rv int64) ActionRequest {
	req := defaultActionRequest()
	req.RowVersion = &rv
	return req
}

// -----------------------------------------------------------------------
// 4-Eyes happy path: DRAFT → PENDING_REVIEW → PENDING_APPROVAL → APPROVED
// -----------------------------------------------------------------------

func TestEngine_FourEyes_HappyPath(t *testing.T) {
	cfg := cfg4Eyes(false)
	eng := buildEngine(cfg)
	inst := draftInstance(cfg)

	// 1. DRAFT → PENDING_REVIEW (submit by maker)
	res, err := eng.Transition(TransitionInput{
		Instance: inst, Action: ActionSubmit,
		CurrentUserID: makerID.String(), CurrentUsername: "maker",
		Request: defaultActionRequest(),
	})
	if err != nil {
		t.Fatalf("submit: unexpected error: %v", err)
	}
	if res.PreviousState != StateDraft {
		t.Errorf("previous state: got %s, want %s", res.PreviousState, StateDraft)
	}
	if res.NewState != StatePendingReview {
		t.Errorf("new state: got %s, want %s", res.NewState, StatePendingReview)
	}
	inst.CurrentState = res.NewState

	// 2. PENDING_REVIEW → PENDING_APPROVAL (review by reviewer)
	inst.ReviewerID = &reviewerID
	res, err = eng.Transition(TransitionInput{
		Instance: inst, Action: ActionReview,
		CurrentUserID: reviewerID.String(), CurrentUsername: "reviewer",
		Request: defaultActionRequest(),
	})
	if err != nil {
		t.Fatalf("review: unexpected error: %v", err)
	}
	if res.NewState != StatePendingApproval {
		t.Errorf("new state: got %s, want %s", res.NewState, StatePendingApproval)
	}
	inst.CurrentState = res.NewState

	// 3. PENDING_APPROVAL → APPROVED (approve by approver)
	inst.Approver1ID = &approverID
	res, err = eng.Transition(TransitionInput{
		Instance: inst, Action: ActionApprove,
		CurrentUserID: approverID.String(), CurrentUsername: "approver",
		Request: defaultActionRequest(),
	})
	if err != nil {
		t.Fatalf("approve: unexpected error: %v", err)
	}
	if res.NewState != StateApproved {
		t.Errorf("4-eyes approve: got %s, want APPROVED", res.NewState)
	}
	if len(res.NextActions) != 0 {
		t.Errorf("terminal state should have no next actions, got %v", res.NextActions)
	}
}

// -----------------------------------------------------------------------
// 6-Eyes happy path: DRAFT → ... → PENDING_APPROVAL_2 → APPROVED
// -----------------------------------------------------------------------

func TestEngine_SixEyes_HappyPath(t *testing.T) {
	cfg := cfg6Eyes()
	eng := buildEngine(cfg)
	inst := draftInstance(cfg)

	// Submit
	res, err := eng.Transition(TransitionInput{
		Instance: inst, Action: ActionSubmit,
		CurrentUserID: makerID.String(),
		Request:       defaultActionRequest(),
	})
	mustNoError(t, "submit", err)
	inst.CurrentState = res.NewState

	// Review
	inst.ReviewerID = &reviewerID
	res, err = eng.Transition(TransitionInput{
		Instance: inst, Action: ActionReview,
		CurrentUserID: reviewerID.String(),
		Request:       defaultActionRequest(),
	})
	mustNoError(t, "review", err)
	inst.CurrentState = res.NewState

	// Approve (6-eyes → PENDING_APPROVAL_2)
	inst.Approver1ID = &approverID
	res, err = eng.Transition(TransitionInput{
		Instance: inst, Action: ActionApprove,
		CurrentUserID: approverID.String(),
		Request:       defaultActionRequest(),
		StepUpFresh:   false, // approve stepUp=false for klasifikasi
	})
	mustNoError(t, "approve", err)
	if res.NewState != StatePendingApproval2 {
		t.Errorf("6-eyes approve: got %s, want PENDING_APPROVAL_2", res.NewState)
	}
	inst.CurrentState = res.NewState

	// Approve2 (6-eyes → APPROVED) with step-up
	inst.Approver2ID = &approver2ID
	res, err = eng.Transition(TransitionInput{
		Instance: inst, Action: ActionApprove2,
		CurrentUserID: approver2ID.String(),
		Request:       defaultActionRequest(),
		StepUpFresh:   true, // approve2 requires step-up
	})
	mustNoError(t, "approve2", err)
	if res.NewState != StateApproved {
		t.Errorf("6-eyes approve2: got %s, want APPROVED", res.NewState)
	}
}

// -----------------------------------------------------------------------
// Invalid transitions → WORKFLOW_INVALID_TRANSITION (422)
// -----------------------------------------------------------------------

func TestEngine_InvalidTransition_SkipReview(t *testing.T) {
	cfg := cfg4Eyes(false)
	eng := buildEngine(cfg)
	inst := draftInstance(cfg)

	// Try to approve directly from DRAFT (skip submit+review)
	_, err := eng.Transition(TransitionInput{
		Instance: inst, Action: ActionApprove,
		CurrentUserID: approverID.String(),
		Request:       defaultActionRequest(),
	})
	assertDomainCode(t, err, domainerrors.CodeWorkflowInvalidTransition)
}

func TestEngine_InvalidTransition_ApproveFromDraft(t *testing.T) {
	cfg := cfg4Eyes(false)
	eng := buildEngine(cfg)
	inst := draftInstance(cfg)

	_, err := eng.Transition(TransitionInput{
		Instance: inst, Action: ActionApprove,
		CurrentUserID: approverID.String(),
		Request:       defaultActionRequest(),
	})
	assertDomainCode(t, err, domainerrors.CodeWorkflowInvalidTransition)
}

func TestEngine_InvalidTransition_Approve2OnFourEyes(t *testing.T) {
	cfg := cfg4Eyes(false)
	eng := buildEngine(cfg)
	inst := draftInstance(cfg)
	inst.CurrentState = StatePendingApproval // simulate past review

	// PENDING_APPROVAL → APPROVE2 doesn't exist on 4-eyes
	_, err := eng.Transition(TransitionInput{
		Instance: inst, Action: ActionApprove2,
		CurrentUserID: approver2ID.String(),
		Request:       defaultActionRequest(),
	})
	assertDomainCode(t, err, domainerrors.CodeWorkflowInvalidTransition)
}

func TestEngine_InvalidTransition_FromTerminalApproved(t *testing.T) {
	cfg := cfg4Eyes(false)
	eng := buildEngine(cfg)
	inst := draftInstance(cfg)
	inst.CurrentState = StateApproved

	_, err := eng.Transition(TransitionInput{
		Instance: inst, Action: ActionSubmit,
		CurrentUserID: makerID.String(),
		Request:       defaultActionRequest(),
	})
	assertDomainCode(t, err, domainerrors.CodeWorkflowInvalidTransition)
}

func TestEngine_InvalidTransition_FromTerminalRejected(t *testing.T) {
	cfg := cfg4Eyes(false)
	eng := buildEngine(cfg)
	inst := draftInstance(cfg)
	inst.CurrentState = StateRejected

	_, err := eng.Transition(TransitionInput{
		Instance: inst, Action: ActionReview,
		CurrentUserID: reviewerID.String(),
		Request:       defaultActionRequest(),
	})
	assertDomainCode(t, err, domainerrors.CodeWorkflowInvalidTransition)
}

// -----------------------------------------------------------------------
// SoD violations → 403
// -----------------------------------------------------------------------

func TestEngine_SoD_MakerTriesToReview(t *testing.T) {
	cfg := cfg4Eyes(false)
	eng := buildEngine(cfg)
	inst := draftInstance(cfg)
	inst.CurrentState = StatePendingReview

	// Maker tries to review their own submission
	_, err := eng.Transition(TransitionInput{
		Instance: inst, Action: ActionReview,
		CurrentUserID: makerID.String(), // same as maker
		Request:       defaultActionRequest(),
	})
	assertDomainCode(t, err, domainerrors.CodeSoDViolation)
}

func TestEngine_SoD_MakerTriesToApprove(t *testing.T) {
	cfg := cfg4Eyes(false)
	eng := buildEngine(cfg)
	inst := draftInstance(cfg)
	inst.CurrentState = StatePendingApproval
	inst.ReviewerID = &reviewerID

	// Maker tries to approve — SoD violation
	_, err := eng.Transition(TransitionInput{
		Instance: inst, Action: ActionApprove,
		CurrentUserID: makerID.String(),
		Request:       defaultActionRequest(),
	})
	assertDomainCode(t, err, domainerrors.CodeSoDViolation)
}

func TestEngine_SoD_ReviewerTriesToApprove(t *testing.T) {
	cfg := cfg4Eyes(false)
	eng := buildEngine(cfg)
	inst := draftInstance(cfg)
	inst.CurrentState = StatePendingApproval
	inst.ReviewerID = &reviewerID

	// Reviewer tries to approve (reviewer == approver) — SoD violation
	_, err := eng.Transition(TransitionInput{
		Instance: inst, Action: ActionApprove,
		CurrentUserID: reviewerID.String(), // same as reviewer
		Request:       defaultActionRequest(),
	})
	assertDomainCode(t, err, domainerrors.CodeSoDViolation)
}

func TestEngine_SoD_Approver2SameAsReviewer_SixEyes(t *testing.T) {
	cfg := cfg6Eyes()
	eng := buildEngine(cfg)
	inst := draftInstance(cfg)
	inst.CurrentState = StatePendingApproval2
	inst.ReviewerID = &reviewerID
	inst.Approver1ID = &approverID

	// reviewerID tries to be approver2 — SOD_APPROVER2_SAME_AS_REVIEWER
	_, err := eng.Transition(TransitionInput{
		Instance: inst, Action: ActionApprove2,
		CurrentUserID: reviewerID.String(),
		Request:       defaultActionRequest(),
		StepUpFresh:   true,
	})
	assertDomainCode(t, err, domainerrors.CodeSoDApprover2SameAsReviewer)
}

func TestEngine_SoD_Approver2SameAsApprover1_SixEyes(t *testing.T) {
	cfg := cfg6Eyes()
	eng := buildEngine(cfg)
	inst := draftInstance(cfg)
	inst.CurrentState = StatePendingApproval2
	inst.ReviewerID = &reviewerID
	inst.Approver1ID = &approverID

	// approverID tries to be approver2 — SoD violation (approver2NotAnyPrevious)
	_, err := eng.Transition(TransitionInput{
		Instance: inst, Action: ActionApprove2,
		CurrentUserID: approverID.String(), // same as approver1
		Request:       defaultActionRequest(),
		StepUpFresh:   true,
	})
	assertDomainCode(t, err, domainerrors.CodeSoDViolation)
}

// -----------------------------------------------------------------------
// Optimistic lock → CONFLICT (409)
// -----------------------------------------------------------------------

func TestEngine_OptimisticLock_Mismatch(t *testing.T) {
	cfg := cfg4Eyes(false)
	eng := buildEngine(cfg)
	inst := draftInstance(cfg)

	wrongVersion := int64(999)
	req := ActionRequest{
		SignatureMethod: SignatureMethodJWTStandard,
		RowVersion:     &wrongVersion,
	}

	_, err := eng.Transition(TransitionInput{
		Instance: inst, Action: ActionSubmit,
		CurrentUserID: makerID.String(),
		Request:       req,
	})
	assertDomainCode(t, err, domainerrors.CodeConflict)
}

func TestEngine_OptimisticLock_Match(t *testing.T) {
	cfg := cfg4Eyes(false)
	eng := buildEngine(cfg)
	inst := draftInstance(cfg) // RowVersion = 1

	rv := int64(1)
	req := ActionRequest{
		SignatureMethod: SignatureMethodJWTStandard,
		RowVersion:     &rv,
	}

	_, err := eng.Transition(TransitionInput{
		Instance: inst, Action: ActionSubmit,
		CurrentUserID: makerID.String(),
		Request:       req,
	})
	if err != nil {
		t.Fatalf("expected no error with correct rowVersion, got: %v", err)
	}
}

// -----------------------------------------------------------------------
// Step-up MFA guards
// -----------------------------------------------------------------------

func TestEngine_StepUp_Required_Missing(t *testing.T) {
	cfg := cfg6EyesStepUpApprove()
	eng := buildEngine(cfg)
	inst := draftInstance(cfg)
	inst.CurrentState = StatePendingApproval
	inst.ReviewerID = &reviewerID
	inst.Approver1ID = &approverID

	// approve requires step-up but StepUpFresh=false
	_, err := eng.Transition(TransitionInput{
		Instance: inst, Action: ActionApprove,
		CurrentUserID: approverID.String(),
		Request:       defaultActionRequest(),
		StepUpFresh:   false,
	})
	assertDomainCode(t, err, domainerrors.CodeStepUpRequired)
}

func TestEngine_StepUp_Required_Fresh(t *testing.T) {
	cfg := cfg6EyesStepUpApprove()
	eng := buildEngine(cfg)
	inst := draftInstance(cfg)
	inst.CurrentState = StatePendingApproval
	inst.ReviewerID = &reviewerID

	_, err := eng.Transition(TransitionInput{
		Instance: inst, Action: ActionApprove,
		CurrentUserID: approverID.String(),
		Request:       defaultActionRequest(),
		StepUpFresh:   true, // fresh step-up
	})
	if err != nil {
		t.Fatalf("expected no error with fresh step-up, got: %v", err)
	}
}

// -----------------------------------------------------------------------
// Reject from various PENDING states
// -----------------------------------------------------------------------

func TestEngine_Reject_FromPendingReview(t *testing.T) {
	cfg := cfg4Eyes(false)
	eng := buildEngine(cfg)
	inst := draftInstance(cfg)
	inst.CurrentState = StatePendingReview

	comment := "insufficient documentation"
	_, err := eng.Transition(TransitionInput{
		Instance: inst, Action: ActionReject,
		CurrentUserID: reviewerID.String(),
		Request:       defaultActionRequest(),
		RejectComment: &comment,
	})
	mustNoError(t, "reject from PENDING_REVIEW", err)
}

func TestEngine_Reject_FromPendingApproval(t *testing.T) {
	cfg := cfg4Eyes(false)
	eng := buildEngine(cfg)
	inst := draftInstance(cfg)
	inst.CurrentState = StatePendingApproval
	inst.ReviewerID = &reviewerID

	comment := "limit exceeded"
	_, err := eng.Transition(TransitionInput{
		Instance: inst, Action: ActionReject,
		CurrentUserID: approverID.String(),
		Request:       defaultActionRequest(),
		RejectComment: &comment,
	})
	mustNoError(t, "reject from PENDING_APPROVAL", err)
}

// -----------------------------------------------------------------------
// Retract (optional config)
// -----------------------------------------------------------------------

func TestEngine_Retract_WhenConfigured(t *testing.T) {
	cfg := cfg4Eyes(true) // retractable=true
	eng := buildEngine(cfg)
	inst := draftInstance(cfg)
	inst.CurrentState = StatePendingReview

	res, err := eng.Transition(TransitionInput{
		Instance: inst, Action: ActionRetract,
		CurrentUserID: makerID.String(),
		Request:       defaultActionRequest(),
	})
	mustNoError(t, "retract", err)
	if res.NewState != StateDraft {
		t.Errorf("retract: got %s, want DRAFT", res.NewState)
	}
}

func TestEngine_Retract_WhenNotConfigured(t *testing.T) {
	cfg := cfg4Eyes(false) // retractable=false
	eng := buildEngine(cfg)
	inst := draftInstance(cfg)
	inst.CurrentState = StatePendingReview

	_, err := eng.Transition(TransitionInput{
		Instance: inst, Action: ActionRetract,
		CurrentUserID: makerID.String(),
		Request:       defaultActionRequest(),
	})
	assertDomainCode(t, err, domainerrors.CodeWorkflowInvalidTransition)
}

// -----------------------------------------------------------------------
// Signature hash determinism
// -----------------------------------------------------------------------

func TestComputeSignatureHash_Deterministic(t *testing.T) {
	h1 := computeSignatureHash("user1", "SUBMIT", "entity1", "2026-01-01T00:00:00Z", "comment")
	h2 := computeSignatureHash("user1", "SUBMIT", "entity1", "2026-01-01T00:00:00Z", "comment")
	if h1 != h2 {
		t.Errorf("signature hash not deterministic: %s != %s", h1, h2)
	}
}

func TestComputeSignatureHash_DifferentInputsDifferentHash(t *testing.T) {
	h1 := computeSignatureHash("user1", "SUBMIT", "entity1", "2026-01-01T00:00:00Z", "comment")
	h2 := computeSignatureHash("user2", "SUBMIT", "entity1", "2026-01-01T00:00:00Z", "comment")
	if h1 == h2 {
		t.Error("different users should produce different hashes")
	}
}

// -----------------------------------------------------------------------
// Config validation
// -----------------------------------------------------------------------

func TestWorkflowConfig_Validate_InvalidEyes(t *testing.T) {
	cfg := cfg4Eyes(false)
	cfg.Eyes = 3
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for Eyes=3")
	}
}

func TestWorkflowConfig_Validate_SoDReviewerNotMakerFalse(t *testing.T) {
	cfg := cfg4Eyes(false)
	cfg.SoDRules.ReviewerNotMaker = false
	if err := cfg.Validate(); err == nil {
		t.Error("expected error when ReviewerNotMaker=false")
	}
}

func TestWorkflowConfig_Validate_SoDApproverNotMakerOrReviewerFalse(t *testing.T) {
	cfg := cfg4Eyes(false)
	cfg.SoDRules.ApproverNotMakerOrReviewer = false
	if err := cfg.Validate(); err == nil {
		t.Error("expected error when ApproverNotMakerOrReviewer=false")
	}
}

func TestWorkflowConfig_ApproveTarget(t *testing.T) {
	if cfg4Eyes(false).ApproveTarget() != StateApproved {
		t.Error("4-eyes approve target should be APPROVED")
	}
	if cfg6Eyes().ApproveTarget() != StatePendingApproval2 {
		t.Error("6-eyes approve target should be PENDING_APPROVAL_2")
	}
}

// -----------------------------------------------------------------------
// State helpers
// -----------------------------------------------------------------------

func TestState_IsTerminal(t *testing.T) {
	cases := map[State]bool{
		StateDraft:            false,
		StatePendingReview:    false,
		StatePendingApproval:  false,
		StatePendingApproval2: false,
		StateApproved:         true,
		StateRejected:         true,
	}
	for s, want := range cases {
		if got := s.IsTerminal(); got != want {
			t.Errorf("State(%s).IsTerminal() = %v, want %v", s, got, want)
		}
	}
}

func TestState_IsPending(t *testing.T) {
	cases := map[State]bool{
		StateDraft:            false,
		StatePendingReview:    true,
		StatePendingApproval:  true,
		StatePendingApproval2: true,
		StateApproved:         false,
		StateRejected:         false,
	}
	for s, want := range cases {
		if got := s.IsPending(); got != want {
			t.Errorf("State(%s).IsPending() = %v, want %v", s, got, want)
		}
	}
}

// -----------------------------------------------------------------------
// CachedConfigLoader
// -----------------------------------------------------------------------

func TestCachedConfigLoader_Cache(t *testing.T) {
	inner := NewInMemoryConfigLoader(map[string]*WorkflowConfig{
		"PENEMPATAN": cfg4Eyes(false),
	})
	cached := NewCachedConfigLoaderWithTTL(inner, 1*time.Hour)

	c1, err := cached.Load("PENEMPATAN")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := cached.Load("PENEMPATAN")
	if err != nil {
		t.Fatal(err)
	}
	// Cached — same pointer
	if c1 != c2 {
		t.Error("expected same pointer from cache")
	}
}

func TestCachedConfigLoader_Invalidate(t *testing.T) {
	inner := NewInMemoryConfigLoader(map[string]*WorkflowConfig{
		"PENEMPATAN": cfg4Eyes(false),
	})
	cached := NewCachedConfigLoaderWithTTL(inner, 1*time.Hour)

	// Load to populate cache.
	c1, err := cached.Load("PENEMPATAN")
	if err != nil {
		t.Fatal(err)
	}
	// Invalidate, then reload — cache should be re-populated (not the old entry).
	cached.Invalidate("PENEMPATAN")
	c2, err := cached.Load("PENEMPATAN")
	if err != nil {
		t.Fatal(err)
	}
	// Both should have the same EntityType (values are equal after reload).
	if c1.EntityType != c2.EntityType {
		t.Errorf("entity type mismatch after invalidate: %s != %s", c1.EntityType, c2.EntityType)
	}
	// Load again without invalidation — should be cached (same pointer as c2).
	c3, _ := cached.Load("PENEMPATAN")
	if c2 != c3 {
		t.Error("expected same pointer from cache (no invalidation between c2 and c3)")
	}
}

// -----------------------------------------------------------------------
// ParseWorkflowConfig
// -----------------------------------------------------------------------

func TestParseWorkflowConfig_Valid(t *testing.T) {
	raw := `{
		"entityType": "PENEMPATAN",
		"eyes": 4,
		"retractable": false,
		"requiredPermissions": {
			"submit": "penempatan.submit",
			"review": "penempatan.review",
			"approve": "penempatan.approve",
			"reject": "penempatan.reject"
		},
		"stepUpRequired": {"approve": false},
		"sodRules": {
			"reviewerNotMaker": true,
			"approverNotMakerOrReviewer": true,
			"approver2NotAnyPrevious": false
		}
	}`
	cfg, err := ParseWorkflowConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Eyes != 4 {
		t.Errorf("eyes: got %d, want 4", cfg.Eyes)
	}
}

func TestParseWorkflowConfig_InvalidEyes(t *testing.T) {
	raw := `{"entityType":"X","eyes":3,"sodRules":{"reviewerNotMaker":true,"approverNotMakerOrReviewer":true}}`
	if _, err := ParseWorkflowConfig(raw); err == nil {
		t.Error("expected error for invalid eyes")
	}
}

// -----------------------------------------------------------------------
// NextActions
// -----------------------------------------------------------------------

func TestNextActions_DraftHasSubmit(t *testing.T) {
	cfg := cfg4Eyes(false)
	actions := nextActions(StateDraft, cfg)
	if !contains(actions, "submit") {
		t.Errorf("DRAFT should have 'submit' in next actions, got %v", actions)
	}
}

func TestNextActions_ApprovedIsEmpty(t *testing.T) {
	cfg := cfg4Eyes(false)
	actions := nextActions(StateApproved, cfg)
	if len(actions) != 0 {
		t.Errorf("APPROVED should have no next actions, got %v", actions)
	}
}

// -----------------------------------------------------------------------
// Test helpers
// -----------------------------------------------------------------------

func mustNoError(t *testing.T, label string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: unexpected error: %v", label, err)
	}
}

func assertDomainCode(t *testing.T, err error, code domainerrors.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %s, got nil", code)
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok {
		t.Fatalf("expected DomainError with code %s, got: %T: %v", code, err, err)
	}
	if de.Code() != code {
		t.Errorf("error code: got %s, want %s (message: %s)", de.Code(), code, de.Message())
	}
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
