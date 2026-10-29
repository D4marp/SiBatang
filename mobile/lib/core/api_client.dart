import 'dart:convert';

import 'package:http/http.dart' as http;

import 'config.dart';

/// Error API terstruktur (RFC 9457 problem+json). Pesan tidak memuat PII.
class ApiException implements Exception {
  ApiException(this.status, this.code, this.detail);
  final int status;
  final String code;
  final String detail;

  @override
  String toString() => 'ApiException($status $code)';
}

class ApiClient {
  ApiClient({http.Client? client}) : _http = client ?? http.Client();
  final http.Client _http;

  Uri _uri(String path, [Map<String, String>? query]) =>
      Uri.parse('${AppConfig.apiBaseUrl}$path').replace(queryParameters: query);

  Future<dynamic> _decode(http.Response res) async {
    if (res.statusCode >= 200 && res.statusCode < 300) {
      if (res.statusCode == 204 || res.body.isEmpty) return null;
      return (jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>)['data'];
    }
    try {
      final p = jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
      throw ApiException(res.statusCode, '${p['code']}', '${p['detail']}');
    } on FormatException {
      throw ApiException(res.statusCode, 'UNKNOWN', res.reasonPhrase ?? '');
    }
  }

  Future<dynamic> get(String path, {Map<String, String>? query, Map<String, String>? headers}) async =>
      _decode(await _http.get(_uri(path, query), headers: headers));

  Future<dynamic> post(String path, {Object? body, Map<String, String>? headers}) async =>
      _decode(await _http.post(
        _uri(path),
        headers: {'Content-Type': 'application/json', ...?headers},
        body: body == null ? null : jsonEncode(body),
      ));
}
