---
description: Plan data and schema migrations. Define migration sequence, rollback procedures, data transformation rules, zero-downtime strategy, validation queries, and emergency rollback procedures.
invoked_from:
  - workflows/antigravity/07_architecture_spec.md
  - workflows/antigravity/feature_iteration.md
  - workflows/claude_code/migration_execute.md
produces:
  - Migration sequence with dependency ordering
  - Rollback procedures for each migration step
  - Data transformation rules
  - Zero-downtime migration strategy
  - Validation queries to confirm success
  - Emergency rollback procedures
  - Pre-migration and post-migration checklists
browser_usage: None (relies on architecture, database design, and domain inputs)
---

# Skill: Migration Plan

Plan how to evolve the database schema and transform data without downtime, data loss, or corruption. Migrations are the most operationally dangerous changes in a system — a bad migration can take down production and lose data. Every step must be reversible, validated, and rehearsed.

---

## Prerequisites

Before invoking this skill, ensure the following exist:

- `TECH_STACK.md` — for database engine, ORM/migration tool
- `ARCHITECTURE.md` — for understanding current schema and target schema
- `database_design.md` output — for target table definitions
- Current schema documentation or access to the database to inspect the current state

---

## Step 1: Assess Migration Scope

### Migration Trigger

| Trigger | Description |
|---------|------------|
| New feature | Adding tables, columns, or indexes for new functionality |
| Schema evolution | Modifying existing tables (rename, type change, restructure) |
| Data backfill | Populating new columns with computed or default data |
| Performance optimization | Adding indexes, partitioning, denormalization |
| Data model refactor | Splitting/merging tables, changing relationships |
| Compliance requirement | Encrypting columns, adding audit fields, retention changes |

### Impact Assessment

| Factor | Assessment |
|--------|-----------|
| Tables affected | [List every table that will be modified] |
| Estimated row count affected | [Total rows across all affected tables] |
| Estimated migration duration | [Based on row count and operation type] |
| Downtime required | [None / Brief / Extended — with justification] |
| Data loss risk | [None / Low / Medium / High — with justification] |
| Rollback complexity | [Simple / Moderate / Complex] |
| Dependent services | [Services that must be coordinated] |

---

## Step 2: Define Migration Sequence

### Ordering Rules

1. **Create before reference:** Create new tables before adding foreign keys that reference them
2. **Add before populate:** Add new columns (nullable or with default) before backfilling data
3. **Backfill before constrain:** Populate data before adding NOT NULL or unique constraints
4. **New code before old schema removal:** Deploy code that handles both old and new schemas before removing old columns/tables
5. **Index after data:** Create indexes after bulk data operations (faster than maintaining index during bulk write)

### Migration Steps

For **each** migration in the sequence:

```markdown
### Migration [N]: [Description]

**Type:** Schema change / Data transformation / Index / Constraint
**Estimated duration:** [time based on row count and operation]
**Locking behavior:** [ACCESS EXCLUSIVE / ROW EXCLUSIVE / SHARE / None]
**Blocking writes:** [Yes / No — and for how long]
**Reversible:** [Yes / No — if no, explain why and define alternative rollback]

**Forward (UP):**
```sql
-- Description of what this does
BEGIN;

[SQL statements]

COMMIT;
```

**Backward (DOWN):**
```sql
-- Description of what the rollback does
BEGIN;

[SQL statements]

COMMIT;
```

**Validation query:**
```sql
-- Confirm the migration succeeded
[Query that returns expected result if migration is correct]
```

**Dependencies:**
- Requires migration [N-1] to be complete
- Requires [service] to be deployed with version [X] or later

**Risks:**
- [Specific risk and its mitigation]
```

---

## Step 3: Zero-Downtime Migration Patterns

### Pattern: Add Column (Safe)

```
Phase 1: Add nullable column (no lock on reads/writes)
Phase 2: Deploy code that writes to new column (backward-compatible)
Phase 3: Backfill existing rows
Phase 4: Add NOT NULL constraint with default (if needed)
Phase 5: Deploy code that reads from new column
```

### Pattern: Rename Column (Safe)

```
Phase 1: Add new column
Phase 2: Deploy code that writes to both old and new columns
Phase 3: Backfill new column from old column
Phase 4: Deploy code that reads from new column, still writes to both
Phase 5: Deploy code that only uses new column
Phase 6: Drop old column
```

**Never use `ALTER TABLE RENAME COLUMN` in production** — it breaks all existing queries referencing the old name instantly.

### Pattern: Change Column Type (Safe)

```
Phase 1: Add new column with target type
Phase 2: Deploy code that writes to both columns (cast on write)
Phase 3: Backfill new column (with type conversion)
Phase 4: Validate all data converted correctly
Phase 5: Deploy code that reads from new column
Phase 6: Drop old column
Phase 7: Rename new column to original name (optional — using the rename pattern)
```

### Pattern: Split Table (Safe)

```
Phase 1: Create new table
Phase 2: Deploy code that writes to both tables
Phase 3: Backfill new table from old table
Phase 4: Deploy code that reads from new table
Phase 5: Deploy code that stops writing to old columns
Phase 6: Drop old columns (after retention period)
```

### Pattern: Add Index (Safe)

```sql
-- Use CONCURRENTLY to avoid locking the table
CREATE INDEX CONCURRENTLY idx_name ON table_name (column);
```

**Note:** `CREATE INDEX CONCURRENTLY` cannot run inside a transaction. Handle separately from transactional migrations.

### Pattern: Add NOT NULL Constraint (Safe — PostgreSQL 12+)

```sql
-- Add constraint as NOT VALID first (instant, no table scan)
ALTER TABLE table_name ADD CONSTRAINT chk_col_not_null CHECK (column IS NOT NULL) NOT VALID;

-- Validate in background (no lock, scans table)
ALTER TABLE table_name VALIDATE CONSTRAINT chk_col_not_null;

-- Optionally convert to native NOT NULL (instant in PG 12+ if check exists)
ALTER TABLE table_name ALTER COLUMN column SET NOT NULL;
ALTER TABLE table_name DROP CONSTRAINT chk_col_not_null;
```

### Pattern: Add Foreign Key (Safe)

```sql
-- Add as NOT VALID (instant, no table scan)
ALTER TABLE child_table ADD CONSTRAINT fk_name
    FOREIGN KEY (column) REFERENCES parent_table(id) NOT VALID;

-- Validate in background (no exclusive lock)
ALTER TABLE child_table VALIDATE CONSTRAINT fk_name;
```

---

## Step 4: Data Transformation Rules

### Transformation Definitions

For each data transformation in the migration:

```markdown
### Transformation: [Description]

**Source:** [table.column or computed expression]
**Target:** [table.column]
**Transform logic:**
```sql
UPDATE target_table
SET new_column = [transformation expression]
WHERE [condition]
```

**Batch strategy:** Process [N] rows per batch with [M]ms delay between batches
**Null handling:** [What to do when source is NULL — default value / skip / error]
**Conflict handling:** [What to do on duplicate key — skip / update / error]
**Validation:**
```sql
-- Verify transformation correctness
SELECT COUNT(*) FROM target_table WHERE new_column IS NULL AND old_column IS NOT NULL;
-- Expected result: 0
```
```

### Batch Processing Strategy

For large data transformations, define the batching approach:

```sql
-- Example: Backfill in batches to avoid long-running transactions
DO $$
DECLARE
    batch_size INT := 10000;
    affected INT;
BEGIN
    LOOP
        UPDATE target_table
        SET new_column = [expression]
        WHERE id IN (
            SELECT id FROM target_table
            WHERE new_column IS NULL
            LIMIT batch_size
            FOR UPDATE SKIP LOCKED
        );

        GET DIAGNOSTICS affected = ROW_COUNT;
        EXIT WHEN affected = 0;

        -- Brief pause to allow other transactions
        PERFORM pg_sleep(0.1);

        RAISE NOTICE 'Processed % rows', affected;
    END LOOP;
END $$;
```

| Parameter | Value | Rationale |
|-----------|-------|-----------|
| Batch size | [N rows] | Balance between speed and transaction size |
| Delay between batches | [N ms] | Reduce replication lag and lock contention |
| Parallelism | [1 / N threads] | Single-threaded for safety unless proven safe |
| Estimated total duration | [time] | Based on row count and batch size |

---

## Step 5: Validation Queries

### Pre-Migration Validation

Run before executing any migration:

```sql
-- 1. Verify current schema matches expectations
SELECT column_name, data_type, is_nullable
FROM information_schema.columns
WHERE table_name = '[table]'
ORDER BY ordinal_position;
-- Expected: [list expected current schema]

-- 2. Verify row counts are in expected range
SELECT COUNT(*) FROM [table];
-- Expected: approximately [N] rows

-- 3. Verify no ongoing long-running transactions
SELECT pid, now() - pg_stat_activity.query_start AS duration, query
FROM pg_stat_activity
WHERE state = 'active' AND query NOT ILIKE '%pg_stat_activity%'
ORDER BY duration DESC;
-- Expected: no transactions running > 5 minutes

-- 4. Verify replication is caught up (if applicable)
SELECT client_addr, state, sent_lsn, write_lsn, flush_lsn, replay_lsn
FROM pg_stat_replication;
-- Expected: replay_lsn close to sent_lsn
```

### Post-Migration Validation

Run after each migration step:

```sql
-- 1. Verify new schema is correct
SELECT column_name, data_type, is_nullable, column_default
FROM information_schema.columns
WHERE table_name = '[table]'
ORDER BY ordinal_position;
-- Expected: [list expected new schema]

-- 2. Verify data integrity
SELECT COUNT(*) FROM [table] WHERE [new_column] IS NULL AND [condition_where_should_not_be_null];
-- Expected: 0

-- 3. Verify constraints are active
SELECT conname, convalidated
FROM pg_constraint
WHERE conrelid = '[table]'::regclass;
-- Expected: all constraints validated = true

-- 4. Verify indexes exist and are valid
SELECT indexname, indexdef
FROM pg_indexes
WHERE tablename = '[table]';
-- Expected: [list expected indexes]

-- 5. Verify foreign key integrity
SELECT COUNT(*)
FROM child_table c
LEFT JOIN parent_table p ON c.parent_id = p.id
WHERE p.id IS NULL AND c.parent_id IS NOT NULL;
-- Expected: 0 (no orphaned references)

-- 6. Spot-check transformed data
SELECT id, old_column, new_column
FROM [table]
ORDER BY RANDOM()
LIMIT 10;
-- Expected: transformation logic applied correctly
```

### Business Logic Validation

```sql
-- Verify application-specific invariants still hold
-- Example: every order has at least one line item
SELECT o.id
FROM orders o
LEFT JOIN order_items oi ON o.id = oi.order_id
GROUP BY o.id
HAVING COUNT(oi.id) = 0;
-- Expected: 0 rows (no empty orders)
```

---

## Step 6: Rollback Procedures

### Per-Step Rollback

Every migration step must have a documented rollback. If a step fails:

1. **Do NOT proceed to the next step**
2. Execute the rollback for the failed step
3. Verify the rollback with validation queries
4. Investigate the failure before retrying

### Rollback Strategies by Operation Type

| Operation | Rollback Strategy | Instant | Data Risk |
|-----------|------------------|---------|-----------|
| Add column | `ALTER TABLE DROP COLUMN` | Yes | None (data in new column lost — acceptable) |
| Add index | `DROP INDEX` | Yes | None |
| Add constraint | `ALTER TABLE DROP CONSTRAINT` | Yes | None |
| Rename column | Reverse rename (if using add-new/drop-old pattern) | Phased | None |
| Drop column | **IRREVERSIBLE** — must restore from backup | N/A | High |
| Data transformation | Reverse transformation query (if invertible) | No (takes time) | Low if idempotent |
| Drop table | **IRREVERSIBLE** — must restore from backup | N/A | High |

### Emergency Rollback Procedure

```markdown
## Emergency Rollback Runbook

### Trigger Conditions
- Migration script errors out mid-execution
- Application errors spike after migration
- Data corruption detected in validation queries
- Migration takes longer than [2x estimated duration]

### Immediate Actions (within 5 minutes)

1. **Stop the migration** if still running:
   ```sql
   -- Find the migration process
   SELECT pid, query FROM pg_stat_activity WHERE query ILIKE '%migration%';
   -- Cancel it
   SELECT pg_cancel_backend([pid]);
   -- If cancel does not work within 30 seconds:
   SELECT pg_terminate_backend([pid]);
   ```

2. **Assess the state:**
   - How many steps completed successfully?
   - Is the database in a consistent state?
   - Run validation queries for the last completed step

3. **Execute rollback:**
   - Roll back steps in **reverse order** from the last completed step
   - Run validation queries after each rollback step
   - Verify the schema matches the pre-migration state

4. **If rollback is not possible** (irreversible step was executed):
   - Activate the backup restoration plan
   - Estimate RPO (how much data since last backup)
   - Communicate to stakeholders

### Restore from Backup (Last Resort)

1. Identify the most recent backup before the migration
2. Restore to a new database instance (do not overwrite production)
3. Validate restored data with pre-migration validation queries
4. Redirect application to restored database
5. Replay any transactions between backup time and migration start (from WAL/binlog if available)

### Post-Rollback

1. Verify application is functioning correctly
2. Notify all stakeholders
3. Create incident report
4. Root cause analysis before reattempting
```

---

## Step 7: Migration Execution Plan

### Pre-Migration Checklist

- [ ] Migration scripts tested against a copy of production data
- [ ] Rollback scripts tested against post-migration state
- [ ] Validation queries prepared and tested
- [ ] Database backup completed and verified (can restore from it)
- [ ] Replication lag is minimal (< 100ms)
- [ ] No other deployments or migrations scheduled concurrently
- [ ] On-call engineer available and informed
- [ ] Stakeholders notified of migration window
- [ ] Monitoring dashboards open (error rate, latency, database metrics)
- [ ] Application code is compatible with both pre- and post-migration schema

### Execution Timeline

```markdown
T-60 min: Final backup, verify backup integrity
T-30 min: Run pre-migration validation queries
T-15 min: Notify team, open monitoring dashboards
T-10 min: Verify no long-running transactions
T-5 min:  Reduce background job concurrency (if applicable)
T-0:      Begin migration execution

Step 1:   [description] — estimated [N] seconds
          Run validation, proceed if passing

Step 2:   [description] — estimated [N] seconds
          Run validation, proceed if passing

...

T+N min:  All steps complete, run full post-migration validation
T+N+5:    Re-enable background jobs, restore normal operations
T+N+10:   Verify application metrics are nominal
T+N+30:   All-clear notification to team
T+24h:    Confirm no delayed issues, close migration ticket
```

### Post-Migration Checklist

- [ ] All validation queries pass
- [ ] Application error rate has not increased
- [ ] API latency is within SLA
- [ ] Database metrics are nominal (connections, CPU, replication)
- [ ] All dependent services are functioning correctly
- [ ] Background jobs are processing normally
- [ ] Old columns/tables scheduled for removal (if applicable) with ticket and date

---

## Step 8: Cross-Reference Validation

Before finalizing the migration plan, verify:

- [ ] Every schema change is covered by a numbered migration step
- [ ] Every migration step has a rollback procedure
- [ ] Every migration step has validation queries
- [ ] Migration sequence respects dependency ordering
- [ ] Zero-downtime patterns are used for all changes (or downtime is explicitly approved)
- [ ] Batch processing parameters are defined for large data transformations
- [ ] Emergency rollback procedure is documented and tested
- [ ] Migration has been rehearsed on a staging environment with production-like data
- [ ] Application code changes are deployed in the correct order relative to migrations

---

## Output

The final output is a complete migration plan that can be executed by the operations team or automated through the CI/CD pipeline. The plan feeds into `deployment_topology.md` deployment procedures and is referenced by `workflows/claude_code/migration_execute.md` for automated execution.
