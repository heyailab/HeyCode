/// AppConfig：后端连接配置。
///
/// baseUrl 由用户在设置页填写（如 http://192.168.1.10:8787）。
/// wsUrl 由 baseUrl 推导：http→ws、https→wss，路径 /ws。
/// authToken 用于 Bearer 鉴权（后端配置了 AUTH_TOKEN 时必填）；
///   为空时不带 Authorization 头（兼容后端鉴权未启用的本地调试场景）。
class AppConfig {
  final String baseUrl;
  final String wsUrl;
  final String authToken;

  const AppConfig({required this.baseUrl, required this.wsUrl, this.authToken = ''});

  factory AppConfig.fromBaseUrl(String baseUrl, {String authToken = ''}) {
    final ws = baseUrl
        .replaceFirst('https://', 'wss://')
        .replaceFirst('http://', 'ws://');
    return AppConfig(baseUrl: baseUrl, wsUrl: '$ws/ws', authToken: authToken);
  }

  String get healthUrl => '$baseUrl/api/health';

  /// 是否配置了 token（用于 UI 提示，不代表后端是否启用鉴权）。
  bool get hasAuthToken => authToken.isNotEmpty;

  AppConfig copyWith({String? baseUrl, String? authToken}) =>
      AppConfig.fromBaseUrl(
        baseUrl ?? this.baseUrl,
        authToken: authToken ?? this.authToken,
      );

  static const defaultBaseUrl = 'http://localhost:8787';
  static const empty = AppConfig(baseUrl: '', wsUrl: '');
}
