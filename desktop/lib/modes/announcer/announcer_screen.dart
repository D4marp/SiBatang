import 'package:flutter/material.dart';

import 'announcer_controller.dart';

class AnnouncerScreen extends StatefulWidget {
  const AnnouncerScreen({super.key, required this.controller});
  final AnnouncerController controller;

  @override
  State<AnnouncerScreen> createState() => _AnnouncerScreenState();
}

class _AnnouncerScreenState extends State<AnnouncerScreen> {
  @override
  void initState() {
    super.initState();
    widget.controller.start();
  }

  @override
  void dispose() {
    widget.controller.stopLoop();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: widget.controller,
      builder: (context, _) {
        final c = widget.controller;
        Widget row(String k, String v, bool ok) => ListTile(
              leading: Icon(ok ? Icons.check_circle : Icons.error, color: ok ? Colors.green : Colors.red),
              title: Text(k),
              subtitle: Text(v),
            );
        return Scaffold(
          appBar: AppBar(title: const Text('Announcer — kesehatan')),
          body: ListView(children: [
            row('Koneksi API', c.online ? 'Terhubung' : 'Terputus', c.online),
            row('Suara id-ID', c.voiceReady ? 'Terpasang' : 'Tidak tersedia — gunakan SOP manual', c.voiceReady),
            row('Status zona', c.standby ? 'Standby (announcer lain aktif)' : 'Aktif', true),
            row('Diputar / gagal', '${c.played} / ${c.failed}', c.failed == 0),
            row('Kesalahan terakhir', c.lastError ?? '-', c.lastError == null),
            row('Ack terakhir', c.lastAckAt?.toLocal().toString() ?? '-', true),
          ]),
        );
      },
    );
  }
}
