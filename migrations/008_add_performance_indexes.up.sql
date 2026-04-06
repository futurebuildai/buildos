-- Migration 008: Performance indexes for production query paths.
-- All indexes use IF NOT EXISTS for idempotency.

-- Users (OIDC subject lookups, org-scoped queries)
CREATE INDEX IF NOT EXISTS idx_users_oidc_subject ON users(oidc_subject);
CREATE INDEX IF NOT EXISTS idx_users_org ON users(org_id);

-- Projects (org-scoped, status filtering)
CREATE INDEX IF NOT EXISTS idx_projects_org ON projects(org_id);
CREATE INDEX IF NOT EXISTS idx_projects_status ON projects(status);

-- Project Tasks (CPM scheduling, critical path detection)
CREATE INDEX IF NOT EXISTS idx_tasks_project ON project_tasks(project_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON project_tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_project_critical ON project_tasks(project_id, is_critical) WHERE is_critical = true;

-- Task Dependencies (CPM forward/backward pass joins)
CREATE INDEX IF NOT EXISTS idx_task_deps_project ON task_dependencies(project_id);
CREATE INDEX IF NOT EXISTS idx_task_deps_predecessor ON task_dependencies(predecessor_id);
CREATE INDEX IF NOT EXISTS idx_task_deps_successor ON task_dependencies(successor_id);

-- Task Progress (idempotency lookups)
CREATE INDEX IF NOT EXISTS idx_task_progress_task ON task_progress(task_id);

-- Invoices (financial queries by project/org/status)
CREATE INDEX IF NOT EXISTS idx_invoices_project ON invoices(project_id);
CREATE INDEX IF NOT EXISTS idx_invoices_org ON invoices(org_id);
CREATE INDEX IF NOT EXISTS idx_invoices_status ON invoices(status);
CREATE INDEX IF NOT EXISTS idx_invoices_due_date ON invoices(due_date) WHERE status IN ('pending', 'approved');

-- Project Budgets (project-scoped financial rollups)
CREATE INDEX IF NOT EXISTS idx_budgets_project ON project_budgets(project_id);

-- Corporate Budgets (org + fiscal period queries)
CREATE INDEX IF NOT EXISTS idx_corp_budgets_org ON corporate_budgets(org_id, fiscal_year, quarter);

-- AR Aging Snapshots (org + date range)
CREATE INDEX IF NOT EXISTS idx_ar_aging_org ON ar_aging_snapshots(org_id, snapshot_date DESC);

-- Procurement Items (project-scoped, status transitions, must_order_date)
CREATE INDEX IF NOT EXISTS idx_procurement_project ON procurement_items(project_id);
CREATE INDEX IF NOT EXISTS idx_procurement_status ON procurement_items(status);
CREATE INDEX IF NOT EXISTS idx_procurement_must_order ON procurement_items(must_order_date) WHERE status IN ('ok', 'warning');

-- Feed Cards (org-scoped, status filtering, target routing)
CREATE INDEX IF NOT EXISTS idx_feed_org_status ON feed_cards(org_id, status);
CREATE INDEX IF NOT EXISTS idx_feed_target_user ON feed_cards(target_user_id) WHERE target_user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_feed_created ON feed_cards(created_at DESC);

-- Communication Logs (project-scoped sub-liaison queries)
CREATE INDEX IF NOT EXISTS idx_comms_project ON communication_logs(project_id);

-- Fleet Assets (org-scoped)
CREATE INDEX IF NOT EXISTS idx_fleet_assets_org ON fleet_assets(org_id);

-- Equipment Allocations (project-scoped lookups)
CREATE INDEX IF NOT EXISTS idx_equipment_alloc_project ON equipment_allocations(project_id);

-- Employees (org-scoped HR queries)
CREATE INDEX IF NOT EXISTS idx_employees_org ON employees(org_id);

-- Certifications (expiry alerting, employee lookups)
CREATE INDEX IF NOT EXISTS idx_certs_employee ON certifications(employee_id);
CREATE INDEX IF NOT EXISTS idx_certs_expiry ON certifications(expiry_date);

-- Pipeline Prospects (org-scoped, stage filtering)
CREATE INDEX IF NOT EXISTS idx_prospects_org ON pipeline_prospects(org_id);
CREATE INDEX IF NOT EXISTS idx_prospects_stage ON pipeline_prospects(pipeline_stage);

-- Estimates (prospect-scoped)
CREATE INDEX IF NOT EXISTS idx_estimates_prospect ON estimates(prospect_id);

-- Permits (prospect-scoped)
CREATE INDEX IF NOT EXISTS idx_permits_prospect ON permits(prospect_id);

-- A2A Webhook Log (idempotency key lookups)
CREATE INDEX IF NOT EXISTS idx_a2a_webhook_idempotency ON a2a_webhook_log(idempotency_key);

-- Field Notification DLQ (user-scoped retry queries)
CREATE INDEX IF NOT EXISTS idx_notification_dlq_user ON field_notification_dlq(user_id);
CREATE INDEX IF NOT EXISTS idx_notification_dlq_status ON field_notification_dlq(status) WHERE status = 'pending';

-- Field Checkins (user + project lookups)
CREATE INDEX IF NOT EXISTS idx_field_checkins_user ON field_checkins(user_id);
CREATE INDEX IF NOT EXISTS idx_field_checkins_project ON field_checkins(project_id);

-- Field Daily Logs (user + project + date lookups)
CREATE INDEX IF NOT EXISTS idx_field_daily_logs_user ON field_daily_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_field_daily_logs_project ON field_daily_logs(project_id);
