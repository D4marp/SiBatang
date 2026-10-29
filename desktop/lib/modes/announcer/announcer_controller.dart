import 'dart:async';

import 'package:flutter/foundation.dart';

import '../../core/device_api.dart';
import '../../core/models.dart';
import '../../core/sse.dart';
import 'voice_announcer.dart';
import 'voice_renderer.dart';

/// Announcer: SSE hanya sebagai pemicu (tanpa PII); data suara selalu lewat klaim privat.
/// Server memberi satu job aktif per zona dengan lease + fencing; klien memutar berurutan dan tidak pernah
/// memutar ulang job basi saat restart (job kedaluwarsa diabaikan dan dilaporkan FAILED).
class AnnouncerController extends ChangeNotifier {
  AnnouncerController({required this.api, required this.voice, this.volume = 1.0});
  final DeviceApi api;
  final VoiceAnnouncer voice;
  final double volume;

  bool running = false;
  bool standby = false;
  bool online = true;
  bool voiceReady = false;
  String? lastError; // kode saja, tanpa teks/nama
  int played = 0;
  int failed = 0;
  DateTime? lastAckAt;

  Completer<void>? _wake;

  Future<void> start() async {
    if (running) return;
    running = true;
    unawaited(_listenWakeups());
    unawaited(_heartbeatLoop());
    while (running) {
      try {
        final h = await voice.health();
        voiceReady = h.localeAvailable;
        final claim = await api.claim();
        online = true;
        standby = false;
        if (claim == null) {
          await _sleepOrWake(const Duration(seconds: 3));
          continue;
        }
        await playClaim(claim);
      } on ApiException catch (e) {
        if (e.isStandby) {
          standby = true; // announcer lain memegang lease zona
        } else {
          lastError = e.code;
        }
        notifyListeners();
        await _sleepOrWake(const Duration(seconds: 5));
      } catch (_) {
        online = false;
        lastError = 'NETWORK';
        notifyListeners();
        await _sleepOrWake(const Duration(seconds: 5));
      }
    }
  }

  void stopLoop() {
    running = false;
    _wake?.complete();
  }

  @visibleForTesting
  Future<void> playClaim(AnnouncementClaim c) async {
    if (c.expiresAt.isBefore(DateTime.now().toUtc())) {
      await _safeAck(c, 'failed', code: 'EXPIRED_LOCALLY');
      failed++;
      notifyListeners();
      return;
    }
    await _safeAck(c, 'started');
    try {
      await voice.speak(renderUtterance(c.speech), locale: c.locale, volume: volume);
      await _safeAck(c, 'completed');
      played++;
      lastError = null;
    } catch (e) {
      // Kegagalan audio tidak mengubah tiket; hanya dilaporkan agar ada alarm dan SOP manual.
      lastError = e is StateError ? e.message.toString() : 'TTS_ERROR';
      await _safeAck(c, 'failed', code: lastError);
      failed++;
    }
    lastAckAt = DateTime.now();
    notifyListeners();
  }

  Future<void> _safeAck(AnnouncementClaim c, String action, {String? code}) async {
    try {
      await api.ack(c.jobId, action, c.fencingToken, errorCode: code);
    } on ApiException catch (e) {
      if (e.isStaleFencing) {
        lastError = 'STALE_FENCING'; // lease diambil alih; hentikan semua tindakan atas job ini
        return;
      }
      lastError = e.code;
    } catch (_) {
      lastError = 'ACK_NETWORK';
    }
  }

  Future<void> _sleepOrWake(Duration d) async {
    _wake = Completer<void>();
    await Future.any([Future<void>.delayed(d), _wake!.future]);
  }

  Future<void> _listenWakeups() async {
    String? lastId;
    while (running) {
      try {
        await for (final m in connectSse(api.uri('/devices/${api.identity!.deviceId}/announcer/events'), headers: api.authHeaders, lastEventId: lastId)) {
          lastId = m.id ?? lastId;
          if (m.event == 'announcement.job_available') _wake?.complete();
        }
      } catch (_) {
        // reconnect dengan backoff; polling claim tetap berjalan sebagai rekonsiliasi
      }
      await Future<void>.delayed(const Duration(seconds: 3));
    }
  }

  Future<void> _heartbeatLoop() async {
    while (running) {
      try {
        await api.heartbeat({'speaker_state': voiceReady ? 'ok' : 'no_voice', 'voice_ok': voiceReady, 'last_error_code': lastError ?? ''});
      } catch (_) {
        // dashboard akan menandai perangkat offline bila heartbeat berhenti
      }
      await Future<void>.delayed(const Duration(seconds: 30));
    }
  }
}
