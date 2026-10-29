import 'package:flutter/material.dart';

import 'display_controller.dart';

/// Hanya nomor antrean, loket, layanan. Tidak ada nama/NIK/HP.
class DisplayScreen extends StatefulWidget {
  const DisplayScreen({super.key, required this.controller});
  final DisplayController controller;

  @override
  State<DisplayScreen> createState() => _DisplayScreenState();
}

class _DisplayScreenState extends State<DisplayScreen> {
  @override
  void initState() {
    super.initState();
    widget.controller.start();
  }

  @override
  void dispose() {
    widget.controller.stop();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: widget.controller,
      builder: (context, _) {
        final c = widget.controller;
        final s = c.snapshot;
        return Scaffold(
          backgroundColor: Colors.black,
          body: DefaultTextStyle(
            style: const TextStyle(color: Colors.white, fontSize: 40),
            child: Padding(
              padding: const EdgeInsets.all(32),
              child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Row(children: [
                  const Expanded(child: Text('SIBATANG — Sedang dilayani', style: TextStyle(fontSize: 48, fontWeight: FontWeight.bold))),
                  if (!c.connected) const Chip(label: Text('Koneksi terputus — data mungkin tidak terkini')),
                ]),
                const SizedBox(height: 24),
                if (s == null) const Text('Memuat…'),
                if (s != null && s.nowServing.isEmpty) const Text('Belum ada antrean dipanggil'),
                if (s != null)
                  Expanded(
                    child: GridView.count(
                      crossAxisCount: 2,
                      childAspectRatio: 3,
                      children: [
                        for (final e in s.nowServing)
                          Card(
                            color: Colors.blueGrey.shade900,
                            child: Padding(
                              padding: const EdgeInsets.all(16),
                              child: Row(mainAxisAlignment: MainAxisAlignment.spaceBetween, children: [
                                Text(e.ticketNumber, style: const TextStyle(fontSize: 64, fontWeight: FontWeight.w800, color: Colors.white)),
                                Text(e.counterLabel, style: const TextStyle(color: Colors.amber)),
                              ]),
                            ),
                          ),
                      ],
                    ),
                  ),
                if (c.lastUpdate != null) Text('Diperbarui ${c.lastUpdate!.toLocal().toString().substring(11, 19)}', style: const TextStyle(fontSize: 18)),
              ]),
            ),
          ),
        );
      },
    );
  }
}
