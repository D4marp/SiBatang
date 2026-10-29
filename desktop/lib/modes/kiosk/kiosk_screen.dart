import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../core/device_api.dart';
import '../../core/models.dart';
import 'ticket_printer.dart';

/// Kiosk: scanner USB HID (keyboard wedge) mengetik token lalu Enter. Nomor hanya dari Go; tidak ada nomor offline.
class KioskScreen extends StatefulWidget {
  const KioskScreen({super.key, required this.api, required this.printer});
  final DeviceApi api;
  final TicketPrinter printer;

  @override
  State<KioskScreen> createState() => _KioskScreenState();
}

class _KioskScreenState extends State<KioskScreen> {
  final _input = TextEditingController();
  final _focus = FocusNode();
  CheckinResult? _result;
  String? _message;
  bool _busy = false;
  Timer? _idle;

  @override
  void dispose() {
    _idle?.cancel();
    _input.dispose();
    _focus.dispose();
    super.dispose();
  }

  Future<void> _scan(String raw) async {
    final token = raw.trim();
    _input.clear(); // token QR tidak disimpan setelah dipakai
    if (token.length < 16 || _busy) return;
    setState(() {
      _busy = true;
      _message = null;
    });
    try {
      final r = await widget.api.checkin(token);
      final ok = await widget.printer.print(r.displayNumber, isReprint: r.isReprint);
      unawaited(widget.api.reportPrint(r.ticketId, ok: ok).catchError((_) {}));
      setState(() {
        _result = r;
        _message = ok ? null : 'Printer bermasalah. Nomor Anda tetap ${r.displayNumber}; hubungi petugas untuk cetak ulang.';
      });
    } on ApiException catch (e) {
      setState(() => _message = switch (e.code) {
            'NOT_FOUND' => 'QR tidak dikenali.',
            'BOOKING_EXPIRED' => 'Pendaftaran kedaluwarsa atau dibatalkan.',
            'SERVICE_POLICY_NOT_MET' => 'Belum waktunya check-in atau lokasi tidak sesuai.',
            _ => 'Layanan sementara tidak tersedia. Hubungi petugas.',
          });
    } catch (_) {
      // Jangan pernah membuat nomor sendiri saat API tidak dapat dijangkau.
      setState(() => _message = 'Tidak terhubung ke server. Hubungi petugas (proses manual).');
    } finally {
      setState(() => _busy = false);
      _idle?.cancel();
      _idle = Timer(const Duration(seconds: 12), () {
        if (mounted) setState(() => _result = null);
      });
      _focus.requestFocus();
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Center(
        child: Column(mainAxisSize: MainAxisSize.min, children: [
          Text('SIBATANG', style: Theme.of(context).textTheme.displayMedium),
          const SizedBox(height: 16),
          if (_result == null) ...[
            const Text('Scan QR pendaftaran Anda', style: TextStyle(fontSize: 32)),
            SizedBox(
              width: 1,
              height: 1,
              child: TextField(
                controller: _input,
                focusNode: _focus,
                autofocus: true,
                onSubmitted: _scan,
                inputFormatters: [FilteringTextInputFormatter.deny(RegExp(r'\s'))],
              ),
            ),
          ] else ...[
            const Text('Nomor antrean Anda', style: TextStyle(fontSize: 32)),
            Text(_result!.displayNumber, style: const TextStyle(fontSize: 120, fontWeight: FontWeight.w800)),
            if (_result!.isReprint) const Text('(cetak ulang nomor yang sama)'),
          ],
          if (_message != null) Padding(padding: const EdgeInsets.all(16), child: Text(_message!, style: const TextStyle(fontSize: 24, color: Colors.red))),
        ]),
      ),
    );
  }
}
