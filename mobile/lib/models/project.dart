/// Project 模型。
library;

import 'unified_event.dart';

class Project {
  final String id;
  final String serverId;
  final String name;
  final String cwd;
  final CliKind defaultCli;
  final String? defaultModel;
  final String? rules;
  final DateTime createdAt;

  const Project({
    required this.id,
    required this.serverId,
    required this.name,
    required this.cwd,
    required this.defaultCli,
    this.defaultModel,
    this.rules,
    required this.createdAt,
  });

  factory Project.fromJson(Map<String, dynamic> j) => Project(
        id: j['id'] as String? ?? '',
        serverId: j['serverId'] as String? ?? '',
        name: j['name'] as String? ?? '',
        cwd: j['cwd'] as String? ?? '',
        defaultCli: CliKind.fromWire(j['defaultCli'] as String?),
        defaultModel: j['defaultModel'] as String?,
        rules: j['rules'] as String?,
        createdAt: DateTime.tryParse(j['createdAt'] as String? ?? '') ?? DateTime.now(),
      );

  Map<String, dynamic> toJson() => {
        'id': id,
        'serverId': serverId,
        'name': name,
        'cwd': cwd,
        'defaultCli': defaultCli.wire,
        if (defaultModel != null) 'defaultModel': defaultModel,
        if (rules != null) 'rules': rules,
        'createdAt': createdAt.toIso8601String(),
      };
}
