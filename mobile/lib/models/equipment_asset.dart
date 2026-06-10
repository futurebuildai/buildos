/// Mirrors internal/models.FieldEquipment — the field-safe projection of a
/// fleet asset currently allocated to one of the caller's active sites
/// (Phase 4a-ii, read-only). Carries no org_id / cost — only what the field
/// needs to know which equipment is on site, plus the allocation window.
class EquipmentAsset {
  const EquipmentAsset({
    required this.id,
    required this.name,
    required this.assetType,
    required this.status,
    required this.startDate,
    required this.endDate,
    this.serialNumber,
  });

  final String id;
  final String name;
  final String assetType;
  final String status;
  final String? serialNumber;

  /// Allocation window [startDate, endDate); end is exclusive.
  final DateTime startDate;
  final DateTime endDate;

  factory EquipmentAsset.fromJson(Map<String, dynamic> json) => EquipmentAsset(
    id: json['id'] as String? ?? '',
    name: json['name'] as String? ?? '',
    assetType: json['asset_type'] as String? ?? '',
    status: json['status'] as String? ?? 'available',
    serialNumber: json['serial_number'] as String?,
    startDate:
        DateTime.tryParse(json['start_date'] as String? ?? '')?.toUtc() ??
        DateTime.fromMillisecondsSinceEpoch(0, isUtc: true),
    endDate:
        DateTime.tryParse(json['end_date'] as String? ?? '')?.toUtc() ??
        DateTime.fromMillisecondsSinceEpoch(0, isUtc: true),
  );
}
