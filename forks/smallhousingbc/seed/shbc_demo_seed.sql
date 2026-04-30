-- SHBC × BuildOS — Demo seed for a real BuildOS database
--
-- Mirror of the demo's mock data, shaped for the BuildOS Sprint 5 schema.
-- Run AFTER all migrations have been applied. Idempotent-ish: uses
-- `INSERT ... ON CONFLICT DO NOTHING` where the table has a usable
-- unique key. Re-running on top of partial state is supported.
--
-- This file is illustrative — the demo's primary surface is the
-- standalone HTML/JS in `forks/smallhousingbc/demo/`. This SQL is here
-- so a future engineer can take the same dataset and seed a live BuildOS
-- instance for an end-to-end demo against real APIs.
--
-- Tables touched:
--   organizations, users, projects, project_tasks (sketch only),
--   procurement_items, project_budgets, invoices, certifications,
--   feed_cards, fleet_assets, employees
--
-- NOT included (because they don't yet exist in core BuildOS):
--   municipalities, bylaw_checklist_items, design_library_entries,
--   cohort_members. These would be net-new tables in the SHBC fork
--   migration. See `forks/smallhousingbc/demo/data/*.json` for the
--   shape; production schema design lands when this demo greenlights
--   the proposal.

BEGIN;

-- ============================================================
-- Organizations
-- ============================================================

INSERT INTO organizations (id, name, slug) VALUES
  ('11111111-1111-1111-1111-aaaaaaaaaaaa', 'Small Housing BC', 'small-housing-bc'),
  ('22222222-2222-2222-2222-aaaaaaaaaaaa', 'Aldergrove Build Co.', 'aldergrove-build'),
  ('33333333-3333-3333-3333-aaaaaaaaaaaa', 'East Van Density Studio', 'east-van-density'),
  ('44444444-4444-4444-4444-aaaaaaaaaaaa', 'Sea-to-Sky Workshop', 'sea-to-sky-workshop'),
  ('55555555-5555-5555-5555-aaaaaaaaaaaa', 'Cottonwood Carpentry', 'cottonwood-carpentry')
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- Users (org principals + key crew)
-- ============================================================

INSERT INTO users (id, oidc_subject, org_id, email, display_name, role) VALUES
  -- SHBC staff
  ('aaaa1111-0000-0000-0000-000000000001', 'oidc:daniel-winer', '11111111-1111-1111-1111-aaaaaaaaaaaa', 'daniel@smallhousingbc.org', 'Daniel Winer', 'owner'),
  ('aaaa1111-0000-0000-0000-000000000002', 'oidc:tamara-white', '11111111-1111-1111-1111-aaaaaaaaaaaa', 'tamara@smallhousingbc.org', 'Tamara White', 'owner'),
  -- Aldergrove
  ('bbbb2222-0000-0000-0000-000000000001', 'oidc:priya-mehta', '22222222-2222-2222-2222-aaaaaaaaaaaa', 'priya@aldergrove-build.ca', 'Priya Mehta', 'owner'),
  ('bbbb2222-0000-0000-0000-000000000002', 'oidc:jess-kowalski', '22222222-2222-2222-2222-aaaaaaaaaaaa', 'jess@aldergrove-build.ca', 'Jess Kowalski', 'superintendent'),
  ('bbbb2222-0000-0000-0000-000000000003', 'oidc:tomas-rivera', '22222222-2222-2222-2222-aaaaaaaaaaaa', 'tomas@aldergrove-build.ca', 'Tomás Rivera', 'field_worker'),
  -- East Van Density
  ('cccc3333-0000-0000-0000-000000000001', 'oidc:marc-tremblay', '33333333-3333-3333-3333-aaaaaaaaaaaa', 'marc@eastvandensity.ca', 'Marc Tremblay', 'owner'),
  ('cccc3333-0000-0000-0000-000000000002', 'oidc:maria-hernandez', '33333333-3333-3333-3333-aaaaaaaaaaaa', 'maria@eastvandensity.ca', 'Maria Hernandez', 'superintendent'),
  ('cccc3333-0000-0000-0000-000000000003', 'oidc:amir-khan', '33333333-3333-3333-3333-aaaaaaaaaaaa', 'amir@eastvandensity.ca', 'Amir Khan', 'admin'),
  -- Sea-to-Sky
  ('dddd4444-0000-0000-0000-000000000001', 'oidc:laila-brennan', '44444444-4444-4444-4444-aaaaaaaaaaaa', 'laila@seatoskyworkshop.ca', 'Laila Brennan', 'owner'),
  ('dddd4444-0000-0000-0000-000000000002', 'oidc:jamie-forrest', '44444444-4444-4444-4444-aaaaaaaaaaaa', 'jamie@seatoskyworkshop.ca', 'Jamie Forrest', 'superintendent'),
  -- Cottonwood
  ('eeee5555-0000-0000-0000-000000000001', 'oidc:devon-singh', '55555555-5555-5555-5555-aaaaaaaaaaaa', 'devon@cottonwood-carpentry.ca', 'Devon Singh', 'owner'),
  ('eeee5555-0000-0000-0000-000000000002', 'oidc:rae-macintosh', '55555555-5555-5555-5555-aaaaaaaaaaaa', 'rae@cottonwood-carpentry.ca', 'Rae MacIntosh', 'superintendent'),
  ('eeee5555-0000-0000-0000-000000000003', 'oidc:paul-bjornsson', '55555555-5555-5555-5555-aaaaaaaaaaaa', 'paul@cottonwood-carpentry.ca', 'Paul Bjornsson', 'field_worker')
ON CONFLICT (oidc_subject) DO NOTHING;

-- ============================================================
-- Projects
-- ============================================================
-- gsf is INTEGER 1500-6000 in core; some laneway/ADU projects are
-- below that range so use the lower bound check or relax it in the
-- fork's migration. Below uses the schema as-is; rows that violate the
-- check are commented out.

INSERT INTO projects (id, org_id, name, address, permit_issued_date, project_start_date, status, gsf) VALUES
  ('a0000001-0000-0000-0000-000000000001', '22222222-2222-2222-2222-aaaaaaaaaaaa', 'Buchanan Sixplex',         '5142 Buchanan Ave, Burnaby BC',     '2025-08-22', '2025-09-12', 'active', 5800),
  ('a0000001-0000-0000-0000-000000000002', '22222222-2222-2222-2222-aaaaaaaaaaaa', 'Boundary Triplex',         '3895 Boundary Rd, Burnaby BC',      '2026-01-30', '2026-02-10', 'active', 3200),
  ('a0000001-0000-0000-0000-000000000003', '22222222-2222-2222-2222-aaaaaaaaaaaa', 'Brentwood Park Sixplex',   '2287 Springer Ave, Burnaby BC',     '2025-07-04', '2025-07-22', 'active', 5500),
  ('a0000001-0000-0000-0000-000000000004', '33333333-3333-3333-3333-aaaaaaaaaaaa', 'Commercial Drive Fourplex','1840 Commercial Dr, Vancouver BC',   NULL,         '2025-11-04', 'active', 3600),
  ('a0000001-0000-0000-0000-000000000005', '33333333-3333-3333-3333-aaaaaaaaaaaa', 'Mount Pleasant Fourplex',  '236 East 11th Ave, Vancouver BC',   '2025-07-15', '2025-08-02', 'active', 3800),
  ('a0000001-0000-0000-0000-000000000007', '44444444-4444-4444-4444-aaaaaaaaaaaa', 'Brackendale Laneway',      '1112 Depot Rd, Squamish BC',        '2025-09-25', '2025-10-08', 'active', 1500),  -- gsf clamped to schema min
  ('a0000001-0000-0000-0000-00000000000A', '55555555-5555-5555-5555-aaaaaaaaaaaa', 'Maple Ridge Sixplex',      '11785 222 St, Maple Ridge BC',      '2025-08-02', '2025-08-15', 'active', 5400),
  ('a0000001-0000-0000-0000-00000000000B', '55555555-5555-5555-5555-aaaaaaaaaaaa', 'Hammond Triplex',          '11402 Maple Crescent, Maple Ridge BC','2025-10-15','2025-10-22', 'active', 2900)
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- Procurement (composite-currency)
-- ============================================================

INSERT INTO procurement_items
  (id, project_id, org_id, name, wbs_code, estimated_cost_cents, estimated_cost_currency_code,
   lead_time_days, weather_buffer_days, need_by_date, status, po_number, ordered_at)
VALUES
  -- Commercial Drive Fourplex
  ('c1000001-0000-0000-0000-000000000001', 'a0000001-0000-0000-0000-000000000004', '33333333-3333-3333-3333-aaaaaaaaaaaa',
    'Framing lumber package',     '06.10.10', 8400000, 'CAD', 18, 7,  CURRENT_DATE + 14, 'CRITICAL', NULL, NULL),
  ('c1000001-0000-0000-0000-000000000002', 'a0000001-0000-0000-0000-000000000004', '33333333-3333-3333-3333-aaaaaaaaaaaa',
    'Triple-glazed windows (12)', '08.50.00', 4900000, 'CAD', 56, 7,  CURRENT_DATE + 65, 'WARNING',  NULL, NULL),
  ('c1000001-0000-0000-0000-000000000003', 'a0000001-0000-0000-0000-000000000004', '33333333-3333-3333-3333-aaaaaaaaaaaa',
    'BC Hydro 200A → 400A',       '26.00.00', 4240000, 'CAD', 90, 0,  CURRENT_DATE + 70, 'CRITICAL', NULL, NULL),

  -- Buchanan Sixplex (mostly ordered)
  ('c1000002-0000-0000-0000-000000000001', 'a0000001-0000-0000-0000-000000000001', '22222222-2222-2222-2222-aaaaaaaaaaaa',
    'Framing + I-joists',         '06.10.10', 14200000,'CAD', 21, 7, CURRENT_DATE - 3,  'ORDERED',  'PO-2025-441', '2025-09-22'),
  ('c1000002-0000-0000-0000-000000000002', 'a0000001-0000-0000-0000-000000000001', '22222222-2222-2222-2222-aaaaaaaaaaaa',
    'BC Hydro UG service',        '26.00.10', 9800000, 'CAD', 120, 0, CURRENT_DATE + 12, 'CRITICAL', NULL, NULL),
  ('c1000002-0000-0000-0000-000000000003', 'a0000001-0000-0000-0000-000000000001', '22222222-2222-2222-2222-aaaaaaaaaaaa',
    'Window package (18)',        '08.50.00', 7300000, 'CAD', 56, 7,  CURRENT_DATE + 30, 'WARNING',  NULL, NULL)
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- Project budgets — keyed by project + WBS
-- ============================================================
-- Schema in core uses (project_id, wbs_code) composite uniqueness;
-- BulkInsertBudgets is the typical service entrypoint. This raw INSERT
-- is illustrative.

INSERT INTO project_budgets
  (id, project_id, org_id, wbs_code, label, budgeted_amount_cents, currency_code,
   committed_amount_cents, actual_amount_cents)
VALUES
  ('b0000001-0000-0000-0000-000000000001', 'a0000001-0000-0000-0000-000000000004', '33333333-3333-3333-3333-aaaaaaaaaaaa',
    '06.10', 'Framing & Lumber', 32000000, 'CAD', 8400000, 0),
  ('b0000001-0000-0000-0000-000000000002', 'a0000001-0000-0000-0000-000000000004', '33333333-3333-3333-3333-aaaaaaaaaaaa',
    '23.20', 'MEP Rough-in',     24000000, 'CAD', 3200000, 0),
  ('b0000001-0000-0000-0000-000000000003', 'a0000001-0000-0000-0000-000000000004', '33333333-3333-3333-3333-aaaaaaaaaaaa',
    '26.00', 'Electrical & BC Hydro', 14000000, 'CAD', 4240000, 0)
ON CONFLICT DO NOTHING;

-- ============================================================
-- Invoices
-- ============================================================

INSERT INTO invoices
  (id, project_id, org_id, vendor_name, invoice_number, amount_cents, currency_code, status, due_date, wbs_code)
VALUES
  ('d0000001-0000-0000-0000-000000000001', 'a0000001-0000-0000-0000-000000000001', '22222222-2222-2222-2222-aaaaaaaaaaaa',
    'BC Hydro',          'BCH-2026-3041', 4240000, 'CAD', 'approved', CURRENT_DATE + 14, '26.00'),
  ('d0000001-0000-0000-0000-000000000002', 'a0000001-0000-0000-0000-000000000004', '33333333-3333-3333-3333-aaaaaaaaaaaa',
    'Doman Building Materials', 'DBM-79204', 8400000, 'CAD', 'approved', CURRENT_DATE + 21, '06.10')
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- Employees + certifications
-- ============================================================

INSERT INTO employees (id, org_id, user_id, first_name, last_name, role, phone, hire_date) VALUES
  ('e0000001-0000-0000-0000-000000000001', '33333333-3333-3333-3333-aaaaaaaaaaaa', 'cccc3333-0000-0000-0000-000000000002', 'Maria',  'Hernandez', 'superintendent', '+1-604-555-0102', '2022-04-04'),
  ('e0000001-0000-0000-0000-000000000002', '22222222-2222-2222-2222-aaaaaaaaaaaa', 'bbbb2222-0000-0000-0000-000000000002', 'Jess',   'Kowalski',  'superintendent', '+1-604-555-0203', '2020-08-12'),
  ('e0000001-0000-0000-0000-000000000003', '55555555-5555-5555-5555-aaaaaaaaaaaa', 'eeee5555-0000-0000-0000-000000000002', 'Rae',    'MacIntosh', 'superintendent', '+1-604-555-0411', '2022-11-10')
ON CONFLICT (id) DO NOTHING;

INSERT INTO certifications (id, employee_id, cert_type, cert_number, issued_date, expiry_date, status) VALUES
  ('f0000001-0000-0000-0000-000000000001', 'e0000001-0000-0000-0000-000000000001', 'BC Housing Licensed Builder', 'BCH-2024-1188', '2024-03-04', CURRENT_DATE + 47,  'active'),
  ('f0000001-0000-0000-0000-000000000002', 'e0000001-0000-0000-0000-000000000002', 'BC Construction Safety Officer (NCSO)', 'NCSO-2023-4920', '2023-04-12', CURRENT_DATE - 10, 'expired'),
  ('f0000001-0000-0000-0000-000000000003', 'e0000001-0000-0000-0000-000000000003', 'Heritage Conservation Practitioner', 'HCP-2022-0044', '2022-11-10', CURRENT_DATE + 200, 'active')
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- Feed cards (the seed for the daily focus)
-- ============================================================
-- field_notification_dlq isn't used here — feed_cards are direct
-- service writes per A2 (Phase A2 of PRODUCTION_READINESS_PLAN.md).

INSERT INTO feed_cards
  (id, org_id, project_id, card_type, title, body, priority, target_user_id, target_role, actions, status)
VALUES
  ('aaaa0000-0000-0000-0000-000000000001', '33333333-3333-3333-3333-aaaaaaaaaaaa', 'a0000001-0000-0000-0000-000000000004',
    'permit.heritage_review', 'Vancouver heritage character review response due Wednesday',
    'Commercial Drive Fourplex submitted to City of Vancouver heritage 22 days ago. Response window opens Wednesday. Framing crew on standby; further delay pushes critical path into the rainy season.',
    'critical', 'cccc3333-0000-0000-0000-000000000001', NULL,
    '[{"label":"Mark approved","action_type":"permit.approved","payload":{"permit_id":"blw-cd-3"}},{"label":"Defer 1 week","action_type":"permit.defer","payload":{"weeks":1}}]'::jsonb,
    'active'),

  ('aaaa0000-0000-0000-0000-000000000002', '33333333-3333-3333-3333-aaaaaaaaaaaa', NULL,
    'lead.toolbox_inbound', 'New Toolbox lead — Sarah Chen, fourplex on East Pender',
    'SHBC Toolbox visitor picked BC Standardized #BCS-FX-04 for lot 2231 East Pender (Vancouver R1-1). Estimated budget $1.65M CAD.',
    'urgent', 'cccc3333-0000-0000-0000-000000000001', NULL,
    '[{"label":"Schedule pre-design review","action_type":"lead.schedule_review","payload":{"lead_id":"lead-001"}}]'::jsonb,
    'active'),

  ('aaaa0000-0000-0000-0000-000000000003', '22222222-2222-2222-2222-aaaaaaaaaaaa', 'a0000001-0000-0000-0000-000000000001',
    'permit.utility_blocked', 'BC Hydro underground trench in queue 47 days — schedule slip risk',
    'BC Hydro West queue at 11 weeks; project critical path was costed at 8 weeks. Currently 6 weeks behind on framing → MEP transition.',
    'critical', 'bbbb2222-0000-0000-0000-000000000001', NULL,
    '[]'::jsonb, 'active')
ON CONFLICT (id) DO NOTHING;

COMMIT;

-- ============================================================
-- Net-new tables for the SHBC fork (illustrative DDL only)
-- ============================================================
-- These would land as a separate migration in the fork. Shown here
-- inline so the seed file is self-contained reading.

/*
-- Municipality registry + bylaw rules
CREATE TABLE municipalities (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name            TEXT NOT NULL,
  province        TEXT NOT NULL DEFAULT 'BC',
  population      INTEGER,
  ssmuh_zone_examples TEXT[],
  bill_44_status  TEXT NOT NULL DEFAULT 'unknown',
  bill_25_status  TEXT NOT NULL DEFAULT 'unknown',
  permit_portal_url TEXT,
  average_permit_days INTEGER,
  quirks          JSONB NOT NULL DEFAULT '[]'::jsonb,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Per-project bylaw checklist instance (cloned from a per-municipality template)
CREATE TABLE bylaw_checklist_items (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  muni_id         UUID NOT NULL REFERENCES municipalities(id),
  label           TEXT NOT NULL,
  status          TEXT NOT NULL DEFAULT 'not_started',  -- not_started, in_progress, complete, not_applicable
  note            TEXT,
  completed_on    DATE,
  deadline        DATE,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_bylaw_checklist_project ON bylaw_checklist_items(project_id);

-- Curated design library (links to SHBC Toolbox + CMHC + BC Standardized)
CREATE TABLE design_library_entries (
  id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source               TEXT NOT NULL,        -- 'SHBC Toolbox' | 'CMHC HDC 2025' | 'BC Standardized 2024'
  code                 TEXT NOT NULL UNIQUE,
  name                 TEXT NOT NULL,
  typology             TEXT NOT NULL,
  approx_gsf           INTEGER NOT NULL,
  approx_cost_per_sf_cents INTEGER NOT NULL, -- Composite Currency convention; CAD only for now
  stories              NUMERIC(3,1),
  units                INTEGER NOT NULL,
  lot_min_sqm          INTEGER,
  tags                 TEXT[],
  external_url         TEXT,
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Cohort membership — many-to-many between SHBC the operator org and
-- builder orgs, with cohort metadata.
CREATE TABLE cohort_members (
  id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  cohort_label       TEXT NOT NULL,
  builder_org_id     UUID NOT NULL REFERENCES organizations(id),
  joined_at          DATE NOT NULL,
  available_for_subtrade BOOLEAN NOT NULL DEFAULT true,
  capacity_pct       INTEGER NOT NULL DEFAULT 0  CHECK (capacity_pct BETWEEN 0 AND 100),
  notes              TEXT,
  UNIQUE (cohort_label, builder_org_id)
);
*/
