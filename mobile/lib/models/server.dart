/// Server 相关模型：SshAuth、SshAuthKind、ServerStatus、Server、FileListing、FileContent、ServerTestResult。
library;

import 'file_entry.dart';
import 'unified_event.dart';

// ---- SshAuthKind ----

enum SshAuthKind {
  password('password'),
  privateKey('privateKey'),
  agent('agent');

  final String wire;
  const SshAuthKind(this.wire);

  static SshAuthKind fromWire(String? v) =>
      values.firstWhere((e) => e.wire == v, orElse: () => SshAuthKind.password);
}

// ---- SshAuth sealed ----

sealed class SshAuth {
  const SshAuth();

  factory SshAuth.fromJson(Map<String, dynamic> j) {
    final kind = j['kind'] as String?;
    switch (kind) {
      case 'password':
        return PasswordAuth(password: j['password'] as String? ?? '');
      case 'privateKey':
        return PrivateKeyAuth(
          privateKey: j['privateKey'] as String? ?? '',
          passphrase: j['passphrase'] as String?,
        );
      case 'agent':
        return const AgentAuth();
      default:
        return PasswordAuth(password: '');
    }
  }

  Map<String, dynamic> toJson();
}

class PasswordAuth extends SshAuth {
  final String password;
  const PasswordAuth({required this.password});

  @override
  Map<String, dynamic> toJson() => {'kind': 'password', 'password': password};
}

class PrivateKeyAuth extends SshAuth {
  final String privateKey;
  final String? passphrase;
  const PrivateKeyAuth({required this.privateKey, this.passphrase});

  @override
  Map<String, dynamic> toJson() => {
        'kind': 'privateKey',
        'privateKey': privateKey,
        if (passphrase != null) 'passphrase': passphrase,
      };
}

class AgentAuth extends SshAuth {
  const AgentAuth();

  @override
  Map<String, dynamic> toJson() => {'kind': 'agent'};
}

// ---- ServerStatus ----

enum ServerStatus {
  ok('ok'),
  fail('fail'),
  unknown('unknown');

  final String wire;
  const ServerStatus(this.wire);

  static ServerStatus fromWire(String? v) =>
      values.firstWhere((e) => e.wire == v, orElse: () => ServerStatus.unknown);
}

// ---- Server ----

class Server {
  final String id;
  final String name;
  final String host;
  final int port;
  final String username;
  final SshAuthKind authKind;
  final DateTime createdAt;
  final ServerStatus lastStatus;
  final DateTime? lastCheckedAt;

  const Server({
    required this.id,
    required this.name,
    required this.host,
    required this.port,
    required this.username,
    required this.authKind,
    required this.createdAt,
    required this.lastStatus,
    this.lastCheckedAt,
  });

  factory Server.fromJson(Map<String, dynamic> j) => Server(
        id: j['id'] as String? ?? '',
        name: j['name'] as String? ?? '',
        host: j['host'] as String? ?? '',
        port: j['port'] as int? ?? 22,
        username: j['username'] as String? ?? '',
        authKind: SshAuthKind.fromWire(j['authKind'] as String?),
        createdAt: DateTime.tryParse(j['createdAt'] as String? ?? '') ?? DateTime.now(),
        lastStatus: ServerStatus.fromWire(j['lastStatus'] as String?),
        lastCheckedAt: j['lastCheckedAt'] != null
            ? DateTime.tryParse(j['lastCheckedAt'] as String)
            : null,
      );

  Map<String, dynamic> toJson() => {
        'id': id,
        'name': name,
        'host': host,
        'port': port,
        'username': username,
        'authKind': authKind.wire,
        'createdAt': createdAt.toIso8601String(),
        if (lastStatus != ServerStatus.unknown) 'lastStatus': lastStatus.wire,
        if (lastCheckedAt != null) 'lastCheckedAt': lastCheckedAt!.toIso8601String(),
      };
}

// ---- FileListing ----

class FileListing {
  final String path;
  final List<FileEntry> entries;

  const FileListing({required this.path, required this.entries});

  factory FileListing.fromJson(Map<String, dynamic> j) => FileListing(
        path: j['path'] as String? ?? '',
        entries: (j['entries'] as List?)
                ?.map((e) => FileEntry.fromJson(e as Map<String, dynamic>))
                .toList() ??
            const [],
      );
}

// ---- FileContent ----

class FileContent {
  final String path;
  final String content;
  final int size;

  const FileContent({required this.path, required this.content, required this.size});

  factory FileContent.fromJson(Map<String, dynamic> j) => FileContent(
        path: j['path'] as String? ?? '',
        content: j['content'] as String? ?? '',
        size: j['size'] as int? ?? 0,
      );
}

// ---- ServerTestResult ----

class ServerTestResult {
  final bool ok;
  final int? latencyMs;
  final String? error;

  const ServerTestResult({required this.ok, this.latencyMs, this.error});

  factory ServerTestResult.fromJson(Map<String, dynamic> j) => ServerTestResult(
        ok: j['ok'] as bool? ?? false,
        latencyMs: j['latencyMs'] as int?,
        error: j['error'] as String?,
      );
}


