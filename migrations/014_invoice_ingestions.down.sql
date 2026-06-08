-- migrations/014_invoice_ingestions.down.sql
-- buildos:destructive: rollback of 2a invoice-ingestion outbox + invoices.source provenance column
ALTER TABLE invoices DROP COLUMN source;
DROP TABLE invoice_ingestions;
