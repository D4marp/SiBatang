import 'package:flutter/material.dart';

import '../core/credential_store.dart';
import '../core/device_api.dart';
import '../core/models.dart';
import '../modes/announcer/announcer_controller.dart';
import '../modes/announcer/announcer_screen.dart';
import '../modes/announcer/voice_announcer.dart';
import '../modes/display/display_controller.dart';
import '../modes/display/display_screen.dart';
import '../modes/kiosk/kiosk_screen.dart';
import '../modes/kiosk/ticket_printer.dart';
import '../modes/operator/operator_screen.dart';

class DesktopApp extends StatefulWidget {
  const DesktopApp({super.key});

  @override
  State<DesktopApp> createState() => _DesktopAppState();
}

class _DesktopAppState extends State<DesktopApp> {
  final _store = const CredentialStore();
  late Future<DeviceIdentity?> _identity = _store.load();

  Future<void> _activate(String code) async {
    final id = await DeviceApi().activate(code);
    await _store.save(id);
    setState(() => _identity = Future.value(id));
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'SIBATANG Desktop',
      theme: ThemeData(colorSchemeSeed: const Color(0xFF14407A), useMaterial3: true),
      home: FutureBuilder<DeviceIdentity?>(
        future: _identity,
        builder: (context, snap) {
          if (snap.connectionState != ConnectionState.done) return const Scaffold(body: Center(child: CircularProgressIndicator()));
          final id = snap.data;
          if (id == null) return _EnrollScreen(onActivate: _activate);
          final api = DeviceApi(identity: id);
          // Mode berasal dari catatan perangkat di server (device_type), bukan flag yang bisa diubah pengguna.
          return switch (id.deviceType) {
            'ANNOUNCER' => AnnouncerScreen(controller: AnnouncerController(api: api, voice: FlutterTtsAnnouncer())),
            'DISPLAY' => DisplayScreen(controller: DisplayController(api)),
            'KIOSK' => KioskScreen(api: api, printer: MockPrinter()),
            _ => const OperatorScreen(),
          };
        },
      ),
    );
  }
}

class _EnrollScreen extends StatefulWidget {
  const _EnrollScreen({required this.onActivate});
  final Future<void> Function(String) onActivate;

  @override
  State<_EnrollScreen> createState() => _EnrollScreenState();
}

class _EnrollScreenState extends State<_EnrollScreen> {
  final _code = TextEditingController();
  String? _error;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Center(
        child: SizedBox(
          width: 420,
          child: Column(mainAxisSize: MainAxisSize.min, children: [
            const Text('Aktivasi perangkat SIBATANG', style: TextStyle(fontSize: 24)),
            const SizedBox(height: 12),
            TextField(controller: _code, decoration: const InputDecoration(labelText: 'Kode aktivasi sekali pakai'), obscureText: true),
            if (_error != null) Text(_error!, style: const TextStyle(color: Colors.red)),
            const SizedBox(height: 12),
            FilledButton(
              onPressed: () async {
                try {
                  await widget.onActivate(_code.text);
                } catch (_) {
                  setState(() => _error = 'Kode tidak valid atau kedaluwarsa.');
                }
              },
              child: const Text('Aktifkan'),
            ),
          ]),
        ),
      ),
    );
  }
}
