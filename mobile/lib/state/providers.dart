/// 全部 Riverpod providers。
///
/// storageProvider 需在 main() 中用 SharedPreferences 实例 override。
/// apiClientProvider / wsClientProvider 通过 ref.watch(configProvider)
/// 在配置变更时自动重建。
library;

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../config.dart';
import '../models/file_snapshot.dart';
import '../models/project.dart';
import '../models/server.dart';
import '../models/session.dart';
import '../models/task.dart';
import '../models/unified_event.dart';
import '../services/api_client.dart';
import '../services/storage.dart';
import '../services/ws_client.dart';
import 'session_controller.dart';

// ---- 基础设施 ----

/// SharedPreferences 包装。需在 main 中 override。
final storageProvider = Provider<Storage>((ref) {
  throw UnimplementedError('storageProvider 必须在 main 中 override');
});

/// 当前后端配置。
final configProvider = StateProvider<AppConfig>((ref) {
  return AppConfig.fromBaseUrl(AppConfig.defaultBaseUrl);
});

/// 启动时从存储读取 baseUrl 写入 configProvider。
final initConfigProvider = FutureProvider<void>((ref) async {
  final storage = ref.read(storageProvider);
  final url = await storage.getBaseUrl();
  final cfg = AppConfig.fromBaseUrl(url.isEmpty ? AppConfig.defaultBaseUrl : url);
  ref.read(configProvider.notifier).state = cfg;
});

/// Dio REST 客户端单例（config 变更自动重建）。
final apiClientProvider = Provider<ApiClient>((ref) {
  final config = ref.watch(configProvider);
  final client = ApiClient(config);
  ref.onDispose(client.dio.close);
  return client;
});

/// WebSocket 客户端单例（config 变更自动重建）。
final wsClientProvider = Provider<WsClient>((ref) {
  final config = ref.watch(configProvider);
  final client = WsClient();
  client.setWsUrl(config.wsUrl);
  ref.onDispose(() => client.close());
  return client;
});

// ---- 列表查询 ----

/// 服务器列表（参数 projectId，null=全部）。
final serversProvider =
    FutureProvider.family<List<Server>, String?>((ref, projectId) async {
  final api = ref.watch(apiClientProvider);
  return api.listServers(projectId: projectId);
});

/// 项目列表（参数 serverId，null=全部）。
final projectsProvider =
    FutureProvider.family<List<Project>, String?>((ref, serverId) async {
  final api = ref.watch(apiClientProvider);
  return api.listProjects(serverId: serverId);
});

/// 某项目下任务列表。
final tasksProvider =
    FutureProvider.family<List<Task>, String>((ref, projectId) async {
  final api = ref.watch(apiClientProvider);
  return api.listTasks(projectId);
});

/// 单个服务器。
final serverProvider =
    FutureProvider.family<Server, String>((ref, id) async {
  final api = ref.watch(apiClientProvider);
  return api.getServer(id);
});

/// 单个项目。
final projectProvider =
    FutureProvider.family<Project, String>((ref, id) async {
  final api = ref.watch(apiClientProvider);
  return api.getProject(id);
});

/// 单个任务。
final taskProvider =
    FutureProvider.family<Task, String>((ref, id) async {
  final api = ref.watch(apiClientProvider);
  return api.getTask(id);
});

/// 某会话的快照列表。
final snapshotsProvider =
    FutureProvider.family<List<FileSnapshot>, String>((ref, sessionId) async {
  final api = ref.watch(apiClientProvider);
  return api.listSnapshots(sessionId);
});

/// API Key 列表。
final apiKeysProvider = FutureProvider<List<ApiKeyMeta>>((ref) async {
  final api = ref.watch(apiClientProvider);
  return api.listApiKeys();
});

// ---- 数据版本刷新机制 ----

/// 配置变更后刷新列表的机制。写操作后 bumpDataVersion 触发列表 invalidate。
final dataVersionProvider = StateProvider<int>((ref) => 0);

void bumpDataVersion(WidgetRef ref) {
  ref.read(dataVersionProvider.notifier).state++;
}

// ---- 会话控制器 ----

/// 会话状态控制器（autoDispose：离开会话页自动销毁，断开 WS）。
final sessionControllerProvider =
    StateNotifierProvider.autoDispose<SessionController, SessionControllerState>(
        (ref) {
  final api = ref.watch(apiClientProvider);
  final ws = ref.watch(wsClientProvider);
  return SessionController(api: api, ws: ws);
});
