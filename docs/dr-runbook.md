# Disaster Recovery Runbook

BuildOS is **single-tenant per customer fork** ([ADR-002](../.agents/handoff/ADR-002-single-tenant-fork-model.md)):
each customer runs their own deployment against their own Postgres database.
Backup and restore are therefore a **per-fork operational concern** — there is
no central fleet backup job, and one fork's recovery never touches another's.

This runbook covers the logical-backup tooling shipped in
[`scripts/backup-db.sh`](../scripts/backup-db.sh) and
[`scripts/restore-db.sh`](../scripts/restore-db.sh), the recovery objectives,
the restore drill, and the failure playbook.

> Logical (`pg_dump`) backups are the baseline every fork gets for free. For
> tighter RPO than the snapshot cadence below, layer **continuous archiving /
> PITR** (WAL archiving + base backups) or your managed provider's
> point-in-time restore on top — see "Tightening RPO" at the end.

---

## Recovery objectives

| Objective | Target (logical baseline) | How to tighten |
| --- | --- | --- |
| **RPO** (max data loss) | ≤ 24h with a daily snapshot; ≤ 1h with hourly | WAL archiving / managed PITR → seconds–minutes |
| **RTO** (time to restore) | minutes for a small fork (single `pg_restore`) | pre-provisioned standby / managed failover |

Pick the snapshot cadence from the customer's tolerance: a daily cron is the
default; an hourly River/CronJob schedule tightens RPO at the cost of more
dump volume (pruned by retention).

---

## What the tooling does

### `scripts/backup-db.sh`
- `pg_dump -Fc` (compressed custom format) → `BACKUP_DIR/buildos-<db>-<UTC>.dump`.
- Writes a `<dump>.sha256` integrity sidecar.
- Optional storage-agnostic upload via `BACKUP_UPLOAD_CMD` (the literal
  `{file}` is replaced with the dump path) — no cloud SDK is a hard dependency.
- Prunes old local backups by the **timestamp embedded in the filename** (not
  mtime, so copying backups around can't change what survives): drops anything
  older than `BACKUP_RETENTION_DAYS`, but always keeps at least
  `BACKUP_RETAIN_MIN` of the most recent (a stalled backup job can never erase
  the last good copy).

| Env | Default | Meaning |
| --- | --- | --- |
| `DATABASE_URL` | local dev DSN | database to back up |
| `BACKUP_DIR` | `./backups` | destination directory |
| `BACKUP_RETENTION_DAYS` | `30` | local age window |
| `BACKUP_RETAIN_MIN` | `7` | floor of most-recent backups always kept |
| `BACKUP_UPLOAD_CMD` | _(unset)_ | e.g. `aws s3 cp {file} s3://acme-buildos-backups/db/` |

Modes: `--prune-only` (retention sweep, no dump), `--no-prune` (dump only).

### `scripts/restore-db.sh`
- Verifies the `.sha256` sidecar (if present) and **aborts on mismatch** — a
  corrupt backup must never half-restore over a live database.
- **Destructive-op guard:** refuses unless `--confirm` (or
  `BACKUP_RESTORE_CONFIRM=1`) is given, the same opt-in posture the migration
  linter forces for destructive DDL.
- `pg_restore --clean --if-exists --no-owner --no-privileges` into the target
  `DATABASE_URL`.

### Make targets
```bash
make backup-db                 # dump + sidecar + upload + prune
make backup-db PRUNE_ONLY=1    # retention sweep only
make restore-db DUMP=path/to.dump CONFIRM=1
make backup-db-test            # DB-free regression suite (part of `make audit`)
```

---

## Scheduling a fork's backups

Pick one; all just invoke `backup-db.sh` on a timer.

**System cron (host/VM deployment), daily 02:15 UTC:**
```cron
15 2 * * *  cd /opt/buildos && BACKUP_DIR=/var/backups/buildos \
  BACKUP_UPLOAD_CMD='aws s3 cp {file} s3://acme-buildos-backups/db/' \
  bash scripts/backup-db.sh >> /var/log/buildos-backup.log 2>&1
```

**Kubernetes CronJob:** run the same image with `BUILDOS_ROLE` unset and the
entrypoint overridden to `bash scripts/backup-db.sh`, mounting a PVC (or using
the upload hook to ship straight to object storage), `DATABASE_URL` from the
same secret the server uses.

**Object-store lifecycle for GFS tiering:** keep the *local* retention small
(the floor + a few days) and let the bucket's lifecycle rules do
grandfather-father-son tiering (e.g. transition to cold storage after 30d,
expire after 365d). Reimplementing GFS rotation in the script is intentionally
avoided — lifecycle rules are the idiomatic, auditable place for it.

---

## Restore drill (run quarterly + before any risky migration)

A backup you have never restored is a hope, not a backup. Drill against a
**throwaway** target, never production:

1. Provision an empty Postgres (e.g. `make db-up` locally, or a scratch
   instance) and point `DATABASE_URL` at it.
2. Pick the dump to verify: the latest in `BACKUP_DIR`, or pull one from object
   storage. Ensure its `.sha256` sidecar is alongside it.
3. Restore: `make restore-db DUMP=<dump> CONFIRM=1`. The integrity check must
   pass and `pg_restore` must complete without errors.
4. Boot the server against the restored DB and hit `GET /ready` (DB-backed
   readiness) — it must return 200.
5. Spot-check row counts on a couple of core tables (`organizations`, `projects`,
   `audit_log`) against expectations.
6. Record the drill (date, dump timestamp, RTO observed) in the fork's ops log.

---

## Failure playbook

**Primary database is down / corrupt:**
1. Stop the `server` and `worker` so they don't write to a half-dead DB.
2. Provision a fresh empty Postgres; set `DATABASE_URL` to it.
3. Restore the most recent **integrity-verified** dump:
   `make restore-db DUMP=<latest> CONFIRM=1`.
4. Run migrations (idempotent) to ensure schema is current: `make migrate`.
5. Boot `server`; confirm `GET /ready` is 200; confirm `worker` reconnects.
6. Communicate the data-loss window (gap between the dump timestamp and the
   incident) to the customer.

**A backup fails to verify (checksum mismatch):** do NOT restore it. Fall back
to the previous good dump (the `BACKUP_RETAIN_MIN` floor guarantees one exists
locally) and investigate the storage/transfer path.

**Backups stopped running:** the retention floor means old-but-valid dumps are
still present; re-enable the schedule and take an immediate manual
`make backup-db`. Alert on backup-job success in the scheduler — a silent
backup failure is the most common DR trap.

---

## Tightening RPO beyond daily/hourly snapshots

Logical dumps cap RPO at the snapshot cadence. When a fork needs
seconds-to-minutes RPO:
- **WAL archiving + base backups** (`pgBackRest` / `wal-g`) for true PITR, or
- the **managed provider's PITR** (RDS/Cloud SQL/Neon) — usually the lowest-ops
  path for a single-fork deployment.

The logical backups here remain valuable as a portable, provider-independent
escape hatch (and the only restore path that survives "the whole provider
account is gone").
