-- 023_project_client_contact.up.sql
-- Chunk D (DAILY_REPORTS_CLIENT_UPDATES): the homeowner contact on a project.
-- A client update is emailed to the project's homeowner, so the project needs
-- to carry that contact. All three columns are nullable and additive — existing
-- projects are unaffected and a project without a homeowner contact simply
-- cannot have a client update sent (the service rejects an empty client_email
-- at send with 422 NO_CLIENT_CONTACT).
--
-- Backfill: a project created from a pre_construction_prospect inherits the
-- prospect's client contact. CreateProjectFromProspect (internal/store/
-- pipeline.go) previously DROPPED these fields on the floor — that leak is
-- fixed in code in the same chunk so NEW conversions carry them forward; this
-- one-shot UPDATE repairs the EXISTING projects that lost them. Only fills
-- where the project's own column is still NULL (idempotent, never clobbers an
-- operator-entered contact).
--
-- PII: client_name / client_email / client_phone are Restricted (homeowner
-- personal data). pii.FieldClass gains client_name/client_email/client_phone/
-- recipient_email → Restricted in the same chunk; never serialize client_email
-- to field_worker responses (handler/role-gated). Composite Currency N/A
-- (no monetary columns).

ALTER TABLE projects ADD COLUMN client_name  TEXT;
ALTER TABLE projects ADD COLUMN client_email TEXT;
ALTER TABLE projects ADD COLUMN client_phone TEXT;

-- Backfill from the originating prospect (close the historical leak). pcp links
-- to its project via project_id once it advances to PERMIT_ISSUED.
UPDATE projects p
   SET client_name  = pcp.client_name,
       client_email = pcp.client_email,
       client_phone = pcp.client_phone
  FROM pre_construction_prospects pcp
 WHERE pcp.project_id = p.id
   AND p.client_email IS NULL
   AND p.client_name IS NULL
   AND p.client_phone IS NULL;
