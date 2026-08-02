/// WsClient：WebSocket 客户端，状态机 + 重连 + 心跳。
///
/// 状态机：disconnected → connecting → connected
///                           ↓ 断线
///                       reconnecting → 3s → connecting
///
/// 重连后若已初始化且 _lastEventId > 0，发 SessionResyncCommand 请求增量。
/// 双层心跳：协议层 20s + 应用层 15s。
library;

import 'dart:async';
import 'dart:convert';
import 'dart:io' show Platform;

import 'package:web_socket_channel/io.dart';
import 'package:web_socket_channel/web_socket_channel.dart';

import '../models/unified_event.dart';

enum WsConnectionState { disconnected, connecting, connected, reconnecting }

/// WebSocket 工厂：移动端用 IOWebSocketChannel（更稳定）。
WebSocketChannel buildWebSocketChannel(Uri uri) {
  if (Platform.isAndroid ||
      Platform.isIOS ||
      Platform.isMacOS ||
      Platform.isLinux ||
      Platform.isWindows) {
    return IOWebSocketChannel.connect(uri, pingInterval: const Duration(seconds: 20));
  }
  return WebSocketChannel.connect(uri);
}

class WsClient {
  WebSocketChannel? _channel;
  StreamSubscription? _socketSub;
  Timer? _heartbeatTimer;
  Timer? _reconnectTimer;
  String? _sessionId;
  int _lastEventId = 0;
  bool _disposed = false;
  bool _initialized = false; // 是否已收到 session.init

  String _wsUrl = '';
  String _authToken = '';

  final _stateController = StreamController<WsConnectionState>.broadcast();
  final _envelopeController = StreamController<ServerEnvelope>.broadcast();

  Stream<WsConnectionState> get stateStream => _stateController.stream;
  Stream<ServerEnvelope> get envelopeStream => _envelopeController.stream;

  WsConnectionState _state = WsConnectionState.disconnected;
  WsConnectionState get state => _state;

  set _setState(WsConnectionState s) {
    _state = s;
    _stateController.add(s);
  }

  /// 设置 wsUrl（由 providers 在 config 变更时调用）。
  void setWsUrl(String url) {
    _wsUrl = url;
  }

  /// 设置鉴权 token（由 providers 在 config 变更时调用）。
  /// 非空时连接 URL 会附加 ?token= query param。
  void setAuthToken(String token) {
    _authToken = token;
  }

  /// 连接到指定会话。
  /// 切换会话时不清零 _lastEventId（依赖服务端 eventId 全局递增）。
  Future<void> connect(String sessionId) async {
    // 清理旧连接
    _reconnectTimer?.cancel();
    _stopHeartbeat();
    await _socketSub?.cancel();
    await _channel?.sink.close();

    _initialized = false;
    _sessionId = sessionId;
    _setState = WsConnectionState.connecting;
    _doConnect();
  }

  void _doConnect() {
    if (_disposed || _sessionId == null || _wsUrl.isEmpty) return;
    try {
      // 拼接鉴权 query param（浏览器 WS 不能设 header，统一走 ?token=）。
      final base = '$_wsUrl/$_sessionId';
      final uri = _authToken.isEmpty
          ? Uri.parse(base)
          : Uri.parse(base).replace(queryParameters: {
              'token': _authToken,
            });
      _channel = buildWebSocketChannel(uri);
      _socketSub = _channel!.stream.listen(
        _onData,
        onError: (Object e) => _onError(e),
        onDone: () => _onDone(),
      );
      _setState = WsConnectionState.connected;
      _startHeartbeat();

      // 重连后请求增量
      if (_initialized && _lastEventId > 0) {
        _sendRaw(SessionResyncCommand(sinceEventId: '$_lastEventId').toJson());
      }
    } catch (e) {
      _scheduleReconnect();
    }
  }

  void _scheduleReconnect() {
    if (_disposed) return;
    _setState = WsConnectionState.reconnecting;
    _reconnectTimer = Timer(const Duration(seconds: 3), () {
      if (!_disposed) _doConnect();
    });
  }

  // ---- 心跳 ----

  void _startHeartbeat() {
    _stopHeartbeat();
    _heartbeatTimer = Timer.periodic(const Duration(seconds: 15), (_) {
      _sendRaw({'type': 'ping'});
    });
  }

  void _stopHeartbeat() {
    _heartbeatTimer?.cancel();
    _heartbeatTimer = null;
  }

  // ---- 消息收发 ----

  void _sendRaw(Map<String, dynamic> payload) {
    _channel?.sink.add(jsonEncode(payload));
  }

  void sendCommand(ClientCommand cmd) => _sendRaw(cmd.toJson());

  void _onData(dynamic data) {
    if (data is! String) return;
    Map<String, dynamic>? json;
    try {
      json = jsonDecode(data) as Map<String, dynamic>;
    } catch (_) {
      return;
    }
    final type = json['type'];
    if (type == 'pong' || type == 'ping') return; // 心跳忽略

    final env = ServerEnvelope.fromJson(json);
    if (env.eventId > _lastEventId) {
      _lastEventId = env.eventId;
    }
    if (env.event is SessionInitEvent) {
      _initialized = true;
    }
    _envelopeController.add(env);
  }

  void _onError(Object e) {
    _scheduleReconnect();
  }

  void _onDone() {
    if (!_disposed) {
      _scheduleReconnect();
    }
  }

  // ---- 生命周期 ----

  void dispose() {
    _disposed = true;
    _reconnectTimer?.cancel();
    _stopHeartbeat();
    _socketSub?.cancel();
    _channel?.sink.close();
    _setState = WsConnectionState.disconnected;
  }

  void close() {
    dispose();
    _stateController.close();
    _envelopeController.close();
  }
}
