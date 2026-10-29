class DeviceIdentity {
  const DeviceIdentity({required this.deviceId, required this.credential, required this.deviceType, required this.siteId, this.audioZoneId});
  final String deviceId;
  final String credential; // "<device_id>.<secret>" - hanya di secure storage
  final String deviceType; // KIOSK | OPERATOR | DISPLAY | ANNOUNCER (ditetapkan server, bukan flag lokal)
  final String siteId;
  final String? audioZoneId;
}

class Speech {
  const Speech({required this.serviceSpoken, required this.numberSpoken, required this.counterSpoken, required this.templateId, this.publicCallName});
  final String serviceSpoken;
  final String numberSpoken;
  final String counterSpoken;
  final String templateId;
  final String? publicCallName; // null = nomor + loket saja

  factory Speech.fromJson(Map<String, dynamic> j) => Speech(
        serviceSpoken: j['service_spoken'] as String,
        numberSpoken: j['number_spoken'] as String,
        counterSpoken: j['counter_spoken'] as String,
        templateId: j['template_id'] as String,
        publicCallName: j['public_call_name'] as String?,
      );
}

class AnnouncementClaim {
  const AnnouncementClaim({required this.jobId, required this.fencingToken, required this.expiresAt, required this.locale, required this.speech});
  final String jobId;
  final int fencingToken;
  final DateTime expiresAt;
  final String locale;
  final Speech speech;

  factory AnnouncementClaim.fromJson(Map<String, dynamic> j) => AnnouncementClaim(
        jobId: j['job_id'] as String,
        fencingToken: (j['fencing_token'] as num).toInt(),
        expiresAt: DateTime.parse(j['expires_at'] as String),
        locale: j['locale'] as String,
        speech: Speech.fromJson(j['speech'] as Map<String, dynamic>),
      );
}

class DisplayEntry {
  const DisplayEntry({required this.ticketNumber, required this.counterLabel, required this.service, required this.status});
  final String ticketNumber;
  final String counterLabel;
  final String service;
  final String status;
}

class DisplaySnapshot {
  const DisplaySnapshot({required this.nowServing, required this.waitingByService, required this.lastEventId, required this.asOf});
  final List<DisplayEntry> nowServing;
  final Map<String, int> waitingByService;
  final int lastEventId;
  final DateTime asOf;

  factory DisplaySnapshot.fromJson(Map<String, dynamic> j) => DisplaySnapshot(
        nowServing: [
          for (final e in j['now_serving'] as List)
            DisplayEntry(
              ticketNumber: (e as Map<String, dynamic>)['ticket_number'] as String,
              counterLabel: e['counter_label'] as String,
              service: e['service'] as String,
              status: e['status'] as String,
            ),
        ],
        waitingByService: {for (final e in (j['waiting_by_service'] as Map<String, dynamic>).entries) e.key: (e.value as num).toInt()},
        lastEventId: (j['last_event_id'] as num).toInt(),
        asOf: DateTime.parse(j['as_of'] as String),
      );
}

class CheckinResult {
  const CheckinResult({required this.ticketId, required this.displayNumber, required this.isReprint});
  final String ticketId;
  final String displayNumber;
  final bool isReprint;
}
