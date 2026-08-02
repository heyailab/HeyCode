/// AppConfig：后端连接配置。
///
/// baseUrl 由用户在设置页填写（如 http://192.168.1.10:8787）。
/// wsUrl 由 baseUrl 推导：http→ws、https→wss，路径 /ws。
class AppConfig {
  final String baseUrl;
  final String wsUrl;

  const AppConfig({required this.baseUrl, required this.wsUrl});

  factory AppConfig.fromBaseUrl(String baseUrl) {
    final ws = baseUrl
        .replaceFirst('https://', 'wss://')
        .replaceFirst('http://', 'ws://');
    return AppConfig(baseUrl: baseUrl, wsUrl: '$ws/ws');
  }

  String get healthUrl => '$baseUrl/api/health';

  AppConfig copyWith({String? baseUrl}) =>
      AppConfig.fromBaseUrl(baseUrl ?? this.baseUrl);

  static const defaultBaseUrl = 'http://localhost:8787';
  static const empty = AppConfig(baseUrl: '', wsUrl: '');
}
