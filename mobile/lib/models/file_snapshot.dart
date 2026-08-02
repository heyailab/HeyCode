/// FileSnapshot 与 RollbackResult 模型。
///
/// 两者都只有 fromJson（无 toJson、无 copyWith）。
library;

class FileSnapshot {
  final String id;
  final String sessionId;
  final String path;
  final String action; // 裸 String，不是枚举
  final String? diff;
  final int? addedLines;
  final int? removedLines;
  final DateTime createdAt;

  const FileSnapshot({
    required this.id,
    required this.sessionId,
    required this.path,
    required this.action,
    this.diff,
    this.addedLines,
    this.removedLines,
    required this.createdAt,
  });

  factory FileSnapshot.fromJson(Map<String, dynamic> j) => FileSnapshot(
        id: j['id'] as String? ?? '',
        sessionId: j['sessionId'] as String? ?? '',
        path: j['path'] as String? ?? '',
        action: j['action'] as String? ?? 'edit',
        diff: j['diff'] as String?,
        addedLines: j['addedLines'] as int?,
        removedLines: j['removedLines'] as int?,
        createdAt: DateTime.tryParse(j['createdAt'] as String? ?? '') ?? DateTime.now(),
      );
}

class RollbackResult {
  final String snapshotId;
  final String path;
  final String action;
  final bool rolled;
  final String method; // git-checkout / git-clean / skip
  final String message;

  const RollbackResult({
    required this.snapshotId,
    required this.path,
    required this.action,
    required this.rolled,
    required this.method,
    required this.message,
  });

  factory RollbackResult.fromJson(Map<String, dynamic> j) => RollbackResult(
        snapshotId: j['snapshotId'] as String? ?? '',
        path: j['path'] as String? ?? '',
        action: j['action'] as String? ?? 'edit',
        rolled: j['rolled'] as bool? ?? false,
        method: j['method'] as String? ?? 'skip',
        message: j['message'] as String? ?? '',
      );
}
