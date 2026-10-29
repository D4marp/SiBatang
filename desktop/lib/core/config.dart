/// Tidak ada tenant/site/API key tertanam di build. Identitas perangkat berasal dari aktivasi sekali pakai.
class AppConfig {
  static const apiBaseUrl =
      String.fromEnvironment('API_BASE_URL', defaultValue: 'http://localhost:8080/api/v1');
}
