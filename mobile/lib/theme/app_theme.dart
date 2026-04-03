import 'package:flutter/material.dart';

/// GableLBM Industrial Dark theme for FutureBuild Field Portal.
///
/// Dark mode only. No light theme exists.
/// Color tokens sourced from the GableLBM Design System.
class AppTheme {
  AppTheme._();

  // ── Core Palette ──────────────────────────────────────────────
  static const Color gableGreen = Color(0xFF00FFA3);
  static const Color deepSpace = Color(0xFF0A0B10);
  static const Color slateSteel = Color(0xFF161821);
  static const Color blueprintBlue = Color(0xFF38BDF8);
  static const Color safetyRed = Color(0xFFF43F5E);
  static const Color amberWarning = Color(0xFFF59E0B);

  // ── Surface Elevation Scale ───────────────────────────────────
  static const Color surface0 = Color(0xFF0A0B10);
  static const Color surface1 = Color(0xFF161821);
  static const Color surface2 = Color(0xFF1E2029);
  static const Color surface3 = Color(0xFF252836);

  // ── Text Colors ───────────────────────────────────────────────
  static const Color onBackground = Color(0xFFF0F0F5);
  static const Color onSurfaceVariant = Color(0xFF8B8D98);
  static const Color onPrimary = Color(0xFF003822);

  // ── Border Colors ─────────────────────────────────────────────
  static const Color outline = Color(0xFF5A5B66);
  static const Color outlineVariant = Color(0x0DFFFFFF); // 5% white

  // ── Typography ────────────────────────────────────────────────
  /// JetBrains Mono for numerical/data fields. Falls back to monospace.
  static const String monoFontFamily = 'JetBrains Mono';
  static const List<String> monoFontFallback = ['monospace', 'Courier New'];

  /// Outfit for UI labels. Falls back to system sans-serif.
  static const String labelFontFamily = 'Outfit';
  static const List<String> labelFontFallback = ['Roboto', 'sans-serif'];

  /// Monospaced text style for numerical data.
  static TextStyle monoStyle({
    double fontSize = 14,
    FontWeight fontWeight = FontWeight.w400,
    Color color = onBackground,
  }) {
    return TextStyle(
      fontFamily: monoFontFamily,
      fontFamilyFallback: monoFontFallback,
      fontSize: fontSize,
      fontWeight: fontWeight,
      color: color,
    );
  }

  // ── Theme Data ────────────────────────────────────────────────
  static ThemeData get darkTheme {
    return ThemeData.dark().copyWith(
      scaffoldBackgroundColor: deepSpace,
      primaryColor: gableGreen,
      colorScheme: const ColorScheme.dark(
        primary: gableGreen,
        onPrimary: onPrimary,
        secondary: blueprintBlue,
        onSecondary: deepSpace,
        error: safetyRed,
        onError: Colors.white,
        surface: slateSteel,
        onSurface: onBackground,
        outline: outline,
        outlineVariant: outlineVariant,
      ),
      appBarTheme: const AppBarTheme(
        backgroundColor: slateSteel,
        foregroundColor: onBackground,
        elevation: 0,
        centerTitle: false,
        titleTextStyle: TextStyle(
          fontFamily: labelFontFamily,
          fontFamilyFallback: labelFontFallback,
          fontSize: 20,
          fontWeight: FontWeight.w600,
          color: onBackground,
        ),
      ),
      bottomNavigationBarTheme: const BottomNavigationBarThemeData(
        backgroundColor: slateSteel,
        selectedItemColor: gableGreen,
        unselectedItemColor: onSurfaceVariant,
        type: BottomNavigationBarType.fixed,
        elevation: 0,
      ),
      cardTheme: CardThemeData(
        color: surface1,
        elevation: 0,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(12),
          side: const BorderSide(color: outlineVariant),
        ),
        margin: const EdgeInsets.symmetric(horizontal: 16, vertical: 6),
      ),
      chipTheme: ChipThemeData(
        backgroundColor: surface2,
        labelStyle: const TextStyle(
          fontFamily: labelFontFamily,
          fontFamilyFallback: labelFontFallback,
          fontSize: 12,
          color: onBackground,
        ),
        side: const BorderSide(color: outlineVariant),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(8),
        ),
      ),
      elevatedButtonTheme: ElevatedButtonThemeData(
        style: ElevatedButton.styleFrom(
          backgroundColor: gableGreen,
          foregroundColor: onPrimary,
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(10),
          ),
          textStyle: const TextStyle(
            fontFamily: labelFontFamily,
            fontFamilyFallback: labelFontFallback,
            fontWeight: FontWeight.w600,
            fontSize: 16,
          ),
          padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 14),
        ),
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: surface2,
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(10),
          borderSide: const BorderSide(color: outlineVariant),
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(10),
          borderSide: const BorderSide(color: outlineVariant),
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(10),
          borderSide: const BorderSide(color: gableGreen, width: 1.5),
        ),
        labelStyle: const TextStyle(color: onSurfaceVariant),
        hintStyle: const TextStyle(color: onSurfaceVariant),
        contentPadding:
            const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
      ),
      bottomSheetTheme: const BottomSheetThemeData(
        backgroundColor: surface1,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
        ),
      ),
      snackBarTheme: SnackBarThemeData(
        backgroundColor: surface3,
        contentTextStyle: const TextStyle(color: onBackground),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(10),
        ),
        behavior: SnackBarBehavior.floating,
      ),
      dividerTheme: const DividerThemeData(
        color: outlineVariant,
        thickness: 1,
      ),
      textTheme: const TextTheme(
        headlineLarge: TextStyle(
          fontFamily: labelFontFamily,
          fontFamilyFallback: labelFontFallback,
          fontSize: 28,
          fontWeight: FontWeight.w700,
          color: onBackground,
        ),
        headlineMedium: TextStyle(
          fontFamily: labelFontFamily,
          fontFamilyFallback: labelFontFallback,
          fontSize: 24,
          fontWeight: FontWeight.w600,
          color: onBackground,
        ),
        titleLarge: TextStyle(
          fontFamily: labelFontFamily,
          fontFamilyFallback: labelFontFallback,
          fontSize: 20,
          fontWeight: FontWeight.w600,
          color: onBackground,
        ),
        titleMedium: TextStyle(
          fontFamily: labelFontFamily,
          fontFamilyFallback: labelFontFallback,
          fontSize: 16,
          fontWeight: FontWeight.w500,
          color: onBackground,
        ),
        bodyLarge: TextStyle(
          fontFamily: labelFontFamily,
          fontFamilyFallback: labelFontFallback,
          fontSize: 16,
          fontWeight: FontWeight.w400,
          color: onBackground,
        ),
        bodyMedium: TextStyle(
          fontFamily: labelFontFamily,
          fontFamilyFallback: labelFontFallback,
          fontSize: 14,
          fontWeight: FontWeight.w400,
          color: onBackground,
        ),
        bodySmall: TextStyle(
          fontFamily: labelFontFamily,
          fontFamilyFallback: labelFontFallback,
          fontSize: 12,
          fontWeight: FontWeight.w400,
          color: onSurfaceVariant,
        ),
        labelLarge: TextStyle(
          fontFamily: labelFontFamily,
          fontFamilyFallback: labelFontFallback,
          fontSize: 14,
          fontWeight: FontWeight.w600,
          color: onBackground,
        ),
        labelMedium: TextStyle(
          fontFamily: labelFontFamily,
          fontFamilyFallback: labelFontFallback,
          fontSize: 12,
          fontWeight: FontWeight.w500,
          color: onSurfaceVariant,
        ),
        labelSmall: TextStyle(
          fontFamily: labelFontFamily,
          fontFamilyFallback: labelFontFallback,
          fontSize: 11,
          fontWeight: FontWeight.w500,
          color: onSurfaceVariant,
        ),
      ),
    );
  }
}
