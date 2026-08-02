/// SessionController：会话状态控制器。
///
/// 订阅 WsClient 的 stateStream 和 envelopeStream，
/// 通过 switch 模式匹配分发事件到对应状态字段。
/// 支持四种启动入口：startSessionRaw / startSessionWithTask /
/// startExistingSession / resumeSession。
library;

import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../models/project.dart';
import '../models/session.dart' show SessionStatus;
import '../models/unified_event.dart';
import '../services/api_client.dart';
import '../services/ws_client.dart';

// ---- 辅助数据类 ----

class ChatMessage {
  final String role; // 'user' | 'assistant'
  final List<ContentBlock> blocks;
  final int timestamp;

  const ChatMessage({
    required this.role,
    required this.blocks,
    required this.timestamp,
  });

  ChatMessage copyWith({List<ContentBlock>? blocks}) =>
      ChatMessage(role: role, blocks: blocks ?? this.blocks, timestamp: timestamp);
}

class ToolCall {
  final String toolUseId;
  final String toolName;
  final Object? input;
  final String? result;
  final bool isError;
  final bool done;
  final int timestamp;

  const ToolCall({
    required this.toolUseId,
    required this.toolName,
    this.input,
    this.result,
    this.isError = false,
    this.done = false,
    required this.timestamp,
  });

  ToolCall copyWith({
    String? result,
    bool? isError,
    bool? done,
  }) =>
      ToolCall(
        toolUseId: toolUseId,
        toolName: toolName,
        input: input,
        result: result ?? this.result,
        isError: isError ?? this.isError,
        done: done ?? this.done,
        timestamp: timestamp,
      );
}

// ---- State ----

class SessionControllerState {
  final String? sessionId;
  final SessionStatus status;
  final CliKind? cli;
  final String? model;
  final String? cwd;
  final List<ChatMessage> messages;
  final List<ToolCall> toolCalls;
  final List<FileChange> fileChanges;
  final List<CommandExecEvent> commandLogs;
  final List<TodoItem> todos;
  final ProgressEvent? progress;
  final String? errorMessage;
  final WsConnectionState wsState;
  final SessionStats? stats;

  const SessionControllerState({
    this.sessionId,
    this.status = SessionStatus.idle,
    this.cli,
    this.model,
    this.cwd,
    this.messages = const [],
    this.toolCalls = const [],
    this.fileChanges = const [],
    this.commandLogs = const [],
    this.todos = const [],
    this.progress,
    this.errorMessage,
    this.wsState = WsConnectionState.disconnected,
    this.stats,
  });

  SessionControllerState copyWith({
    String? sessionId,
    SessionStatus? status,
    CliKind? cli,
    String? model,
    String? cwd,
    List<ChatMessage>? messages,
    List<ToolCall>? toolCalls,
    List<FileChange>? fileChanges,
    List<CommandExecEvent>? commandLogs,
    List<TodoItem>? todos,
    ProgressEvent? progress,
    String? errorMessage,
    WsConnectionState? wsState,
    SessionStats? stats,
    bool clearError = false,
    bool clearProgress = false,
  }) =>
      SessionControllerState(
        sessionId: sessionId ?? this.sessionId,
        status: status ?? this.status,
        cli: cli ?? this.cli,
        model: model ?? this.model,
        cwd: cwd ?? this.cwd,
        messages: messages ?? this.messages,
        toolCalls: toolCalls ?? this.toolCalls,
        fileChanges: fileChanges ?? this.fileChanges,
        commandLogs: commandLogs ?? this.commandLogs,
        todos: todos ?? this.todos,
        progress: clearProgress ? null : (progress ?? this.progress),
        errorMessage: clearError ? null : (errorMessage ?? this.errorMessage),
        wsState: wsState ?? this.wsState,
        stats: stats ?? this.stats,
      );
}

// ---- SessionController ----

class SessionController extends StateNotifier<SessionControllerState> {
  final ApiClient _api;
  final WsClient _ws;

  StreamSubscription<WsConnectionState>? _stateSub;
  StreamSubscription<ServerEnvelope>? _envSub;

  /// 流式增量聚合：messageId → messages 列表下标。
  final Map<String, int> _streamingIndex = {};

  SessionController({required ApiClient api, required WsClient ws})
      : _api = api,
        _ws = ws,
        super(const SessionControllerState()) {
    _init();
  }

  void _init() {
    _stateSub = _ws.stateStream.listen((s) {
      if (mounted) state = state.copyWith(wsState: s);
    });
    _envSub = _ws.envelopeStream.listen((env) => _onEnvelope(env));
  }

  // ---- 启动入口 ----

  /// (a) 先 REST 创建 session，再 WS 连接 + 发 session.start。
  Future<void> startSessionRaw({
    required String taskId,
    required String serverId,
    required String cwd,
    required CliKind cli,
    required String prompt,
    String? model,
    String? resumeCliSessionId,
    List<String>? allowedTools,
  }) async {
    state = state.copyWith(
      status: SessionStatus.running,
      cli: cli,
      model: model,
      cwd: cwd,
      wsState: WsConnectionState.connecting,
      clearError: true,
    );
    try {
      final session = await _api.createSession(taskId: taskId, cli: cli, model: model);
      state = state.copyWith(sessionId: session.id);
      await _ws.connect(session.id);
      _ws.sendCommand(SessionStartCommand(
        serverId: serverId,
        cwd: cwd,
        cli: cli,
        prompt: prompt,
        model: model,
        resumeCliSessionId: resumeCliSessionId,
        allowedTools: allowedTools,
      ));
    } catch (e) {
      state = state.copyWith(status: SessionStatus.error, errorMessage: e.toString());
    }
  }

  /// (b) 带 Project 对象的便捷版本。
  Future<void> startSessionWithTask({
    required String taskId,
    required String serverId,
    required Project project,
    required String prompt,
  }) async {
    await startSessionRaw(
      taskId: taskId,
      serverId: serverId,
      cwd: project.cwd,
      cli: project.defaultCli,
      prompt: prompt,
      model: project.defaultModel,
    );
  }

  /// (c) session 记录已由上层 REST 创建，只连 WS + 发首条指令。
  Future<void> startExistingSession({
    required String sessionId,
    required String serverId,
    required String cwd,
    required CliKind cli,
    required String prompt,
    String? model,
    String? resumeCliSessionId,
  }) async {
    state = state.copyWith(
      sessionId: sessionId,
      status: SessionStatus.running,
      cli: cli,
      model: model,
      cwd: cwd,
      wsState: WsConnectionState.connecting,
      clearError: true,
    );
    try {
      await _ws.connect(sessionId);
      _ws.sendCommand(SessionStartCommand(
        serverId: serverId,
        cwd: cwd,
        cli: cli,
        prompt: prompt,
        model: model,
        resumeCliSessionId: resumeCliSessionId,
      ));
    } catch (e) {
      state = state.copyWith(status: SessionStatus.error, errorMessage: e.toString());
    }
  }

  /// (d) 恢复已有会话：拉历史事件回放 + 连 WS 接收增量。
  Future<void> resumeSession({required String sessionId}) async {
    state = state.copyWith(
      sessionId: sessionId,
      status: SessionStatus.running,
      wsState: WsConnectionState.connecting,
      clearError: true,
    );
    try {
      final envelopes = await _api.listSessionEvents(sessionId);
      for (final env in envelopes) {
        _onEnvelope(env, replay: true);
      }
      await _ws.connect(sessionId);
    } catch (e) {
      state = state.copyWith(status: SessionStatus.error, errorMessage: e.toString());
    }
  }

  // ---- 事件分发 ----

  void _onEnvelope(ServerEnvelope env, {bool replay = false}) {
    if (!mounted) return;
    final event = env.event;
    switch (event) {
      case SessionInitEvent():
        state = state.copyWith(
          sessionId: event.sessionId,
          cli: event.cli,
          model: event.model,
          cwd: event.cwd,
          status: SessionStatus.running,
        );
      case MessageEvent():
        final msg = ChatMessage(
          role: event.role,
          blocks: event.blocks,
          timestamp: event.timestamp,
        );
        state = state.copyWith(messages: [...state.messages, msg]);
      case StreamingDeltaEvent():
        _applyDelta(event);
      case StreamingDoneEvent():
        _streamingIndex.remove(event.messageId);
        state = state.copyWith(status: SessionStatus.idle);
      case ToolUseEvent():
        final tc = ToolCall(
          toolUseId: event.toolUseId,
          toolName: event.toolName,
          input: event.input,
          timestamp: event.timestamp,
        );
        state = state.copyWith(toolCalls: [...state.toolCalls, tc]);
      case ToolResultEvent():
        final toolCalls = state.toolCalls.map((tc) {
          if (tc.toolUseId == event.toolUseId) {
            return tc.copyWith(
              result: event.output,
              isError: event.isError ?? false,
              done: true,
            );
          }
          return tc;
        }).toList();
        state = state.copyWith(toolCalls: toolCalls);
      case FileChangeEvent():
        state = state.copyWith(fileChanges: [...state.fileChanges, event.change]);
      case CommandExecEvent():
        state = state.copyWith(commandLogs: [...state.commandLogs, event]);
      case TodoUpdateEvent():
        state = state.copyWith(todos: event.todos);
      case ThinkingEvent():
        final msg = ChatMessage(
          role: 'assistant',
          blocks: [ThinkingBlock(text: event.text)],
          timestamp: event.timestamp,
        );
        state = state.copyWith(messages: [...state.messages, msg]);
      case ProgressEvent():
        state = state.copyWith(progress: event);
      case ErrorEvent():
        final newStatus = event.recoverable == false ? SessionStatus.error : state.status;
        state = state.copyWith(
          errorMessage: event.message,
          status: newStatus,
        );
      case SessionEndEvent():
        state = state.copyWith(
          status: SessionStatus.ended,
          stats: event.stats,
        );
    }
  }

  // ---- 流式增量聚合 ----

  void _applyDelta(StreamingDeltaEvent e) {
    final delta = e.textDelta;
    if (delta == null || delta.isEmpty) return;

    final idx = _streamingIndex[e.messageId];
    if (idx == null || idx >= state.messages.length) {
      // 新建一条 assistant 消息
      final msg = ChatMessage(
        role: 'assistant',
        blocks: [TextBlock(text: delta)],
        timestamp: e.timestamp,
      );
      _streamingIndex[e.messageId] = state.messages.length;
      state = state.copyWith(messages: [...state.messages, msg]);
      return;
    }

    // 已存在，追加到 TextBlock
    final msg = state.messages[idx];
    final blocks = [...msg.blocks];
    final last = blocks.isNotEmpty ? blocks.last : null;
    if (last is TextBlock) {
      blocks[blocks.length - 1] = TextBlock(text: last.text + delta);
    } else {
      blocks.add(TextBlock(text: delta));
    }
    final newMsg = msg.copyWith(blocks: blocks);
    final newMessages = [...state.messages]..[idx] = newMsg;
    state = state.copyWith(messages: newMessages);
  }

  // ---- 用户操作 ----

  void sendMessage(String prompt) {
    final userMsg = ChatMessage(
      role: 'user',
      blocks: [TextBlock(text: prompt)],
      timestamp: DateTime.now().millisecondsSinceEpoch,
    );
    state = state.copyWith(
      messages: [...state.messages, userMsg],
      status: SessionStatus.running,
      clearError: true,
    );
    _ws.sendCommand(SessionSendCommand(prompt: prompt));
  }

  void interruptSession() {
    _ws.sendCommand(const SessionInterruptCommand());
  }

  void endSession() {
    _ws.sendCommand(const SessionEndCommand());
  }

  // ---- 清理 ----

  @override
  void dispose() {
    _envSub?.cancel();
    _stateSub?.cancel();
    _ws.dispose(); // 仅断连，不关闭流（WsClient 是共享单例）
    super.dispose();
  }
}
