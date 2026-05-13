-- ============================================================
-- 010: Setup infrastructure — embedded onboarding wizard (MB-7 / W1)
-- ============================================================
-- When a customer deploys a fresh BuildOS fork (Kelbrook Construction
-- being the first), the first admin login lands on a guided wizard
-- that captures everything the business needs before any projects are
-- created. The devops side (clone, keygen, migrate, deploy) is
-- covered by docs/fork-onboarding.md. This migration models the
-- *business* side: tenant company configuration.
--
-- Wizard step → schema mapping:
--   1. Company info        → organizations (extended)
--   2. Users + roles       → users, employees (existing schema)
--   3. Trades              → trade_categories (new)
--   4. Cost codes          → cost_codes (new)
--   5. Working calendar    → working_calendars + holiday_overrides (new)
--   6. Permit jurisdictions → permit_jurisdictions (new)
--   7. Brain Hub integrations → (Brain-side; no BuildOS rows)
--   8. Review + complete   → organizations.onboarding_complete = true
--
-- Bootstrap edge case: the very first request has no logged-in user.
-- cmd/buildos-fork-init emits a one-time bootstrap_token baked into
-- the deploy secrets. The first admin presents it to
-- POST /api/v1/setup/bootstrap to claim the owner seat. The token
-- table here records the issued token + redemption state so the
-- wizard middleware can verify and one-shot-burn it.
--
-- Design notes:
--   * Every new row is org_id-scoped. The invariant is preserved even
--     though each fork is single-tenant — leaves the door open to the
--     future co-op variant per ADR-002.
--   * trade_categories.code is org-namespaced (UNIQUE(org_id, code)),
--     not globally unique — different orgs can use the same trade
--     short codes (e.g., "ELEC" for electrical) without collision.
--   * cost_codes mirrors CSI MasterFormat (NN-NN-NN division-section-
--     subsection). division is denormalized for fast filters; the
--     full code is the unique key.
--   * working_calendars.working_days is a SMALLINT bitmap (Mon=1<<0
--     through Sun=1<<6) so the SWIM physics engine in internal/physics
--     can branchless-test working-day membership during DHSM duration
--     scaling.
--   * holiday_overrides.date is DATE (not TIMESTAMPTZ) — holidays are
--     calendar-day concepts, not instants. The CPM engine treats a
--     holiday as an entire non-working day in the org's local TZ.
--   * permit_jurisdictions.permit_types and inspection_checklist are
--     JSONB rather than separate tables. The wizard stores them as
--     contractor-supplied free-form lists; future migrations can
--     normalize once the schema stabilizes from real-world usage.
--   * No monetary columns in this migration — onboarding captures
--     structural metadata, not financial values.

-- ------------------------------------------------------------
-- 1. Extend organizations with company-profile fields + setup gate
-- ------------------------------------------------------------
-- These columns are all nullable except onboarding_complete (default
-- false) because the wizard populates them step-by-step. A fresh fork
-- starts with onboarding_complete=false and the SetupGate middleware
-- (W2) returns 403 SETUP_INCOMPLETE on every operational route until
-- the wizard flips it.
ALTER TABLE organizations
    ADD COLUMN legal_name              TEXT,
    ADD COLUMN address                 TEXT,
    ADD COLUMN ein                     TEXT,
    ADD COLUMN company_type            TEXT,           -- e.g. "general_contractor", "sub", "design_build"
    ADD COLUMN region                  TEXT,           -- e.g. "US-CT" — drives weather defaults in SWIM physics
    ADD COLUMN onboarding_complete     BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN onboarding_completed_at TIMESTAMPTZ;

-- ------------------------------------------------------------
-- 2. Bootstrap tokens — one-shot owner-claim for first admin
-- ------------------------------------------------------------
-- cmd/buildos-fork-init (W2 update) emits a token into fork.yaml.
-- POST /api/v1/setup/bootstrap consumes it to mint the first owner
-- user when no JWT-authenticated user exists yet. token_hash is the
-- argon2id hash of the cleartext token; cleartext never lands in the
-- DB. redeemed_at flips on use and we never accept the same token
-- twice (the unique index + redeemed_at non-null check enforces).
CREATE TABLE setup_bootstrap_tokens (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    token_hash   TEXT NOT NULL UNIQUE,
    issued_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ NOT NULL,
    redeemed_at  TIMESTAMPTZ,
    redeemed_by  UUID REFERENCES users(id),

    CONSTRAINT setup_bootstrap_tokens_redeemed_consistency
        CHECK ((redeemed_at IS NULL) = (redeemed_by IS NULL))
);

-- "Find an unredeemed token by its hash" — the bootstrap claim path.
CREATE INDEX idx_setup_bootstrap_tokens_org ON setup_bootstrap_tokens(org_id) WHERE redeemed_at IS NULL; -- buildos:lock-ok: fresh table created in same migration

-- ------------------------------------------------------------
-- 3. Trade categories
-- ------------------------------------------------------------
-- Captured in wizard step 3. Seeded with the standard contractor set
-- (Electrical, Plumbing, HVAC, Carpentry, Masonry, Roofing, Painting,
-- GC labor) when the wizard step is submitted with seed_defaults=true.
-- Operators can add custom trades alongside the seeded set.
CREATE TABLE trade_categories (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    code          TEXT NOT NULL,           -- short code, e.g. "ELEC", "PLBG"
    name          TEXT NOT NULL,           -- display name, e.g. "Electrical"
    description   TEXT,
    is_default    BOOLEAN NOT NULL DEFAULT false,  -- true for the seeded set; informational
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT trade_categories_org_code_unique UNIQUE (org_id, code)
);

CREATE INDEX idx_trade_categories_org ON trade_categories(org_id); -- buildos:lock-ok: fresh table created in same migration

-- ------------------------------------------------------------
-- 4. Cost codes
-- ------------------------------------------------------------
-- CSI MasterFormat-style codes. Wizard step 4 lets the operator
-- pick divisions to seed (e.g., "01 General Requirements",
-- "03 Concrete", "26 Electrical") and optionally a per-org prefix.
-- Operators can override + add codes after the wizard.
CREATE TABLE cost_codes (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    code         TEXT NOT NULL,           -- e.g. "03-30-00" (Cast-in-Place Concrete)
    name         TEXT NOT NULL,
    division     TEXT NOT NULL,           -- e.g. "03 Concrete" — first 2-digit MasterFormat division
    parent_code  TEXT,                    -- nullable; "03-00-00" is a parent of "03-30-00"
    is_default   BOOLEAN NOT NULL DEFAULT false,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT cost_codes_org_code_unique UNIQUE (org_id, code)
);

CREATE INDEX idx_cost_codes_org ON cost_codes(org_id); -- buildos:lock-ok: fresh table created in same migration
CREATE INDEX idx_cost_codes_org_division ON cost_codes(org_id, division); -- buildos:lock-ok: fresh table created in same migration

-- ------------------------------------------------------------
-- 5. Working calendars + holiday overrides
-- ------------------------------------------------------------
-- The CPM physics engine (internal/physics/swim.go) consults the
-- org's working calendar when scaling task durations. working_days
-- is a 7-bit Mon-Sun bitmap; daily_work_minutes lets a fork model
-- non-8-hour-day defaults (e.g., field crews on 4x10 schedules).
-- holiday_overrides are the org's recognized non-working days
-- (federal, state, religious, custom). The physics engine treats
-- each row as a full non-working day in the org's local timezone.
--
-- Only one calendar may be the default per org (idx_working_calendars_
-- org_default_unique partial unique index). Future PRs may model
-- per-crew calendars but the wizard scope is one tenant calendar.
CREATE TABLE working_calendars (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name                TEXT NOT NULL,
    timezone            TEXT NOT NULL DEFAULT 'America/New_York',  -- IANA TZ
    working_days_mask   SMALLINT NOT NULL DEFAULT 31,              -- 0b0011111 = Mon-Fri (Mon=1<<0, Sun=1<<6)
    daily_work_minutes  INTEGER NOT NULL DEFAULT 480,              -- 8h default
    is_default          BOOLEAN NOT NULL DEFAULT false,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT working_calendars_mask_range
        CHECK (working_days_mask >= 0 AND working_days_mask <= 127),
    CONSTRAINT working_calendars_daily_minutes_positive
        CHECK (daily_work_minutes > 0 AND daily_work_minutes <= 1440)
);

CREATE INDEX idx_working_calendars_org ON working_calendars(org_id); -- buildos:lock-ok: fresh table created in same migration
CREATE UNIQUE INDEX idx_working_calendars_org_default_unique ON working_calendars(org_id) WHERE is_default = true; -- buildos:lock-ok: fresh table created in same migration

CREATE TABLE holiday_overrides (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    calendar_id  UUID NOT NULL REFERENCES working_calendars(id) ON DELETE CASCADE,
    org_id       UUID NOT NULL REFERENCES organizations(id),  -- denormalized for per-org sweep queries
    holiday_date DATE NOT NULL,
    name         TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT holiday_overrides_calendar_date_unique UNIQUE (calendar_id, holiday_date)
);

CREATE INDEX idx_holiday_overrides_calendar_date ON holiday_overrides(calendar_id, holiday_date); -- buildos:lock-ok: fresh table created in same migration

-- ------------------------------------------------------------
-- 6. Permit jurisdictions
-- ------------------------------------------------------------
-- Wizard step 6 captures the municipalities (and equivalents) the
-- contractor pulls permits in, the permit types each requires, and
-- the inspection checklist the org follows. permit_types and
-- inspection_checklist are JSONB — these are free-form lists that
-- vary widely by jurisdiction. Normalize in a future migration once
-- real-world usage stabilizes the shape.
CREATE TABLE permit_jurisdictions (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id                UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name                  TEXT NOT NULL,                      -- e.g. "Hartford CT Building Dept"
    region                TEXT,                                -- e.g. "US-CT"
    permit_types          JSONB NOT NULL DEFAULT '[]'::jsonb, -- array of permit type strings
    inspection_checklist  JSONB NOT NULL DEFAULT '[]'::jsonb, -- array of inspection-step objects
    notes                 TEXT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_permit_jurisdictions_org ON permit_jurisdictions(org_id); -- buildos:lock-ok: fresh table created in same migration
