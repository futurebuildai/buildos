-- 023_project_client_contact.down.sql
-- buildos:destructive: revert client-contact columns on projects (Chunk D)
ALTER TABLE projects DROP COLUMN IF EXISTS client_phone;
ALTER TABLE projects DROP COLUMN IF EXISTS client_email;
ALTER TABLE projects DROP COLUMN IF EXISTS client_name;
