CREATE TABLE orphan_table (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid()
);
-- intentionally no .down.sql to test rule 3
