import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:intl/intl.dart';
import 'dart:math' as math;

import '../../core/api_client.dart';
import 'booking_repository.dart';
import 'models.dart';

class BookingFlowScreen extends ConsumerStatefulWidget {
  const BookingFlowScreen({super.key});

  @override
  ConsumerState<BookingFlowScreen> createState() => _BookingFlowScreenState();
}

class _BookingFlowScreenState extends ConsumerState<BookingFlowScreen> {
  late final Future<List<CatalogService>> _catalog = ref.read(bookingRepositoryProvider).catalog();
  CatalogService? _service;
  DateTime _date = DateTime.now();
  Future<List<Slot>>? _slots;
  Slot? _slot;
  final _name = TextEditingController();
  final _contact = TextEditingController();
  bool _voice = false;
  bool _consent = false;
  bool _busy = false;
  String? _error;
  String? _idemKey; // satu kunci per percobaan; retry jaringan tidak membuat booking ganda

  @override
  void dispose() {
    _name.dispose();
    _contact.dispose();
    super.dispose();
  }

  void _loadSlots() {
    final s = _service;
    if (s == null) return;
    setState(() {
      _slot = null;
      _slots = ref.read(bookingRepositoryProvider).slots(s.serviceId, _date);
    });
  }

  String _newKey() {
    final r = math.Random.secure();
    return List.generate(32, (_) => r.nextInt(16).toRadixString(16)).join();
  }

  Future<void> _submit() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    _idemKey ??= _newKey();
    try {
      final b = await ref.read(bookingRepositoryProvider).create(
            idempotencyKey: _idemKey!,
            slotId: _slot!.id,
            serviceId: _service!.serviceId,
            name: _name.text.trim(),
            contact: _contact.text.trim(),
            allowVoiceName: _voice,
          );
      if (mounted) context.go('/booking/${b.reference}');
    } on ApiException catch (e) {
      if (e.code == 'SLOT_FULL' || e.code == 'VALIDATION_ERROR') _idemKey = null;
      setState(() => _error = e.code == 'SLOT_FULL' ? 'Slot sudah penuh, pilih slot lain.' : 'Pendaftaran gagal (${e.code}).');
    } catch (_) {
      setState(() => _error = 'Tidak dapat terhubung ke server.');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final fmt = DateFormat('HH:mm');
    return Scaffold(
      appBar: AppBar(title: const Text('Daftar antrean')),
      body: FutureBuilder<List<CatalogService>>(
        future: _catalog,
        builder: (context, snap) {
          if (snap.hasError) return const Center(child: Text('Layanan belum tersedia.'));
          if (!snap.hasData) return const Center(child: CircularProgressIndicator());
          final items = snap.data!;
          return ListView(padding: const EdgeInsets.all(16), children: [
            DropdownButtonFormField<CatalogService>(
              decoration: const InputDecoration(labelText: 'Layanan'),
              items: [for (final s in items) DropdownMenuItem(value: s, child: Text('${s.displayName} — ${s.siteName}'))],
              onChanged: (v) {
                _service = v;
                _loadSlots();
              },
            ),
            const SizedBox(height: 12),
            ListTile(
              contentPadding: EdgeInsets.zero,
              title: Text('Tanggal: ${DateFormat('dd MMM yyyy').format(_date)}'),
              trailing: const Icon(Icons.calendar_today),
              onTap: () async {
                final d = await showDatePicker(
                    context: context, initialDate: _date, firstDate: DateTime.now(), lastDate: DateTime.now().add(const Duration(days: 60)));
                if (d != null) {
                  _date = d;
                  _loadSlots();
                }
              },
            ),
            if (_slots != null)
              FutureBuilder<List<Slot>>(
                future: _slots,
                builder: (context, s) {
                  if (!s.hasData) return const LinearProgressIndicator();
                  if (s.data!.isEmpty) return const Text('Tidak ada slot pada tanggal ini.');
                  return Wrap(spacing: 8, runSpacing: 8, children: [
                    for (final sl in s.data!)
                      ChoiceChip(
                        label: Text('${fmt.format(sl.startsAt.toLocal())} (${sl.remaining} sisa)'),
                        selected: _slot?.id == sl.id,
                        onSelected: sl.remaining == 0 ? null : (_) => setState(() => _slot = sl),
                      ),
                  ]);
                },
              ),
            TextField(controller: _name, decoration: const InputDecoration(labelText: 'Nama'), autofillHints: const [AutofillHints.name]),
            TextField(
                controller: _contact,
                decoration: const InputDecoration(labelText: 'Nomor HP'),
                keyboardType: TextInputType.phone,
                autofillHints: const [AutofillHints.telephoneNumber]),
            CheckboxListTile(
              contentPadding: EdgeInsets.zero,
              value: _voice,
              onChanged: (v) => setState(() => _voice = v ?? false),
              title: const Text('Izinkan nama depan saya disebut saat dipanggil (opsional)'),
              subtitle: const Text('Jika tidak dicentang, hanya nomor antrean yang disebut.'),
            ),
            CheckboxListTile(
              contentPadding: EdgeInsets.zero,
              value: _consent,
              onChanged: (v) => setState(() => _consent = v ?? false),
              title: const Text('Saya telah membaca pemberitahuan privasi (2026-10-v1).'),
            ),
            if (_error != null) Text(_error!, style: TextStyle(color: Theme.of(context).colorScheme.error)),
            const SizedBox(height: 8),
            FilledButton(
              onPressed: _busy || _slot == null || !_consent || _name.text.trim().length < 2 ? null : _submit,
              child: Text(_busy ? 'Memproses…' : 'Konfirmasi pendaftaran'),
            ),
            const SizedBox(height: 8),
            const Text('Nomor antrean diterbitkan saat Anda scan QR di kiosk.'),
          ]);
        },
      ),
    );
  }
}
