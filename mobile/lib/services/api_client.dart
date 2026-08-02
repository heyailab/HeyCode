/// ApiClient：Dio REST 客户端，封装所有后端 REST 端点。
///
/// 错误处理：DioException → ApiException（Never 返回，可直接声明具体返回类型）。
/// 响应解析：_asMap / _asList 辅助函数。
library;

import 'package:dio/dio.dart';

import '../config.dart';
import '../models/file_entry.dart';
import '../models/file_snapshot.dart';
import '../models/project.dart';
import '../models/server.dart';
import '../models/session.dart';
import '../models/task.dart';
import '../models/unified_event.dart';

// ---- ApiException ----

class ApiException implements Exception {
  final int? statusCode;
  final String message;
  ApiException(this.message, {this.statusCode});

  @override
  String toString() => 'ApiException($statusCode): $message';
}

Never _err(DioException e) {
  String msg = e.message ?? '网络错误';
  if (e.response != null) {
    final data = e.response?.data;
    if (data is Map<String, dynamic> && data['error'] != null) {
      msg = data['error'].toString();
    } else if (data is String && data.isNotEmpty) {
      msg = data;
    }
  }
  throw ApiException(msg, statusCode: e.response?.statusCode);
}

Map<String, dynamic> _asMap(dynamic data) {
  if (data is! Map<String, dynamic>) {
    throw ApiException('响应格式错误：期望对象，得到 ${data.runtimeType}');
  }
  return data;
}

List<Map<String, dynamic>> _asList(dynamic data) {
  if (data is! List) {
    throw ApiException('响应格式错误：期望数组');
  }
  return data.cast<Map<String, dynamic>>();
}

// ---- ApiKeyMeta ----

class ApiKeyMeta {
  final CliKind cli;
  final bool hasKey;
  final String? last4;
  final DateTime? updatedAt;

  const ApiKeyMeta({
    required this.cli,
    required this.hasKey,
    this.last4,
    this.updatedAt,
  });

  factory ApiKeyMeta.fromJson(Map<String, dynamic> j) => ApiKeyMeta(
        cli: CliKind.fromWire(j['cli'] as String?),
        hasKey: j['hasKey'] as bool? ?? false,
        last4: j['last4'] as String?,
        updatedAt: j['updatedAt'] != null
            ? DateTime.tryParse(j['updatedAt'] as String)
            : null,
      );
}

// ---- ApiClient ----

class ApiClient {
  late final Dio _dio;

  ApiClient(AppConfig config) {
    _dio = Dio(BaseOptions(
      baseUrl: config.baseUrl,
      connectTimeout: const Duration(seconds: 10),
      receiveTimeout: const Duration(seconds: 30),
      headers: {'Accept': 'application/json'},
    ));
    _dio.interceptors.add(LogInterceptor(
      requestBody: false,
      responseBody: false,
      error: true,
    ));
  }

  Dio get dio => _dio;

  // ---- 健康检查 ----

  Future<Map<String, dynamic>> health() async {
    try {
      final res = await _dio.get('/api/health');
      return _asMap(res.data);
    } on DioException catch (e) {
      _err(e);
    }
  }

  // ---- 服务器 ----

  Future<List<Server>> listServers({String? projectId}) async {
    try {
      final res = await _dio.get('/api/servers',
          queryParameters: projectId != null ? {'projectId': projectId} : null);
      final list = _asList(res.data);
      return list.map(Server.fromJson).toList();
    } on DioException catch (e) {
      _err(e);
    }
  }

  Future<Server> createServer({
    required String name,
    required String host,
    required int port,
    required String username,
    required SshAuth auth,
  }) async {
    try {
      final res = await _dio.post('/api/servers', data: {
        'name': name,
        'host': host,
        'port': port,
        'username': username,
        'auth': auth.toJson(),
      });
      return Server.fromJson(_asMap(res.data));
    } on DioException catch (e) {
      _err(e);
    }
  }

  Future<Server> getServer(String id) async {
    try {
      final res = await _dio.get('/api/servers/$id');
      return Server.fromJson(_asMap(res.data));
    } on DioException catch (e) {
      _err(e);
    }
  }

  Future<Server> updateServer(
    String id, {
    String? name,
    String? host,
    int? port,
    String? username,
    SshAuth? auth,
  }) async {
    try {
      final body = <String, dynamic>{};
      if (name != null) body['name'] = name;
      if (host != null) body['host'] = host;
      if (port != null) body['port'] = port;
      if (username != null) body['username'] = username;
      if (auth != null) body['auth'] = auth.toJson();
      final res = await _dio.patch('/api/servers/$id', data: body);
      return Server.fromJson(_asMap(res.data));
    } on DioException catch (e) {
      _err(e);
    }
  }

  Future<void> deleteServer(String id) async {
    try {
      await _dio.delete('/api/servers/$id');
    } on DioException catch (e) {
      _err(e);
    }
  }

  Future<ServerTestResult> testServer(String id) async {
    try {
      final res = await _dio.post('/api/servers/$id/test');
      return ServerTestResult.fromJson(_asMap(res.data));
    } on DioException catch (e) {
      _err(e);
    }
  }

  // ---- SFTP 文件 ----

  Future<FileListing> listFiles(String serverId, String path) async {
    try {
      final res = await _dio.get('/api/servers/$serverId/files',
          queryParameters: {'path': path});
      return FileListing.fromJson(_asMap(res.data));
    } on DioException catch (e) {
      _err(e);
    }
  }

  Future<FileContent> readFile(String serverId, String path) async {
    try {
      final res = await _dio.get('/api/servers/$serverId/files/content',
          queryParameters: {'path': path});
      return FileContent.fromJson(_asMap(res.data));
    } on DioException catch (e) {
      _err(e);
    }
  }

  Future<FileContent> writeFile(String serverId, String path, String content) async {
    try {
      final res = await _dio.put('/api/servers/$serverId/files/content',
          data: {'path': path, 'content': content});
      return FileContent.fromJson(_asMap(res.data));
    } on DioException catch (e) {
      _err(e);
    }
  }

  Future<void> deleteFile(String serverId, String path) async {
    try {
      await _dio.delete('/api/servers/$serverId/files',
          queryParameters: {'path': path});
    } on DioException catch (e) {
      _err(e);
    }
  }

  // ---- 项目 ----

  Future<List<Project>> listProjects({String? serverId}) async {
    try {
      final res = await _dio.get('/api/projects',
          queryParameters: serverId != null ? {'serverId': serverId} : null);
      final list = _asList(res.data);
      return list.map(Project.fromJson).toList();
    } on DioException catch (e) {
      _err(e);
    }
  }

  Future<Project> createProject({
    required String serverId,
    required String name,
    required String cwd,
    required CliKind defaultCli,
    String? defaultModel,
    String? rules,
  }) async {
    try {
      final res = await _dio.post('/api/projects', data: {
        'serverId': serverId,
        'name': name,
        'cwd': cwd,
        'defaultCli': defaultCli.wire,
        if (defaultModel != null) 'defaultModel': defaultModel,
        if (rules != null) 'rules': rules,
      });
      return Project.fromJson(_asMap(res.data));
    } on DioException catch (e) {
      _err(e);
    }
  }

  Future<Project> getProject(String id) async {
    try {
      final res = await _dio.get('/api/projects/$id');
      return Project.fromJson(_asMap(res.data));
    } on DioException catch (e) {
      _err(e);
    }
  }

  Future<Project> updateProject(String id, Map<String, dynamic> body) async {
    try {
      final res = await _dio.patch('/api/projects/$id', data: body);
      return Project.fromJson(_asMap(res.data));
    } on DioException catch (e) {
      _err(e);
    }
  }

  Future<void> deleteProject(String id) async {
    try {
      await _dio.delete('/api/projects/$id');
    } on DioException catch (e) {
      _err(e);
    }
  }

  // ---- 任务 ----

  Future<List<Task>> listTasks(String projectId) async {
    try {
      final res = await _dio.get('/api/projects/$projectId/tasks');
      final list = _asList(res.data);
      return list.map(Task.fromJson).toList();
    } on DioException catch (e) {
      _err(e);
    }
  }

  Future<Task> createTask({
    required String projectId,
    required String title,
    String? description,
  }) async {
    try {
      final res = await _dio.post('/api/tasks', data: {
        'projectId': projectId,
        'title': title,
        if (description != null) 'description': description,
      });
      return Task.fromJson(_asMap(res.data));
    } on DioException catch (e) {
      _err(e);
    }
  }

  Future<Task> getTask(String id) async {
    try {
      final res = await _dio.get('/api/tasks/$id');
      return Task.fromJson(_asMap(res.data));
    } on DioException catch (e) {
      _err(e);
    }
  }

  Future<Task> updateTask(String id, Map<String, dynamic> body) async {
    try {
      final res = await _dio.patch('/api/tasks/$id', data: body);
      return Task.fromJson(_asMap(res.data));
    } on DioException catch (e) {
      _err(e);
    }
  }

  Future<void> deleteTask(String id) async {
    try {
      await _dio.delete('/api/tasks/$id');
    } on DioException catch (e) {
      _err(e);
    }
  }

  // ---- 会话 ----

  Future<List<Session>> listSessions(String taskId) async {
    try {
      final res = await _dio.get('/api/tasks/$taskId/sessions');
      final list = _asList(res.data);
      return list.map(Session.fromJson).toList();
    } on DioException catch (e) {
      _err(e);
    }
  }

  Future<Session> createSession({
    required String taskId,
    required CliKind cli,
    String? model,
  }) async {
    try {
      final res = await _dio.post('/api/sessions', data: {
        'taskId': taskId,
        'cli': cli.wire,
        if (model != null) 'model': model,
      });
      return Session.fromJson(_asMap(res.data));
    } on DioException catch (e) {
      _err(e);
    }
  }

  Future<Session> getSession(String id) async {
    try {
      final res = await _dio.get('/api/sessions/$id');
      return Session.fromJson(_asMap(res.data));
    } on DioException catch (e) {
      _err(e);
    }
  }

  Future<List<ServerEnvelope>> listSessionEvents(String id, {int? since}) async {
    try {
      final res = await _dio.get('/api/sessions/$id/events',
          queryParameters: since != null ? {'since': since} : null);
      final data = res.data;
      if (data is! Map<String, dynamic>) {
        throw ApiException('响应格式错误：期望对象');
      }
      final events = data['events'];
      if (events is! List) {
        throw ApiException('响应缺少 events 数组');
      }
      return events
          .map((e) => ServerEnvelope.fromJson(e as Map<String, dynamic>))
          .toList();
    } on DioException catch (e) {
      _err(e);
    }
  }

  Future<void> deleteSession(String id) async {
    try {
      await _dio.delete('/api/sessions/$id');
    } on DioException catch (e) {
      _err(e);
    }
  }

  // ---- 快照 ----

  Future<List<FileSnapshot>> listSnapshots(String sessionId) async {
    try {
      final res = await _dio.get('/api/sessions/$sessionId/snapshots');
      final data = res.data;
      if (data is! Map<String, dynamic>) {
        throw ApiException('响应格式错误：期望对象');
      }
      final snapshots = data['snapshots'];
      if (snapshots is! List) {
        throw ApiException('响应缺少 snapshots 数组');
      }
      return snapshots
          .map((s) => FileSnapshot.fromJson(s as Map<String, dynamic>))
          .toList();
    } on DioException catch (e) {
      _err(e);
    }
  }

  Future<RollbackResult> rollbackSnapshot(
    String snapshotId, {
    String? serverId,
    String? cwd,
  }) async {
    try {
      final res = await _dio.post('/api/snapshots/$snapshotId/rollback', data: {
        if (serverId != null) 'serverId': serverId,
        if (cwd != null) 'cwd': cwd,
      });
      final data = _asMap(res.data);
      return RollbackResult.fromJson(
          data['result'] as Map<String, dynamic>? ?? const {});
    } on DioException catch (e) {
      _err(e);
    }
  }

  Future<List<RollbackResult>> rollbackSession(
    String sessionId, {
    String? serverId,
    String? cwd,
  }) async {
    try {
      final res = await _dio.post('/api/sessions/$sessionId/rollback', data: {
        if (serverId != null) 'serverId': serverId,
        if (cwd != null) 'cwd': cwd,
      });
      final data = _asMap(res.data);
      final results = data['results'];
      if (results is! List) {
        throw ApiException('响应缺少 results 数组');
      }
      return results
          .map((r) => RollbackResult.fromJson(r as Map<String, dynamic>))
          .toList();
    } on DioException catch (e) {
      _err(e);
    }
  }

  // ---- API Key ----

  Future<List<ApiKeyMeta>> listApiKeys() async {
    try {
      final res = await _dio.get('/api/api-keys');
      final list = _asList(res.data);
      return list.map(ApiKeyMeta.fromJson).toList();
    } on DioException catch (e) {
      _err(e);
    }
  }

  Future<ApiKeyMeta> upsertApiKey({
    required CliKind cli,
    required String key,
  }) async {
    try {
      final res = await _dio.post('/api/api-keys', data: {
        'cli': cli.wire,
        'key': key,
      });
      return ApiKeyMeta.fromJson(_asMap(res.data));
    } on DioException catch (e) {
      _err(e);
    }
  }

  Future<void> deleteApiKey(CliKind cli) async {
    try {
      await _dio.delete('/api/api-keys/${cli.wire}');
    } on DioException catch (e) {
      _err(e);
    }
  }
}
