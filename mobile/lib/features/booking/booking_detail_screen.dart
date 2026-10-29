import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:intl/intl.dart';
import 'package:qr_flutter/qr_flutter.dart';

import '../../core/api_client.dart';
import 'booking_repository.dart';
import 'models.dart';

class BookingDetailScreen extends ConsumerStatefulWidget {
  const BookingDetailScreen({super.key, required this.reference});
  final String reference;

  @override
  ConsumerState<BookingDetailScreen> createState() => _BookingDetailScreenState();
}

class _BookingDetailScreenState extends ConsumerState<BookingDetailScreen> {
  BookingDetail? _detail;
  String? _error;
  Timer? _timer;

  @override
  void initState() {
    super.initState();
    _refresh();
    // Polling 5 dtk + resync; fallback bila SSE tidak stabil (SSE privat memakai header X-Booking-Token).
    _timer = Timer.periodic(const Duration(seconds: 5), (_) => _refresh());
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  Future<void> _refresh() async {
    try {
      final d = await ref.read(bookingRepositoryProvider).detail(widget.reference);
      if (mounted) setState(() {
            _detail = d;
            _error = null;
          });
    } on ApiException catch (e) {
      if (mounted) setState(() => _error = e.status == 404 ? 'Pendaftaran tidak ditemukan.' : 'Gagal memuat (${e.code}).');
    } catch (_) {
      if (mounted) setState(() => _error = 'Tidak dapat terhubung. Menampilkan data terakhir.');
    }
  }

  @override
  Widget build(BuildContext context) {
    final d = _detail;
    return Scaffold(
      appBar: AppBar(title: Text('Pendaftaran ${widget.reference}'), leading: BackButton(onPressed: () => context.go('/'))),
      body: ListView(padding: const EdgeInsets.all(16), children: [
        if (_error != null) Text(_error!, style: TextStyle(color: Theme.of(context).colorScheme.error)),
        if (d == null && _error == null) const Center(child: CircularProgressIndicator()),
        if (d != null) ...[
          Text('${d.service} · ${d.serviceDate} · ${DateFormat('HH:mm').format(d.startsAt.toLocal())}'),
          Text('Status: ${d.status}'),
          const SizedBox(height: 16),
          if (d.ticket != null) ...[
            const Text('Nomor antrean Anda'),
            Semantics(
              liveRegion: true,
              child: Text(d.ticket!.displayNumber, style: Theme.of(context).textTheme.displayMedium),
            ),
            Text('Status: ${d.ticket!.status}${d.ticket!.counterLabel != null ? ' · menuju ${d.ticket!.counterLabel}' : ''}'),
            if (d.ticket!.waitingAhead != null) Text('Perkiraan ${d.ticket!.waitingAhead} antrean di depan (bukan janji).'),
          ] else if (d.status == 'CONFIRMED' && d.qrToken != null) ...[
            const Text('Tunjukkan QR ke kiosk saat tiba.'),
            Center(child: QrImageView(data: d.qrToken!, size: 260, semanticsLabel: 'Kode QR check-in')),
            const Text('Jangan bagikan QR ini.'),
            const SizedBox(height: 12),
            OutlinedButton(
              onPressed: () async {
                final ok = await showDialog<bool>(
                  context: context,
                  builder: (c) => AlertDialog(
                    title: const Text('Batalkan pendaftaran?'),
                    actions: [
                      TextButton(onPressed: () => Navigator.pop(c, false), child: const Text('Tidak')),
                      FilledButton(onPressed: () => Navigator.pop(c, true), child: const Text('Batalkan')),
                    ],
                  ),
                );
                if (ok == true) {
                  await ref.read(bookingRepositoryProvider).cancel(widget.reference);
                  await _refresh();
                }
              },
              child: const Text('Batalkan pendaftaran'),
            ),
          ],
        ],
      ]),
    );
  }
}
