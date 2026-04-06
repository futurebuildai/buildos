-- Tribunal decision tracking
CREATE TYPE tribunal_decision_status AS ENUM ('APPROVED', 'REJECTED', 'CONFLICT');
CREATE TYPE tribunal_vote_type AS ENUM ('YEA', 'NAY', 'ABSTAIN');

CREATE TABLE IF NOT EXISTS tribunal_decisions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    case_id         TEXT NOT NULL,
    category        TEXT NOT NULL,
    description     TEXT NOT NULL,
    status          tribunal_decision_status NOT NULL,
    reasoning       TEXT,
    cost_micros     BIGINT NOT NULL DEFAULT 0,
    currency_code   VARCHAR(3) NOT NULL DEFAULT 'USD',
    duration_ms     INTEGER NOT NULL DEFAULT 0,
    metadata        JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_tribunal_decisions_case ON tribunal_decisions(case_id);
CREATE INDEX IF NOT EXISTS idx_tribunal_decisions_status ON tribunal_decisions(status);
CREATE INDEX IF NOT EXISTS idx_tribunal_decisions_category ON tribunal_decisions(category);

CREATE TABLE IF NOT EXISTS tribunal_votes (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    decision_id     UUID NOT NULL REFERENCES tribunal_decisions(id) ON DELETE CASCADE,
    expert_role     TEXT NOT NULL,
    vote            tribunal_vote_type NOT NULL,
    reasoning       TEXT,
    confidence      REAL,
    model_used      TEXT,
    tokens_used     INTEGER,
    duration_ms     INTEGER,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_tribunal_votes_decision ON tribunal_votes(decision_id);

-- Shadow execution logs
CREATE TYPE shadow_execution_status AS ENUM ('PENDING', 'RUNNING', 'COMPLETED', 'FAILED');

CREATE TABLE IF NOT EXISTS shadow_execution_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    skill_id        TEXT NOT NULL,
    execution_id    TEXT NOT NULL,
    status          shadow_execution_status NOT NULL DEFAULT 'PENDING',
    input_params    JSONB,
    output_result   JSONB,
    error_message   TEXT,
    duration_ms     INTEGER,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at    TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_shadow_exec_skill ON shadow_execution_logs(skill_id);
CREATE INDEX IF NOT EXISTS idx_shadow_exec_status ON shadow_execution_logs(status);
