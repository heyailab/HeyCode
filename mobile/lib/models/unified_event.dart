/// UnifiedEvent 及相关类型：CliKind、ContentBlock、ClientCommand、ServerEnvelope。
///
/// 所有事件基于 sealed class + type wire 分发。
/// fromJson 对缺失字段有默认回退，永不抛异常。
/// 未知事件 type 回退 ErrorEvent。
library;

import 'dart:convert';

// ---- CliKind ----

/// CLI 种类枚举，全局共用。默认回退 pty。
enum CliKind {
  claudeCode('claude-code'),
  codex('codex'),
  gemini('gemini'),
  trae('trae'),
  opencode('opencode'),
  lingma('lingma'),
  pty('pty');

  final String wire;
  const CliKind(this.wire);

  static CliKind fromWire(String? v) =>
      values.firstWhere((e) => e.wire == v, orElse: () => CliKind.pty);
}

// ---- ContentBlock sealed ----

/// 消息内容块，sealed union。
sealed class ContentBlock {
  const ContentBlock();

  factory ContentBlock.fromJson(Map<String, dynamic> j) {
    final type = j['type'] as String?;
    switch (type) {
      case 'text':
        return TextBlock(text: j['text'] as String? ?? '');
      case 'thinking':
        return ThinkingBlock(
          text: j['text'] as String? ?? '',
          signature: j['signature'] as String?,
        );
      case 'image':
        return ImageBlock(
          mimeType: j['mimeType'] as String? ?? '',
          dataB64: j['dataB64'] as String? ?? '',
        );
      case 'tool_use':
        return ToolUseBlock(
          toolUseId: j['toolUseId'] as String? ?? '',
          toolName: j['toolName'] as String? ?? '',
          input: j['input'],
        );
      case 'tool_result':
        return ToolResultBlock(
          toolUseId: j['toolUseId'] as String? ?? '',
          output: j['output'],
          isError: j['isError'] as bool?,
        );
      default:
        return TextBlock(text: '<未知 block: $type>');
    }
  }

  Map<String, dynamic> toJson();
}

class TextBlock extends ContentBlock {
  final String text;
  const TextBlock({required this.text});

  @override
  Map<String, dynamic> toJson() => {'type': 'text', 'text': text};
}

class ThinkingBlock extends ContentBlock {
  final String text;
  final String? signature;
  const ThinkingBlock({required this.text, this.signature});

  @override
  Map<String, dynamic> toJson() => {
        'type': 'thinking',
        'text': text,
        if (signature != null) 'signature': signature,
      };
}

class ImageBlock extends ContentBlock {
  final String mimeType;
  final String dataB64;
  const ImageBlock({required this.mimeType, required this.dataB64});

  @override
  Map<String, dynamic> toJson() => {
        'type': 'image',
        'mimeType': mimeType,
        'dataB64': dataB64,
      };
}

class ToolUseBlock extends ContentBlock {
  final String toolUseId;
  final String toolName;
  final Object? input;
  const ToolUseBlock({required this.toolUseId, required this.toolName, this.input});

  @override
  Map<String, dynamic> toJson() => {
        'type': 'tool_use',
        'toolUseId': toolUseId,
        'toolName': toolName,
        if (input != null) 'input': input,
      };
}

class ToolResultBlock extends ContentBlock {
  final String toolUseId;
  final Object? output;
  final bool? isError;
  const ToolResultBlock({required this.toolUseId, this.output, this.isError});

  /// output 转字符串：String 直接返回；Map 含 type 字段则按类型处理；其它 toString()。
  String get outputAsString {
    final o = output;
    if (o is String) return o;
    if (o is Map<String, dynamic>) {
      final type = o['type'];
      if (type == 'json') {
        return const JsonEncoder.withIndent('  ').convert(o['data'] ?? o);
      }
      if (type == 'image') return '[图片]';
    }
    return o?.toString() ?? '';
  }

  @override
  Map<String, dynamic> toJson() => {
        'type': 'tool_result',
        'toolUseId': toolUseId,
        if (output != null) 'output': output,
        if (isError != null) 'isError': isError,
      };
}

// ---- FileChange ----

enum FileChangeAction {
  create('create'),
  edit('edit'),
  delete('delete');

  final String wire;
  const FileChangeAction(this.wire);

  static FileChangeAction fromWire(String? v) =>
      values.firstWhere((e) => e.wire == v, orElse: () => FileChangeAction.edit);
}

class FileChange {
  final String path;
  final FileChangeAction action;
  final String? diff;
  final int? addedLines;
  final int? removedLines;

  const FileChange({
    required this.path,
    required this.action,
    this.diff,
    this.addedLines,
    this.removedLines,
  });

  factory FileChange.fromJson(Map<String, dynamic> j) => FileChange(
        path: j['path'] as String? ?? '',
        action: FileChangeAction.fromWire(j['action'] as String?),
        diff: j['diff'] as String?,
        addedLines: j['addedLines'] as int?,
        removedLines: j['removedLines'] as int?,
      );
}

// ---- TodoItem ----

enum TodoStatus {
  pending('pending'),
  inProgress('in_progress'),
  completed('completed');

  final String wire;
  const TodoStatus(this.wire);

  static TodoStatus fromWire(String? v) =>
      values.firstWhere((e) => e.wire == v, orElse: () => TodoStatus.pending);
}

class TodoItem {
  final String id;
  final String content;
  final TodoStatus status;
  final int? progress;

  const TodoItem({
    required this.id,
    required this.content,
    required this.status,
    this.progress,
  });

  /// 唯一提供 copyWith 的 model。
  TodoItem copyWith({
    String? id,
    String? content,
    TodoStatus? status,
    int? progress,
  }) =>
      TodoItem(
        id: id ?? this.id,
        content: content ?? this.content,
        status: status ?? this.status,
        progress: progress ?? this.progress,
      );

  factory TodoItem.fromJson(Map<String, dynamic> j) => TodoItem(
        id: j['id'] as String? ?? '',
        content: j['content'] as String? ?? '',
        status: TodoStatus.fromWire(j['status'] as String?),
        progress: j['progress'] as int?,
      );
}

// ---- SessionStats ----

class SessionStats {
  final double? costUsd;
  final int? durationMs;
  final int? numTurns;
  final int? inputTokens;
  final int? outputTokens;

  const SessionStats({
    this.costUsd,
    this.durationMs,
    this.numTurns,
    this.inputTokens,
    this.outputTokens,
  });

  factory SessionStats.fromJson(Map<String, dynamic> j) => SessionStats(
        costUsd: (j['costUsd'] as num?)?.toDouble(),
        durationMs: j['durationMs'] as int?,
        numTurns: j['numTurns'] as int?,
        inputTokens: j['inputTokens'] as int?,
        outputTokens: j['outputTokens'] as int?,
      );
}

// ---- UnifiedEvent sealed ----

/// 所有事件基类。
sealed class UnifiedEvent {
  final int timestamp;

  const UnifiedEvent({required this.timestamp});

  /// 事件类型 wire 字符串。
  String get type;

  factory UnifiedEvent.fromJson(Map<String, dynamic> j) {
    final ts = j['timestamp'] as int? ?? DateTime.now().millisecondsSinceEpoch;
    final type = j['type'] as String?;
    switch (type) {
      case 'session.init':
        return SessionInitEvent(
          timestamp: ts,
          sessionId: j['sessionId'] as String? ?? '',
          cliSessionId: j['cliSessionId'] as String?,
          cli: CliKind.fromWire(j['cli'] as String?),
          model: j['model'] as String?,
          cwd: j['cwd'] as String? ?? '',
        );
      case 'message':
        final blocks = (j['blocks'] as List?)
                ?.map((b) => ContentBlock.fromJson(b as Map<String, dynamic>))
                .toList() ??
            const [];
        return MessageEvent(
          timestamp: ts,
          role: j['role'] as String? ?? '',
          blocks: blocks,
        );
      case 'streaming.delta':
        return StreamingDeltaEvent(
          timestamp: ts,
          messageId: j['messageId'] as String? ?? '',
          textDelta: j['textDelta'] as String?,
        );
      case 'streaming.done':
        return StreamingDoneEvent(
          timestamp: ts,
          messageId: j['messageId'] as String? ?? '',
        );
      case 'tool.use':
        return ToolUseEvent(
          timestamp: ts,
          toolUseId: j['toolUseId'] as String? ?? '',
          toolName: j['toolName'] as String? ?? '',
          input: j['input'],
        );
      case 'tool.result':
        return ToolResultEvent(
          timestamp: ts,
          toolUseId: j['toolUseId'] as String? ?? '',
          output: j['output']?.toString() ?? '',
          isError: j['isError'] as bool?,
        );
      case 'file.change':
        return FileChangeEvent(
          timestamp: ts,
          change: j['change'] is Map<String, dynamic>
              ? FileChange.fromJson(j['change'] as Map<String, dynamic>)
              : const FileChange(path: '', action: FileChangeAction.edit),
          toolUseId: j['toolUseId'] as String?,
        );
      case 'command.exec':
        return CommandExecEvent(
          timestamp: ts,
          command: j['command'] as String? ?? '',
          cwd: j['cwd'] as String?,
          exitCode: j['exitCode'] as int?,
          stdout: j['stdout'] as String?,
          stderr: j['stderr'] as String?,
          toolUseId: j['toolUseId'] as String?,
        );
      case 'todo.update':
        final todos = (j['todos'] as List?)
                ?.map((t) => TodoItem.fromJson(t as Map<String, dynamic>))
                .toList() ??
            const [];
        return TodoUpdateEvent(timestamp: ts, todos: todos);
      case 'thinking':
        return ThinkingEvent(
          timestamp: ts,
          text: j['text'] as String? ?? '',
        );
      case 'progress':
        return ProgressEvent(
          timestamp: ts,
          step: j['step'] as int?,
          total: j['total'] as int?,
          message: j['message'] as String?,
        );
      case 'error':
        return ErrorEvent(
          timestamp: ts,
          message: j['message'] as String? ?? '',
          recoverable: j['recoverable'] as bool?,
          cli: j['cli'] as String?,
        );
      case 'session.end':
        return SessionEndEvent(
          timestamp: ts,
          stats: j['stats'] is Map<String, dynamic>
              ? SessionStats.fromJson(j['stats'] as Map<String, dynamic>)
              : null,
        );
      default:
        return ErrorEvent(
          timestamp: ts,
          message: '未知事件类型: $type',
          recoverable: true,
        );
    }
  }

  Map<String, dynamic> toJson();
}

class SessionInitEvent extends UnifiedEvent {
  final String sessionId;
  final String? cliSessionId;
  final CliKind cli;
  final String? model;
  final String cwd;

  const SessionInitEvent({
    required super.timestamp,
    required this.sessionId,
    this.cliSessionId,
    required this.cli,
    this.model,
    required this.cwd,
  });

  @override
  String get type => 'session.init';

  @override
  Map<String, dynamic> toJson() => {
        'type': type,
        'timestamp': timestamp,
        'sessionId': sessionId,
        if (cliSessionId != null) 'cliSessionId': cliSessionId,
        'cli': cli.wire,
        if (model != null) 'model': model,
        'cwd': cwd,
      };
}

class MessageEvent extends UnifiedEvent {
  final String role;
  final List<ContentBlock> blocks;

  const MessageEvent({
    required super.timestamp,
    required this.role,
    required this.blocks,
  });

  @override
  String get type => 'message';

  @override
  Map<String, dynamic> toJson() => {
        'type': type,
        'timestamp': timestamp,
        'role': role,
        'blocks': blocks.map((b) => b.toJson()).toList(),
      };
}

class StreamingDeltaEvent extends UnifiedEvent {
  final String messageId;
  final String? textDelta;

  const StreamingDeltaEvent({
    required super.timestamp,
    required this.messageId,
    this.textDelta,
  });

  @override
  String get type => 'streaming.delta';

  @override
  Map<String, dynamic> toJson() => {
        'type': type,
        'timestamp': timestamp,
        'messageId': messageId,
        if (textDelta != null) 'textDelta': textDelta,
      };
}

class StreamingDoneEvent extends UnifiedEvent {
  final String messageId;

  const StreamingDoneEvent({
    required super.timestamp,
    required this.messageId,
  });

  @override
  String get type => 'streaming.done';

  @override
  Map<String, dynamic> toJson() => {
        'type': type,
        'timestamp': timestamp,
        'messageId': messageId,
      };
}

class ToolUseEvent extends UnifiedEvent {
  final String toolUseId;
  final String toolName;
  final Object? input;

  const ToolUseEvent({
    required super.timestamp,
    required this.toolUseId,
    required this.toolName,
    this.input,
  });

  @override
  String get type => 'tool.use';

  @override
  Map<String, dynamic> toJson() => {
        'type': type,
        'timestamp': timestamp,
        'toolUseId': toolUseId,
        'toolName': toolName,
        if (input != null) 'input': input,
      };
}

class ToolResultEvent extends UnifiedEvent {
  final String toolUseId;
  final String output;
  final bool? isError;

  const ToolResultEvent({
    required super.timestamp,
    required this.toolUseId,
    required this.output,
    this.isError,
  });

  @override
  String get type => 'tool.result';

  @override
  Map<String, dynamic> toJson() => {
        'type': type,
        'timestamp': timestamp,
        'toolUseId': toolUseId,
        'output': output,
        if (isError != null) 'isError': isError,
      };
}

class FileChangeEvent extends UnifiedEvent {
  final FileChange change;
  final String? toolUseId;

  const FileChangeEvent({
    required super.timestamp,
    required this.change,
    this.toolUseId,
  });

  @override
  String get type => 'file.change';

  @override
  Map<String, dynamic> toJson() => {
        'type': type,
        'timestamp': timestamp,
        'change': {
          'path': change.path,
          'action': change.action.wire,
          if (change.diff != null) 'diff': change.diff,
          if (change.addedLines != null) 'addedLines': change.addedLines,
          if (change.removedLines != null) 'removedLines': change.removedLines,
        },
        if (toolUseId != null) 'toolUseId': toolUseId,
      };
}

class CommandExecEvent extends UnifiedEvent {
  final String command;
  final String? cwd;
  final int? exitCode;
  final String? stdout;
  final String? stderr;
  final String? toolUseId;

  const CommandExecEvent({
    required super.timestamp,
    required this.command,
    this.cwd,
    this.exitCode,
    this.stdout,
    this.stderr,
    this.toolUseId,
  });

  @override
  String get type => 'command.exec';

  @override
  Map<String, dynamic> toJson() => {
        'type': type,
        'timestamp': timestamp,
        'command': command,
        if (cwd != null) 'cwd': cwd,
        if (exitCode != null) 'exitCode': exitCode,
        if (stdout != null) 'stdout': stdout,
        if (stderr != null) 'stderr': stderr,
        if (toolUseId != null) 'toolUseId': toolUseId,
      };
}

class TodoUpdateEvent extends UnifiedEvent {
  final List<TodoItem> todos;

  const TodoUpdateEvent({
    required super.timestamp,
    required this.todos,
  });

  @override
  String get type => 'todo.update';

  @override
  Map<String, dynamic> toJson() => {
        'type': type,
        'timestamp': timestamp,
        'todos': todos.map((t) {
          final m = <String, dynamic>{
            'id': t.id,
            'content': t.content,
            'status': t.status.wire,
          };
          if (t.progress != null) m['progress'] = t.progress;
          return m;
        }).toList(),
      };
}

class ThinkingEvent extends UnifiedEvent {
  final String text;

  const ThinkingEvent({
    required super.timestamp,
    required this.text,
  });

  @override
  String get type => 'thinking';

  @override
  Map<String, dynamic> toJson() => {
        'type': type,
        'timestamp': timestamp,
        'text': text,
      };
}

class ProgressEvent extends UnifiedEvent {
  final int? step;
  final int? total;
  final String? message;

  const ProgressEvent({
    required super.timestamp,
    this.step,
    this.total,
    this.message,
  });

  @override
  String get type => 'progress';

  @override
  Map<String, dynamic> toJson() => {
        'type': type,
        'timestamp': timestamp,
        if (step != null) 'step': step,
        if (total != null) 'total': total,
        if (message != null) 'message': message,
      };
}

class ErrorEvent extends UnifiedEvent {
  final String message;
  final bool? recoverable;
  final String? cli;

  const ErrorEvent({
    required super.timestamp,
    required this.message,
    this.recoverable,
    this.cli,
  });

  @override
  String get type => 'error';

  @override
  Map<String, dynamic> toJson() => {
        'type': type,
        'timestamp': timestamp,
        'message': message,
        if (recoverable != null) 'recoverable': recoverable,
        if (cli != null) 'cli': cli,
      };
}

class SessionEndEvent extends UnifiedEvent {
  final SessionStats? stats;

  const SessionEndEvent({
    required super.timestamp,
    this.stats,
  });

  @override
  String get type => 'session.end';

  @override
  Map<String, dynamic> toJson() => {
        'type': type,
        'timestamp': timestamp,
        if (stats != null) 'stats': stats,
      };
}

// ---- ClientCommand sealed ----

/// 客户端 → 服务端命令，sealed union。
sealed class ClientCommand {
  const ClientCommand();

  String get kind;

  Map<String, dynamic> toJson();
}

class SessionStartCommand extends ClientCommand {
  final String serverId;
  final String cwd;
  final CliKind cli;
  final String prompt;
  final String? model;
  final String? resumeCliSessionId;
  final List<String>? allowedTools;

  const SessionStartCommand({
    required this.serverId,
    required this.cwd,
    required this.cli,
    required this.prompt,
    this.model,
    this.resumeCliSessionId,
    this.allowedTools,
  });

  @override
  String get kind => 'session.start';

  @override
  Map<String, dynamic> toJson() => {
        'kind': kind,
        'serverId': serverId,
        'cwd': cwd,
        'cli': cli.wire,
        'prompt': prompt,
        if (model != null) 'model': model,
        if (resumeCliSessionId != null) 'resumeCliSessionId': resumeCliSessionId,
        if (allowedTools != null) 'allowedTools': allowedTools,
      };
}

class SessionSendCommand extends ClientCommand {
  final String prompt;

  const SessionSendCommand({required this.prompt});

  @override
  String get kind => 'session.send';

  @override
  Map<String, dynamic> toJson() => {'kind': kind, 'prompt': prompt};
}

class SessionInterruptCommand extends ClientCommand {
  const SessionInterruptCommand();

  @override
  String get kind => 'session.interrupt';

  @override
  Map<String, dynamic> toJson() => {'kind': kind};
}

class SessionEndCommand extends ClientCommand {
  const SessionEndCommand();

  @override
  String get kind => 'session.end';

  @override
  Map<String, dynamic> toJson() => {'kind': kind};
}

class SessionResyncCommand extends ClientCommand {
  final String sinceEventId;

  const SessionResyncCommand({required this.sinceEventId});

  @override
  String get kind => 'session.resync';

  @override
  Map<String, dynamic> toJson() => {'kind': kind, 'sinceEventId': sinceEventId};
}

// ---- ServerEnvelope ----

/// 服务端 → 客户端信封。
class ServerEnvelope {
  final int eventId;
  final String sessionId;
  final UnifiedEvent event;

  const ServerEnvelope({
    required this.eventId,
    required this.sessionId,
    required this.event,
  });

  factory ServerEnvelope.fromJson(Map<String, dynamic> j) {
    final eventJson = j['event'];
    return ServerEnvelope(
      eventId: j['eventId'] as int? ?? 0,
      sessionId: j['sessionId'] as String? ?? '',
      event: eventJson is Map<String, dynamic>
          ? UnifiedEvent.fromJson(eventJson)
          : ErrorEvent(
              timestamp: DateTime.now().millisecondsSinceEpoch,
              message: '信封缺少 event 字段',
            ),
    );
  }
}
