import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:intl/intl.dart';

import '../../core/api_client.dart';
import '../../core/config.dart';
import 'models.dart';

final apiClientProvider = Provider<ApiClient>((_) => ApiClient());
final secureStorageProvider = Provider<FlutterSecureStorage>((_) => const FlutterSecureStorage());
final bookingRepositoryProvider = Provider<BookingRepository>(
  (ref) => BookingRepository(ref.watch(apiClientProvider), ref.watch(secureStorageProvider)),
);

class BookingRepository {
  BookingRepository(this._api, this._store);
  final ApiClient _api;
  final FlutterSecureStorage _store;

  static const _tokenKeyPrefix = 'booking_token_';

  Future<List<CatalogService>> catalog() async {
    final d = await _api.get('/public/tenants/${AppConfig.tenantSlug}/catalog') as Map<String, dynamic>;
    return [
      for (final s in d['sites'] as List)
        for (final v in (s as Map<String, dynamic>)['services'] as List)
          CatalogService(
            serviceId: (v as Map<String, dynamic>)['service_id'] as String,
            code: v['code'] as String,
            displayName: v['display_name'] as String,
            siteName: s['name'] as String,
          ),
    ];
  }

  Future<List<Slot>> slots(String serviceId, DateTime date) async {
    final d = await _api.get('/public/tenants/${AppConfig.tenantSlug}/slots',
        query: {'service_id': serviceId, 'date': DateFormat('yyyy-MM-dd').format(date)}) as List;
    return [for (final j in d) Slot.fromJson(j as Map<String, dynamic>)];
  }

  /// [idempotencyKey] dibuat sekali per percobaan form dan dipakai ulang saat retry.
  Future<BookingDetail> create({
    required String idempotencyKey,
    required String slotId,
    required String serviceId,
    required String name,
    required String contact,
    required bool allowVoiceName,
  }) async {
    final d = await _api.post('/public/bookings', headers: {'Idempotency-Key': idempotencyKey}, body: {
      'tenant_code': AppConfig.tenantCode,
      'slot_id': slotId,
      'service_id': serviceId,
      'applicant': {'display_name': name, 'contact': contact, 'allow_voice_name': allowVoiceName},
      'privacy_notice_version': AppConfig.privacyNoticeVersion,
    }) as Map<String, dynamic>;
    final b = BookingDetail.fromJson(d);
    if (b.qrToken != null) await _store.write(key: '$_tokenKeyPrefix${b.reference}', value: b.qrToken); // Keystore/Keychain
    return b;
  }

  Future<String?> tokenFor(String reference) => _store.read(key: '$_tokenKeyPrefix$reference');

  Future<BookingDetail> detail(String reference) async {
    final token = await tokenFor(reference);
    if (token == null) throw ApiException(404, 'NOT_FOUND', 'token missing');
    final d = await _api.get('/public/bookings/$reference', headers: {'X-Booking-Token': token}) as Map<String, dynamic>;
    return BookingDetail.fromJson(d).withToken(token);
  }

  Future<void> cancel(String reference) async {
    final token = await tokenFor(reference);
    if (token == null) throw ApiException(404, 'NOT_FOUND', 'token missing');
    await _api.post('/public/bookings/$reference/cancel', headers: {'X-Booking-Token': token});
  }
}

extension on BookingDetail {
  BookingDetail withToken(String token) => BookingDetail(
        reference: reference,
        status: status,
        service: service,
        serviceDate: serviceDate,
        startsAt: startsAt,
        qrExpiresAt: qrExpiresAt,
        qrToken: token,
        ticket: ticket,
      );
}
