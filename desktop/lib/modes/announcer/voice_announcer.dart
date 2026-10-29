import 'package:flutter_tts/flutter_tts.dart';

class VoiceInfo {
  const VoiceInfo(this.name, this.locale);
  final String name;
  final String locale;
}

class SpeakerHealth {
  const SpeakerHealth({required this.localeAvailable, required this.voices});
  final bool localeAvailable; // id-ID terpasang?
  final List<VoiceInfo> voices;
}

/// Abstraksi mesin suara (05_FLUTTER_DESKTOP.md §6). TTS lokal; nama tidak pernah dikirim ke cloud TTS.
abstract interface class VoiceAnnouncer {
  Future<SpeakerHealth> health();
  Future<void> speak(String text, {required String locale, required double volume});
  Future<void> stop();
}

class FlutterTtsAnnouncer implements VoiceAnnouncer {
  final FlutterTts _tts = FlutterTts();

  @override
  Future<SpeakerHealth> health() async {
    final raw = await _tts.getVoices as List<dynamic>? ?? const [];
    final voices = [
      for (final v in raw)
        if (v is Map) VoiceInfo('${v['name']}', '${v['locale']}'),
    ];
    return SpeakerHealth(localeAvailable: voices.any((v) => v.locale.toLowerCase().replaceAll('_', '-') == 'id-id'), voices: voices);
  }

  @override
  Future<void> speak(String text, {required String locale, required double volume}) async {
    final h = await health();
    if (!h.localeAvailable) throw StateError('VOICE_LOCALE_MISSING'); // jangan membaca nama Indonesia dengan suara asing
    await _tts.setLanguage(locale);
    await _tts.setVolume(volume);
    await _tts.awaitSpeakCompletion(true);
    await _tts.speak(text);
  }

  @override
  Future<void> stop() async {
    await _tts.stop();
  }
}

/// Untuk uji otomatis / tanpa speaker.
class MockVoiceAnnouncer implements VoiceAnnouncer {
  final List<String> spoken = [];
  bool localeAvailable = true;

  @override
  Future<SpeakerHealth> health() async => SpeakerHealth(localeAvailable: localeAvailable, voices: const [VoiceInfo('mock', 'id-ID')]);

  @override
  Future<void> speak(String text, {required String locale, required double volume}) async {
    if (!localeAvailable) throw StateError('VOICE_LOCALE_MISSING');
    spoken.add(text);
  }

  @override
  Future<void> stop() async {}
}
