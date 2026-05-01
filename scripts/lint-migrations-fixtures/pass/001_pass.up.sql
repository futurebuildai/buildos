CREATE TABLE pass_table (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    amount_cents    BIGINT NOT NULL DEFAULT 0,
    currency_code   VARCHAR(3) NOT NULL DEFAULT 'USD',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_pass_table_created ON pass_table(created_at DESC); -- buildos:lock-ok: fresh table created in same migration
CREATE INDEX CONCURRENTLY idx_pass_table_amount ON pass_table(amount_cents);
