import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import 'models.dart';

/// Kredensial perangkat hanya di penyimpanan aman OS (Windows Credential Locker / DPAPI). Tidak ada QR/PII yang disimpan.
class CredentialStore {
  const CredentialStore([this._s = const FlutterSecureStorage()]);
  final FlutterSecureStorage _s;

  Future<DeviceIdentity?> load() async {
    final cred = await _s.read(key: 'credential');
    final id = await _s.read(key: 'device_id');
    final type = await _s.read(key: 'device_type');
    final site = await _s.read(key: 'site_id');
    if (cred == null || id == null || type == null || site == null) return null;
    return DeviceIdentity(deviceId: id, credential: cred, deviceType: type, siteId: site, audioZoneId: await _s.read(key: 'audio_zone_id'));
  }

  Future<void> save(DeviceIdentity i) async {
    await _s.write(key: 'credential', value: i.credential);
    await _s.write(key: 'device_id', value: i.deviceId);
    await _s.write(key: 'device_type', value: i.deviceType);
    await _s.write(key: 'site_id', value: i.siteId);
    if (i.audioZoneId != null) await _s.write(key: 'audio_zone_id', value: i.audioZoneId);
  }

  Future<void> clear() => _s.deleteAll();
}
