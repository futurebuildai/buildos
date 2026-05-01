CREATE TABLE indexed_table (id UUID PRIMARY KEY);
CREATE INDEX idx_indexed_table_id ON indexed_table(id);
-- intentionally no CONCURRENTLY and no buildos:lock-ok comment
