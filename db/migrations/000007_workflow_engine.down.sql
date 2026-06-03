-- migration: 0007 workflow_engine (DOWN)
-- author: data-modeler
-- description: Remove workflow_instance, workflow_signature, related functions,
--              and WORKFLOW_CONFIG_* rows from sys.config.

BEGIN;

-- Remove seed config rows
DELETE FROM sys.config WHERE config_key LIKE 'WORKFLOW_CONFIG_%';

-- Drop triggers first
DROP TRIGGER IF EXISTS tg_wf_signature_no_delete  ON sys.workflow_signature;
DROP TRIGGER IF EXISTS tg_wf_signature_no_update  ON sys.workflow_signature;
DROP TRIGGER IF EXISTS tg_wf_protect_timestamps   ON sys.workflow_instance;
DROP TRIGGER IF EXISTS tg_wf_instance_row_version ON sys.workflow_instance;
DROP TRIGGER IF EXISTS tg_wf_instance_updated_at  ON sys.workflow_instance;

-- Drop tables (signature first — FK to workflow_instance)
DROP TABLE IF EXISTS sys.workflow_signature;
DROP TABLE IF EXISTS sys.workflow_instance;

-- Drop functions
DROP FUNCTION IF EXISTS fn_wf_signature_immutable();
DROP FUNCTION IF EXISTS fn_wf_protect_signing_timestamps();

COMMIT;
