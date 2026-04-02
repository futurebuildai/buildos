---
description: Safely execute database and schema migrations with dry-run preview, user approval gates, validation queries, and automatic rollback procedures. Never runs destructive operations without explicit confirmation.
inputs: MIGRATION_PLAN.md (from .agents/handoff/), ARCHITECTURE.md (schema definitions), TECH_STACK.md (migration tooling)
outputs: Migration scripts, updated PROJECT_STATE.md, validation results
---

# Workflow: Migration Execute

This workflow handles database schema changes with extreme care. Migrations are inherently dangerous -- a bad migration can destroy data or cause downtime. Every step includes verification, and destructive operations require explicit user approval.

---

## Step 1: Read the Migration Plan

Read `.agents/handoff/MIGRATION_PLAN.md` (produced by Antigravity or derived from ARCHITECTURE.md). Extract:

1. **What is changing:**
   - New tables/collections being created.
   - Columns/fields being added, modified, or removed.
   - Indexes being added or removed.
   - Constraints being added, modified, or removed.
   - Data transformations (backfills, type conversions).
   - Enum/type changes.

2. **Migration ordering:**
   - Dependencies between migrations (table A must exist before table B's foreign key).
   - Which migrations are safe to run concurrently vs. must be sequential.

3. **Risk assessment:**
   | Change | Risk Level | Reason |
   |--------|-----------|--------|
   | Add column (nullable) | Low | Non-breaking, no data loss |
   | Add column (NOT NULL, with default) | Medium | Lock time on large tables |
   | Drop column | High | Data loss, must verify no code references |
   | Alter column type | High | Potential data loss, lock time |
   | Drop table | Critical | Complete data loss |
   | Add index | Medium | Lock time on large tables |
   | Data backfill | High | Irreversible if not done carefully |

4. **Read TECH_STACK.md** for migration tooling:
   - Migration tool (goose, knex, alembic, prisma migrate, flyway, etc.).
   - Database type and version.
   - Connection configuration approach.

**Pause and present the migration summary and risk assessment to the user. Wait for approval before proceeding.**

---

## Step 2: Generate Migration Scripts

Create migration files using the project's migration tool conventions:

### File Naming
Follow the tool's naming convention:
- Sequential: `001_create_users_table.sql`, `002_add_email_index.sql`
- Timestamp: `20240115120000_create_users_table.sql`
- Framework-specific: per tool documentation.

### Migration File Structure
Each migration file must contain both **up** and **down** (rollback) operations:

```sql
-- UP Migration
-- Description: [what this migration does]
-- Risk: [Low/Medium/High/Critical]
-- Estimated duration: [for large tables]
-- Dependencies: [other migrations that must run first]

[SQL statements for the forward migration]

-- DOWN Migration (Rollback)
-- Warning: [any data that will be lost on rollback]

[SQL statements to reverse the migration]
```

### Migration Rules
1. **One logical change per migration file.** Do not combine unrelated schema changes.
2. **Rollback must be complete.** Every UP must have a corresponding DOWN that fully reverses it.
3. **Data migrations are separate from schema migrations.** Create schema first, backfill data second, drop old schema third.
4. **Use transactions where supported.** Wrap migration in a transaction so it either fully applies or fully rolls back.
5. **Handle large tables carefully:**
   - Add indexes CONCURRENTLY where supported.
   - Add columns as nullable first, backfill, then add NOT NULL constraint.
   - Avoid full table locks on tables with more than 1M rows.
6. **Never use DROP CASCADE** without explicit documentation of what will be dropped.
7. **Never modify a migration that has already been applied.** Create a new migration instead.

---

## Step 3: Validate Migration Scripts (Static Analysis)

Before running anything against a database, perform static validation:

### SQL Review Checklist
- [ ] All table/column names follow the project's naming convention (from ARCHITECTURE.md).
- [ ] Data types match ARCHITECTURE.md schema definitions exactly.
- [ ] Foreign key constraints reference existing tables/columns.
- [ ] Indexes are defined for all columns used in WHERE clauses and JOINs (per ARCHITECTURE.md).
- [ ] Default values are appropriate and match the domain.
- [ ] NOT NULL constraints are only on columns that truly require values.
- [ ] No raw SQL injection vectors (parameterized where applicable).
- [ ] Character set and collation are specified for text columns (if the database requires it).
- [ ] Enum values match the domain model exactly.

### Rollback Review Checklist
- [ ] Every UP has a corresponding DOWN.
- [ ] DOWN operations reverse UP operations completely.
- [ ] DOWN operations handle the case where data was inserted after UP (graceful handling, not errors).
- [ ] No DROP statements in DOWN that would lose data not recreated by the corresponding UP.

### Dependency Order Check
- [ ] Migrations are ordered so that dependencies are created before dependents.
- [ ] Foreign key targets exist before the foreign key is created.
- [ ] Indexes are created after the table/column they index.

---

## Step 4: Dry Run / Preview

Run the migration in preview mode without applying changes:

### For SQL-based tools
```bash
# Show what would be executed without actually running it
make db-migrate-dry-run   # or equivalent tool-specific command
```

### For ORM-based tools
```bash
# Generate the SQL that would be executed
prisma migrate diff       # or equivalent
alembic upgrade --sql     # or equivalent
```

### Preview Output Review
Present the complete SQL that will be executed to the user:

```markdown
### Migration Preview

#### Migration 001: [description]
```sql
[exact SQL that will run]
```

#### Migration 002: [description]
```sql
[exact SQL that will run]
```

**Estimated impact:**
- Tables created: [count]
- Tables modified: [count]
- Indexes added: [count]
- Estimated lock time: [duration estimate for large tables]
- Data at risk: [none / description of data affected]
```

**Pause for user approval.** The user must explicitly confirm before any migration runs against the database. Present the exact SQL and risk assessment. Do not proceed without a clear "yes."

---

## Step 5: Execute Migration

After receiving user approval:

### Pre-Migration Checks
1. **Verify database connectivity:**
   ```bash
   make db-status   # or equivalent connection test
   ```
2. **Verify current migration state:**
   ```bash
   make db-migrate-status   # show which migrations have been applied
   ```
3. **Backup reminder:** Inform the user that a backup should exist before proceeding. If this is a production database, **require** confirmation that a backup was taken.

### Run the Migration
```bash
make db-migrate   # or the tool-specific migration command
```

### Capture Output
Log the complete migration output including:
- Which migrations were applied.
- Execution time for each migration.
- Any warnings or notices from the database.
- Final migration state.

### If Migration Fails
1. **Stop immediately.** Do not attempt to continue with remaining migrations.
2. Check if the migration tool automatically rolled back the failed migration.
3. If not rolled back, assess whether a partial state exists.
4. Log the error in detail.
5. Proceed to the Rollback section (Step 7).

---

## Step 6: Run Validation Queries

After successful migration, verify the schema matches expectations:

### Schema Validation
Run queries to confirm the schema is correct:

```sql
-- Verify tables exist
SELECT table_name FROM information_schema.tables
WHERE table_schema = 'public' ORDER BY table_name;

-- Verify columns for each new/modified table
SELECT column_name, data_type, is_nullable, column_default
FROM information_schema.columns
WHERE table_name = '[table]' ORDER BY ordinal_position;

-- Verify indexes exist
SELECT indexname, indexdef FROM pg_indexes
WHERE tablename = '[table]';

-- Verify foreign keys
SELECT constraint_name, table_name, column_name,
       foreign_table_name, foreign_column_name
FROM information_schema.key_column_usage
JOIN information_schema.constraint_column_usage USING (constraint_name)
WHERE constraint_name LIKE '%fk%';
```

(Adapt queries for the specific database engine.)

### Data Validation (if data migration was performed)
```sql
-- Row counts match expectations
SELECT COUNT(*) FROM [table];

-- Backfilled data is correct (spot check)
SELECT * FROM [table] WHERE [backfilled_column] IS NOT NULL LIMIT 10;

-- No orphaned records
SELECT COUNT(*) FROM [child_table] c
LEFT JOIN [parent_table] p ON c.parent_id = p.id
WHERE p.id IS NULL;

-- No constraint violations
-- (Attempting to add a constraint will fail if data violates it)
```

### Application Compatibility
- [ ] Run `make build` -- application compiles with new schema.
- [ ] Run `make test` -- existing tests still pass.
- [ ] Run `make test-int` -- integration tests pass against the migrated database.

---

## Step 7: Rollback Procedures

Document and test rollback capability:

### Rollback Test (Development Only)
After verifying the migration is correct, test that rollback works:

```bash
# Roll back the migration
make db-rollback

# Verify schema returned to previous state
[run schema validation queries for previous state]

# Re-apply the migration
make db-migrate

# Verify schema is correct again
[run schema validation queries]
```

### Rollback Documentation
Create or update the rollback runbook:

```markdown
### Rollback: [Migration Name]

**Trigger conditions:** [when to roll back -- e.g., application errors, data corruption]

**Rollback command:**
```bash
make db-rollback STEP=[migration-id]
```

**Post-rollback verification:**
1. [Verification query or check]
2. [Verification query or check]

**Data implications:**
- [What data, if any, will be lost on rollback]
- [Whether manual data recovery is needed]

**Point of no return:**
- [If there is a point after which rollback is not possible, describe it]
- [e.g., "After migration 003 runs and drops the old column, rollback will lose data in that column"]
```

### Emergency Rollback (if applicable)
If the migration is for a production system, document the emergency rollback procedure:
1. Exact commands to run.
2. Expected duration.
3. Who to notify.
4. How to verify the rollback succeeded.

---

## Step 8: Document Results in PROJECT_STATE.md

Update `.agents/handoff/PROJECT_STATE.md`:

```markdown
## Migration: [Name/Description]
### Date: [ISO timestamp]
### Migrations Applied
| Migration | Description | Duration | Status |
|-----------|-------------|----------|--------|
| [id] | [description] | [time] | Applied |

### Schema Validation
| Check | Result |
|-------|--------|
| Tables created | [count] -- VERIFIED |
| Columns match spec | VERIFIED |
| Indexes created | [count] -- VERIFIED |
| Foreign keys | [count] -- VERIFIED |
| Data backfill | [count] rows -- VERIFIED |

### Rollback Tested
- Rollback command: [command]
- Rollback verified: YES/NO
- Rollback data implications: [description]

### Post-Migration Test Results
- Build: PASS
- Unit tests: PASS ([count])
- Integration tests: PASS ([count])
```

Also append to `.agents/handoff/AUDIT_LOGS.md`:

```markdown
## Migration Audit: [Description] -- [ISO timestamp]

### Security
- [ ] No credentials in migration files
- [ ] Permissions/grants are appropriate
- [ ] No SQL injection vectors

### Data Integrity
- [ ] Foreign key constraints are correct
- [ ] NOT NULL constraints are appropriate
- [ ] Default values are safe
- [ ] No data loss occurred

### Rollback
- [ ] Rollback scripts exist for all migrations
- [ ] Rollback was tested successfully
- [ ] Rollback documentation is complete

### Result: PASS/FAIL
```
