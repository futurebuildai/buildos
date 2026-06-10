import 'package:flutter/material.dart';

import '../theme/app_theme.dart';

/// A dashed rounded-rectangle border around [child] — the field app's
/// offline affordance (DESIGN_SYSTEM §10.1: a queued / offline-bound action is
/// marked with a dashed border, amber by default). Status is never conveyed by
/// colour alone, so callers pair this with a text caption + the FbSyncChip.
class FbDashedBorder extends StatelessWidget {
  const FbDashedBorder({
    super.key,
    required this.child,
    this.color = FbColors.amber,
    this.radius = FbSizes.radius,
    this.strokeWidth = 2,
    this.dashLength = 6,
    this.gapLength = 4,
    this.padding = const EdgeInsets.all(4),
  }) : assert(
         dashLength > 0 && gapLength >= 0,
         'dashLength must be > 0 and gapLength >= 0 so the dash walk always advances',
       );

  final Widget child;
  final Color color;
  final double radius;
  final double strokeWidth;
  final double dashLength;
  final double gapLength;

  /// Inset between the dashed line and [child] so the dashes sit OUTSIDE a
  /// filled child (e.g. a FilledButton) rather than under its fill.
  final EdgeInsetsGeometry padding;

  @override
  Widget build(BuildContext context) {
    return CustomPaint(
      painter: _DashedBorderPainter(
        color: color,
        radius: radius,
        strokeWidth: strokeWidth,
        dashLength: dashLength,
        gapLength: gapLength,
      ),
      child: Padding(padding: padding, child: child),
    );
  }
}

class _DashedBorderPainter extends CustomPainter {
  _DashedBorderPainter({
    required this.color,
    required this.radius,
    required this.strokeWidth,
    required this.dashLength,
    required this.gapLength,
  });

  final Color color;
  final double radius;
  final double strokeWidth;
  final double dashLength;
  final double gapLength;

  @override
  void paint(Canvas canvas, Size size) {
    // Defensive: in release builds the constructor assert is stripped, so guard
    // the walk invariant here too — a non-advancing step would spin forever.
    if (dashLength <= 0 || gapLength < 0) return;
    final paint = Paint()
      ..color = color
      ..strokeWidth = strokeWidth
      ..style = PaintingStyle.stroke;
    final inset = strokeWidth / 2;
    final rrect = RRect.fromRectAndRadius(
      Rect.fromLTWH(
        inset,
        inset,
        size.width - strokeWidth,
        size.height - strokeWidth,
      ),
      Radius.circular(radius),
    );
    final source = Path()..addRRect(rrect);
    for (final metric in source.computeMetrics()) {
      var distance = 0.0;
      while (distance < metric.length) {
        final next = distance + dashLength;
        canvas.drawPath(
          metric.extractPath(distance, next.clamp(0, metric.length)),
          paint,
        );
        distance = next + gapLength;
      }
    }
  }

  @override
  bool shouldRepaint(covariant _DashedBorderPainter old) =>
      old.color != color ||
      old.radius != radius ||
      old.strokeWidth != strokeWidth ||
      old.dashLength != dashLength ||
      old.gapLength != gapLength;
}
