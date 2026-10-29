/// Model dibuat manual sampai klien Dart di-generate dari backend/api/openapi.yaml
/// (lihat README: openapi-generator `dart-dio`/`dart` dijalankan di CI).
class CatalogService {
  const CatalogService({required this.serviceId, required this.code, required this.displayName, required this.siteName});
  final String serviceId;
  final String code;
  final String displayName;
  final String siteName;
}

class Slot {
  const Slot({required this.id, required this.startsAt, required this.endsAt, required this.remaining});
  final String id;
  final DateTime startsAt;
  final DateTime endsAt;
  final int remaining;

  factory Slot.fromJson(Map<String, dynamic> j) => Slot(
        id: j['slot_id'] as String,
        startsAt: DateTime.parse(j['starts_at'] as String),
        endsAt: DateTime.parse(j['ends_at'] as String),
        remaining: j['remaining'] as int,
      );
}

class TicketView {
  const TicketView({required this.displayNumber, required this.status, this.counterLabel, this.waitingAhead});
  final String displayNumber;
  final String status;
  final String? counterLabel;
  final int? waitingAhead; // perkiraan, bukan janji

  factory TicketView.fromJson(Map<String, dynamic> j) => TicketView(
        displayNumber: j['display_number'] as String,
        status: j['status'] as String,
        counterLabel: j['counter_label'] as String?,
        waitingAhead: j['waiting_ahead_estimate'] as int?,
      );
}

class BookingDetail {
  const BookingDetail({
    required this.reference,
    required this.status,
    required this.service,
    required this.serviceDate,
    required this.startsAt,
    required this.qrExpiresAt,
    this.qrToken,
    this.ticket,
  });
  final String reference;
  final String status;
  final String service;
  final String serviceDate;
  final DateTime startsAt;
  final DateTime qrExpiresAt;
  final String? qrToken; // hanya pada respons pembuatan
  final TicketView? ticket;

  factory BookingDetail.fromJson(Map<String, dynamic> j) => BookingDetail(
        reference: j['reference'] as String,
        status: j['status'] as String,
        service: j['service'] as String,
        serviceDate: j['service_date'] as String,
        startsAt: DateTime.parse(j['starts_at'] as String),
        qrExpiresAt: DateTime.parse(j['qr_expires_at'] as String),
        qrToken: j['qr_token'] as String?,
        ticket: j['ticket'] == null ? null : TicketView.fromJson(j['ticket'] as Map<String, dynamic>),
      );
}
