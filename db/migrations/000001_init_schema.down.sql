-- migration: 0001 init_schema (DOWN)
-- author: data-modeler
-- description: Drop semua schema BLIPS + function publik.
--              Extensions sengaja TIDAK di-drop (non-destructive untuk shared PG instance).

DROP SCHEMA IF EXISTS aud CASCADE;
DROP SCHEMA IF EXISTS ecl CASCADE;
DROP SCHEMA IF EXISTS jrnl CASCADE;
DROP SCHEMA IF EXISTS trx CASCADE;
DROP SCHEMA IF EXISTS sppi CASCADE;
DROP SCHEMA IF EXISTS doc CASCADE;
DROP SCHEMA IF EXISTS mst CASCADE;
DROP SCHEMA IF EXISTS sys CASCADE;
DROP SCHEMA IF EXISTS sec CASCADE;

DROP FUNCTION IF EXISTS public.uuidv7();
DROP FUNCTION IF EXISTS fn_update_updated_at();
DROP FUNCTION IF EXISTS fn_audit_no_modify();
DROP FUNCTION IF EXISTS fn_instrumen_klasifikasi_lock();
DROP FUNCTION IF EXISTS fn_kurs_no_modify_when_locked();
