/// Session 模型与 SessionStatus 枚举。
library;

import 'unified_event.dart';

enum SessionStatus {
  running('running'),
  idle('idle'),
  ended('ended'),
  error('error');

  final String wire;
  const SessionStatus(this.wire);

  static SessionStatus fromWire(String? v) =>
      values.firstWhere((e) => e.wire == v, orElse: () => SessionStatus.idle);
}

class Session {
  final String id;
  final String taskId;
  final String? cliSessionId;
  final CliKind cli;
  final String? model;
  final SessionStatus status;
  final DateTime createdAt;
  final DateTime? endedAt;

  const Session({
    required this.id,
    required this.taskId,
    this.cliSessionId,
    required this.cli,
    this.model,
    required this.status,
    required this.createdAt,
    this.endedAt,
  });

  factory Session.fromJson(Map<String, dynamic> j) => Session(
        id: j['id'] as String? ?? '',
        taskId: j['taskId'] as String? ?? '',
        cliSessionId: j['cliSessionId'] as String?,
        cli: CliKind.fromWire(j['cli'] as String?),
        model: j['model'] as String?,
        status: SessionStatus.fromWire(j['status'] as String?),
        createdAt: DateTime.tryParse(j['createdAt'] as String? ?? '') ?? DateTime.now(),
        endedAt: j['endedAt'] != null ? DateTime.tryParse(j['endedAt'] as String) : null,
      );

  Map<String, dynamic> toJson() => {
        'id': id,
        'taskId': taskId,
        if (cliSessionId != null) 'cliSessionId': cliSessionId,
        'cli': cli.wire,
        if (model != null) 'model': model,
        'status': status.wire,
        'createdAt': createdAt.toIso8601String(),
        if (endedAt != null) 'endedAt': endedAt!.toIso8601String(),
      };
}
