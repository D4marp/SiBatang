import 'dart:async';

import 'package:flutter/foundation.dart';

import '../../core/device_api.dart';
import '../../core/models.dart';
import '../../core/sse.dart';

/// Display: snapshot REST sebagai kebenaran, SSE hanya pemicu resync. Menampilkan indikator basi bila terputus.
class DisplayController extends ChangeNotifier {
  DisplayController(this.api);
  final DeviceApi api;

  DisplaySnapshot? snapshot;
  bool connected = false;
  DateTime? lastUpdate;
  bool running = false;

  Future<void> start() async {
    if (running) return;
    running = true;
    String? lastId;
    while (running) {
      try {
        await _resync();
        await for (final m in connectSse(api.uri('/devices/${api.identity!.deviceId}/display/events'), headers: api.authHeaders, lastEventId: lastId)) {
          lastId = m.id ?? lastId;
          connected = true;
          await _resync();
        }
      } catch (_) {
        connected = false;
        notifyListeners();
      }
      await Future<void>.delayed(const Duration(seconds: 3));
    }
  }

  Future<void> _resync() async {
    snapshot = await api.displaySnapshot();
    lastUpdate = DateTime.now();
    connected = true;
    notifyListeners();
  }

  void stop() => running = false;
}
