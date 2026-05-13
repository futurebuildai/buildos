-- buildos:destructive: rollback of 010_setup_infrastructure — drops all wizard tables and reverts organizations columns.
-- Wizard state and all captured tenant configuration is lost. Acceptable
-- only in pre-prod / dev — production rollback path is forward-only
-- (introduce a 011 migration that disables the wizard at the app layer
-- instead of dropping the schema).

DROP TABLE IF EXISTS permit_jurisdictions;
DROP TABLE IF EXISTS holiday_overrides;
DROP TABLE IF EXISTS working_calendars;
DROP TABLE IF EXISTS cost_codes;
DROP TABLE IF EXISTS trade_categories;
DROP TABLE IF EXISTS setup_bootstrap_tokens;

ALTER TABLE organizations
    DROP COLUMN IF EXISTS onboarding_completed_at,
    DROP COLUMN IF EXISTS onboarding_complete,
    DROP COLUMN IF EXISTS region,
    DROP COLUMN IF EXISTS company_type,
    DROP COLUMN IF EXISTS ein,
    DROP COLUMN IF EXISTS address,
    DROP COLUMN IF EXISTS legal_name;
