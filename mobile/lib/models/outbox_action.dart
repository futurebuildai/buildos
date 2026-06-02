/// Outbox action kinds. Each maps to a caller-scoped field write endpoint
/// (internal/api/field.go). All carry a UUID `idempotency_key` so the server
/// dedups replays — a 409 from the server means "already accepted".
enum OutboxAction {
  progress('progress', '/api/v1/field/progress'),
  checkin('checkin', '/api/v1/field/checkin'),
  dailyLog('daily-log', '/api/v1/field/daily-log');

  const OutboxAction(this.wire, this.path);

  /// Stored in the Drift `action` column.
  final String wire;

  /// POST target.
  final String path;

  static OutboxAction fromWire(String wire) =>
      OutboxAction.values.firstWhere((a) => a.wire == wire);
}

/// Outbox item lifecycle (Drift `status` column).
enum OutboxStatus { pending, synced, failed }
