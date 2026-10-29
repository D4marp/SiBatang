/// Cetak tiket untuk NOMOR YANG SUDAH DITERBITKAN backend. Kegagalan cetak tidak boleh memicu check-in ulang.
abstract interface class TicketPrinter {
  Future<bool> print(String displayNumber, {required bool isReprint});
}

/// Placeholder sampai model printer + driver disahkan (procurement gate). Implementasi nyata: WindowsSpoolPrinter / ESC-POS.
class MockPrinter implements TicketPrinter {
  final List<String> printed = [];

  @override
  Future<bool> print(String displayNumber, {required bool isReprint}) async {
    printed.add(displayNumber);
    return true;
  }
}
