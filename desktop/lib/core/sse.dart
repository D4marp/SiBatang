import 'dart:async';
import 'dart:convert';

import 'package:http/http.dart' as http;

class SseMessage {
  const SseMessage({required this.event, required this.data, this.id});
  final String? id;
  final String event;
  final String data;
}

/// Parser text/event-stream (komentar ": heartbeat" diabaikan).
Stream<SseMessage> parseSse(Stream<List<int>> bytes) async* {
  String? id;
  String? event;
  final data = <String>[];
  await for (final line in bytes.transform(utf8.decoder).transform(const LineSplitter())) {
    if (line.isEmpty) {
      if (data.isNotEmpty) yield SseMessage(id: id, event: event ?? 'message', data: data.join('\n'));
      id = null;
      event = null;
      data.clear();
      continue;
    }
    if (line.startsWith(':')) continue;
    final i = line.indexOf(':');
    final field = i < 0 ? line : line.substring(0, i);
    var value = i < 0 ? '' : line.substring(i + 1);
    if (value.startsWith(' ')) value = value.substring(1);
    switch (field) {
      case 'id':
        id = value;
      case 'event':
        event = value;
      case 'data':
        data.add(value);
    }
  }
}

/// Satu koneksi SSE dengan header otorisasi perangkat. Pemanggil bertanggung jawab reconnect + resync snapshot.
Stream<SseMessage> connectSse(Uri uri, {required Map<String, String> headers, String? lastEventId, http.Client? client}) async* {
  final c = client ?? http.Client();
  try {
    final req = http.Request('GET', uri)
      ..headers.addAll({'Accept': 'text/event-stream', ...headers, if (lastEventId != null) 'Last-Event-ID': lastEventId});
    final res = await c.send(req);
    if (res.statusCode != 200) throw http.ClientException('SSE ${res.statusCode}', uri);
    yield* parseSse(res.stream);
  } finally {
    if (client == null) c.close();
  }
}
