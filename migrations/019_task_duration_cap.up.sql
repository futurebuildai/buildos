-- Phase 4c security hardening: bound project_tasks.duration_days.
--
-- physics.AddWorkDuration advances the schedule one calendar day at a time in a
-- Go loop whose bound is the task's duration_days (called for every task in the
-- forward AND backward CPM pass). The column was an unbounded INTEGER, so a
-- single absurd value (e.g. via import or the AI adjuster) could pin a CPU and
-- blow the bench gates — a cheap authenticated DoS. 36500 days = 100 years, far
-- beyond any real construction schedule. This CHECK guards EVERY write path
-- (create, import, AI apply, direct SQL), not just the service layer.
ALTER TABLE project_tasks
    ADD CONSTRAINT project_tasks_duration_days_sane
    CHECK (duration_days >= 0 AND duration_days <= 36500);
