/// Base URL API ditetapkan saat build: --dart-define=API_BASE_URL=https://<host-resmi>/api/v1
/// Tidak ada tenant selector: pilot hanya SIBATANG_BATANG (slug tetap).
class AppConfig {
  static const apiBaseUrl =
      String.fromEnvironment('API_BASE_URL', defaultValue: 'http://10.0.2.2:8080/api/v1');
  static const tenantSlug = 'sibatang-batang';
  static const tenantCode = 'SIBATANG_BATANG';
  static const privacyNoticeVersion = '2026-10-v1';
}
