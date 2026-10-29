import 'dart:async';
import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:sibatang_desktop/core/models.dart';
import 'package:sibatang_desktop/core/sse.dart';
import 'package:sibatang_desktop/modes/announcer/voice_renderer.dart';

void main() {
  // Kasus yang sama dengan backend/internal/domain TestUtterance - template harus identik.
  test('utterance dengan nama (kebijakan + consent)', () {
    const s = Speech(serviceSpoken: 'S K C K', numberSpoken: 'nol dua lima', counterSpoken: 'loket dua', templateId: 'id-ID-v1', publicCallName: 'Budi');
    expect(renderUtterance(s), 'Nomor antrean S K C K, nol dua lima, atas nama Budi, silakan menuju loket dua.');
  });

  test('utterance nomor saja bila nama null', () {
    const s = Speech(serviceSpoken: 'S K C K', numberSpoken: 'nol dua lima', counterSpoken: 'loket dua', templateId: 'id-ID-v1');
    expect(renderUtterance(s), 'Nomor antrean S K C K, nol dua lima, silakan menuju loket dua.');
  });

  test('parser SSE mengabaikan heartbeat dan membaca id/event/data', () async {
    const raw = ': heartbeat\n\nid: 5\nevent: queue.ticket_called\ndata: {"ticket_number":"SKCK-025"}\n\n';
    final out = await parseSse(Stream.value(utf8.encode(raw))).toList();
    expect(out, hasLength(1));
    expect(out.single.id, '5');
    expect(out.single.event, 'queue.ticket_called');
    expect(jsonDecode(out.single.data)['ticket_number'], 'SKCK-025');
  });
}
