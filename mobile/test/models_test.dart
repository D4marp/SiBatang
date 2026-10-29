import 'package:flutter_test/flutter_test.dart';
import 'package:sibatang_mobile/features/booking/models.dart';

void main() {
  test('BookingDetail parses ticket and never needs personal data', () {
    final b = BookingDetail.fromJson({
      'reference': 'BTG-20261024-ABC123',
      'status': 'CHECKED_IN',
      'service': 'SKCK',
      'service_date': '2026-10-24',
      'starts_at': '2026-10-24T03:00:00Z',
      'qr_expires_at': '2026-10-24T04:00:00Z',
      'ticket': {'display_number': 'SKCK-025', 'status': 'WAITING', 'counter_label': null, 'waiting_ahead_estimate': 3},
    });
    expect(b.ticket!.displayNumber, 'SKCK-025');
    expect(b.ticket!.waitingAhead, 3);
    expect(b.qrToken, isNull);
  });
}
