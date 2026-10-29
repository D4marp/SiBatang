import 'dart:convert';

import 'package:http/http.dart' as http;

import 'config.dart';
import 'models.dart';

class ApiException implements Exception {
  ApiException(this.status, this.code);
  final int status;
  final String code;
  bool get isStandby => code == 'ZONE_LEASED_BY_OTHER';
  bool get isStaleFencing => code == 'STALE_FENCING_TOKEN';
  bool get isAuth => status == 401;
  @override
  String toString() => 'ApiException($status $code)';
}

class DeviceApi {
  DeviceApi({this.identity, http.Client? client}) : _http = client ?? http.Client();
  final DeviceIdentity? identity;
  final http.Client _http;

  Uri uri(String path) => Uri.parse('${AppConfig.apiBaseUrl}$path');
  Map<String, String> get authHeaders => {if (identity != null) 'Authorization': 'Device ${identity!.credential}'};

  Future<dynamic> _send(Future<http.Response> Function() f) async {
    final res = await f();
    if (res.statusCode >= 200 && res.statusCode < 300) {
      if (res.statusCode == 204 || res.body.isEmpty) return null;
      return (jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>)['data'];
    }
    String code = 'UNKNOWN';
    try {
      code = (jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>)['code'] as String? ?? code;
    } on FormatException {
      // body bukan problem+json
    }
    throw ApiException(res.statusCode, code);
  }

  Future<dynamic> _post(String path, Object? body) => _send(() => _http.post(uri(path),
      headers: {'Content-Type': 'application/json', ...authHeaders}, body: body == null ? null : jsonEncode(body)));

  Future<DeviceIdentity> activate(String code) async {
    final d = await _send(() => _http.post(uri('/devices/activate'),
        headers: {'Content-Type': 'application/json'}, body: jsonEncode({'activation_code': code.trim()}))) as Map<String, dynamic>;
    return DeviceIdentity(
      deviceId: d['device_id'] as String,
      credential: d['credential'] as String,
      deviceType: d['device_type'] as String,
      siteId: d['site_id'] as String,
      audioZoneId: (d['audio_zone_id'] as String?)?.isEmpty == true ? null : d['audio_zone_id'] as String?,
    );
  }

  Future<void> heartbeat(Map<String, Object?> health, {String appVersion = '0.1.0'}) =>
      _post('/devices/${identity!.deviceId}/heartbeat', {'app_version': appVersion, 'health': health});

  Future<CheckinResult> checkin(String qrToken) async {
    final d = await _post('/devices/${identity!.deviceId}/checkins', {'qr_token': qrToken}) as Map<String, dynamic>;
    return CheckinResult(ticketId: d['ticket_id'] as String, displayNumber: d['display_number'] as String, isReprint: d['is_reprint'] as bool);
  }

  Future<void> reportPrint(String ticketId, {required bool ok}) =>
      _post('/devices/${identity!.deviceId}/tickets/$ticketId/print-attempts', {'outcome': ok ? 'SUCCESS' : 'FAILED'});

  Future<DisplaySnapshot> displaySnapshot() async {
    final d = await _send(() => _http.get(uri('/devices/${identity!.deviceId}/display/snapshot'), headers: authHeaders)) as Map<String, dynamic>;
    return DisplaySnapshot.fromJson(d);
  }

  Future<AnnouncementClaim?> claim() async {
    final d = await _post('/devices/${identity!.deviceId}/announcer/jobs/claim', null);
    return d == null ? null : AnnouncementClaim.fromJson(d as Map<String, dynamic>);
  }

  Future<void> ack(String jobId, String action, int fencingToken, {String? errorCode}) =>
      _post('/devices/${identity!.deviceId}/announcer/jobs/$jobId/$action', {
        'fencing_token': fencingToken,
        if (errorCode != null) 'error_code': errorCode,
      });
}
