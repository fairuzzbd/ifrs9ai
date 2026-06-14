-- migration: 0028 amendment_detection_idempotency
-- author: ecl-eir-engineer
-- requires: 0001 (init_schema — ecl.eir_reestimation_log),
--           0026 (eir_schema_fix — workflow_status CHECK with CANCELLED),
--           0027 (drift_report_and_amendment_lifecycle — document_id column)
-- description:
--   B3 fix (compliance finding PR #66): adds a partial UNIQUE index on
--   (document_id, instrumen_id) for non-terminal, non-null-document proposals.
--
--   Without this constraint, two concurrent actors (or a 24h Idempotency-Key TTL
--   expiry + retry) can create duplicate amendment proposals from the same document
--   for the same instrument.  The application-layer Idempotency-Key header (24h TTL)
--   covers the normal race window, but does not protect against:
--     - Different actor submitting same (doc, instrument) pair after key expiry.
--     - Direct API calls that bypass the idempotency middleware.
--
--   The index is partial (WHERE … document_id IS NOT NULL) so that proposals
--   created without a document (MANUAL, DRIFT_DETECTION_AUTO, PRE_ECL_GATE) are
--   not constrained — only DOCUMENT_UPLOAD proposals have document_id populated.
--
--   The service (DetectionService.DetectFromDocument) catches PG error code 23505
--   (unique_violation) and returns the existing proposal as a 200 idempotent
--   response instead of a 409 CONFLICT.
--
-- References:
--   - FSD-APP-C §M6-001 (detect-from-document idempotency requirement)
--   - docs/state-machines/p4-m6-amendment-lifecycle.md §3 (DRAFT creation guard)
--   - DEC-018 (no hard delete), DEC-021 (idempotency mandatory)

BEGIN;

-- Partial UNIQUE constraint: one active (non-terminal) proposal per (document, instrument).
-- Skips CANCELLED + REJECTED (terminal) rows so a new detection can proceed after cancel.
-- Skips rows where document_id IS NULL (manual / drift proposals have no document FK).
CREATE UNIQUE INDEX IF NOT EXISTS uq_eir_reestimation_active_doc_instrumen
    ON ecl.eir_reestimation_log (document_id, instrumen_id)
    WHERE workflow_status NOT IN ('CANCELLED', 'REJECTED')
      AND document_id IS NOT NULL;

COMMENT ON INDEX uq_eir_reestimation_active_doc_instrumen IS
    'Prevents duplicate DOCUMENT_UPLOAD amendment proposals for the same '
    '(document_id, instrumen_id) pair while the proposal is non-terminal. '
    'Terminal rows (CANCELLED, REJECTED) are excluded so re-detection after '
    'cancellation is allowed. NULL document_id rows (MANUAL, DRIFT_DETECTION_AUTO) '
    'are excluded from this constraint. '
    'Service layer catches PG-23505 and returns existing proposal (idempotent 200). '
    'Added by migration 000028 (B3 compliance fix, PR #66).';

COMMIT;
