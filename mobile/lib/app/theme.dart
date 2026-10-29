import 'package:flutter/material.dart';

ThemeData buildTheme(Brightness brightness) {
  final scheme = ColorScheme.fromSeed(
    seedColor: const Color(0xFF14407A),
    brightness: brightness,
  );
  return ThemeData(
    colorScheme: scheme,
    useMaterial3: true,
    visualDensity: VisualDensity.standard,
    // Target sentuh minimal 48dp untuk aksesibilitas.
    materialTapTargetSize: MaterialTapTargetSize.padded,
  );
}
