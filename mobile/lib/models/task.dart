/// Task 模型与 TaskStatus 枚举。
library;

enum TaskStatus {
  planned('planned'),
  inProgress('in_progress'),
  done('done'),
  archived('archived');

  final String wire;
  const TaskStatus(this.wire);

  static TaskStatus fromWire(String? v) =>
      values.firstWhere((e) => e.wire == v, orElse: () => TaskStatus.planned);
}

class Task {
  final String id;
  final String projectId;
  final String title;
  final String? description;
  final TaskStatus status;
  final DateTime createdAt;
  final DateTime updatedAt;

  const Task({
    required this.id,
    required this.projectId,
    required this.title,
    this.description,
    required this.status,
    required this.createdAt,
    required this.updatedAt,
  });

  factory Task.fromJson(Map<String, dynamic> j) => Task(
        id: j['id'] as String? ?? '',
        projectId: j['projectId'] as String? ?? '',
        title: j['title'] as String? ?? '',
        description: j['description'] as String?,
        status: TaskStatus.fromWire(j['status'] as String?),
        createdAt: DateTime.tryParse(j['createdAt'] as String? ?? '') ?? DateTime.now(),
        updatedAt: DateTime.tryParse(j['updatedAt'] as String? ?? '') ?? DateTime.now(),
      );

  Map<String, dynamic> toJson() => {
        'id': id,
        'projectId': projectId,
        'title': title,
        if (description != null) 'description': description,
        'status': status.wire,
        'createdAt': createdAt.toIso8601String(),
        'updatedAt': updatedAt.toIso8601String(),
      };
}
