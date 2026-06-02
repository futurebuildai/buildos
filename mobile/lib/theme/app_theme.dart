import 'package:flutter/material.dart';

/// GableLBM "Industrial Dark" theme — the field-app flavor (DESIGN_SYSTEM §14,
/// DESIGN_SYSTEM_COMPONENTS §2/§8).
///
/// Field constraints that differ from the web console:
/// - **Dark-only.** No light mode, no toggle.
/// - **Solid surfaces, not glass.** Glassmorphism is a console aesthetic;
///   sunlight glare makes low-contrast glass unreadable in the field.
/// - **AAA contrast (≥7:1) for primary content** — body text never uses the
///   muted slate token.
/// - **Glove-friendly targets:** controls are ≥56px tall; the one primary
///   action per screen is 64px.
/// - Status is never communicated by color alone — pair color with text/icon.
class FbColors {
  FbColors._();

  /// Background canvas.
  static const deepSpace = Color(0xFF0A0B10);

  /// Card / panel surfaces (solid, not translucent).
  static const slateSteel = Color(0xFF161821);
  static const slateSteelRaised = Color(0xFF1E212C);

  /// Active, success, critical-path, completion.
  static const gableGreen = Color(0xFF00FFA3);

  /// Critical / overdue / error.
  static const safetyRed = Color(0xFFF43F5E);

  /// Warning / near-critical float / approaching deadline.
  static const amber = Color(0xFFF59E0B);

  /// Info / slack / pending.
  static const blueprintBlue = Color(0xFF38BDF8);

  /// Neutral / disabled / offline / muted (NOT for primary field text).
  static const slate = Color(0xFF5A5B66);

  /// AAA-contrast text on the dark canvas.
  static const textPrimary = Color(0xFFF5F6FA);
  static const textSecondary = Color(0xFFC4C7D4);

  /// Hairline borders.
  static const border = Color(0xFF2A2D3A);
}

/// Touch-target sizes (DESIGN_SYSTEM_COMPONENTS §8).
class FbSizes {
  FbSizes._();

  /// Minimum field touch target.
  static const double touchTarget = 56;

  /// Primary "one action per screen" target.
  static const double primaryTarget = 64;

  static const double radius = 12;
  static const double radiusLarge = 20;

  static const double gap = 16;
  static const double gapSmall = 8;
}

/// Monospace family for numerics/IDs/timestamps (JetBrains Mono per the design
/// system); falls back to the platform monospace when the font asset is absent.
const String fbMonoFamily = 'JetBrainsMono';

ThemeData buildFieldTheme() {
  const scheme = ColorScheme.dark(
    primary: FbColors.gableGreen,
    onPrimary: FbColors.deepSpace,
    secondary: FbColors.blueprintBlue,
    onSecondary: FbColors.deepSpace,
    surface: FbColors.slateSteel,
    onSurface: FbColors.textPrimary,
    error: FbColors.safetyRed,
    onError: FbColors.deepSpace,
  );

  final base = ThemeData(
    useMaterial3: true,
    brightness: Brightness.dark,
    colorScheme: scheme,
    scaffoldBackgroundColor: FbColors.deepSpace,
    canvasColor: FbColors.deepSpace,
  );

  return base.copyWith(
    textTheme: base.textTheme.apply(
      bodyColor: FbColors.textPrimary,
      displayColor: FbColors.textPrimary,
    ),
    appBarTheme: const AppBarTheme(
      backgroundColor: FbColors.slateSteel,
      foregroundColor: FbColors.textPrimary,
      elevation: 0,
      centerTitle: false,
    ),
    cardTheme: CardThemeData(
      color: FbColors.slateSteel,
      elevation: 0,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(FbSizes.radius),
        side: const BorderSide(color: FbColors.border),
      ),
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        minimumSize: const Size.fromHeight(FbSizes.primaryTarget),
        backgroundColor: FbColors.gableGreen,
        foregroundColor: FbColors.deepSpace,
        textStyle: const TextStyle(fontSize: 18, fontWeight: FontWeight.w600),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(FbSizes.radius),
        ),
      ),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        minimumSize: const Size.fromHeight(FbSizes.touchTarget),
        foregroundColor: FbColors.textPrimary,
        side: const BorderSide(color: FbColors.border),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(FbSizes.radius),
        ),
      ),
    ),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: FbColors.slateSteelRaised,
      constraints: const BoxConstraints(minHeight: FbSizes.touchTarget),
      labelStyle: const TextStyle(color: FbColors.textSecondary),
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(FbSizes.radius),
        borderSide: const BorderSide(color: FbColors.border),
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(FbSizes.radius),
        borderSide: const BorderSide(color: FbColors.border),
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(FbSizes.radius),
        borderSide: const BorderSide(color: FbColors.gableGreen, width: 2),
      ),
    ),
    navigationBarTheme: NavigationBarThemeData(
      backgroundColor: FbColors.slateSteel,
      indicatorColor: FbColors.gableGreen.withValues(alpha: 0.18),
      height: FbSizes.primaryTarget + 8,
      labelTextStyle: WidgetStateProperty.all(
        const TextStyle(fontSize: 13, fontWeight: FontWeight.w600),
      ),
    ),
    dividerColor: FbColors.border,
  );
}
