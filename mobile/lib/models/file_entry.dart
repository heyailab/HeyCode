/// FileEntry 模型（SFTP 目录条目）。
library;

class FileEntry {
  final String name;
  final String path;
  final bool isDir;
  final int size;
  final DateTime modifiedAt;

  const FileEntry({
    required this.name,
    required this.path,
    required this.isDir,
    required this.size,
    required this.modifiedAt,
  });

  factory FileEntry.fromJson(Map<String, dynamic> j) => FileEntry(
        name: j['name'] as String? ?? '',
        path: j['path'] as String? ?? '',
        isDir: j['isDir'] as bool? ?? false,
        size: j['size'] as int? ?? 0,
        modifiedAt: DateTime.tryParse(j['modifiedAt'] as String? ?? '') ?? DateTime.now(),
      );

  Map<String, dynamic> toJson() => {
        'name': name,
        'path': path,
        'isDir': isDir,
        'size': size,
        'modifiedAt': modifiedAt.toIso8601String(),
      };
}
