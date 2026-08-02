import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:intl/intl.dart';

import '../models/project.dart';
import '../models/session.dart';
import '../models/unified_event.dart';
import '../state/providers.dart';
import '../widgets/empty_state.dart';
import '../widgets/error_view.dart';
import '../widgets/loading_indicator.dart';

/// 某任务下的会话列表 provider（本地定义，providers.dart 中无 sessionsProvider）。
final sessionsProvider =
    FutureProvider.family<List<Session>, String>((ref, taskId) async {
  final api = ref.watch(apiClientProvider);
  return api.listSessions(taskId);
});

/// 会话列表页：展示某任务下的所有会话，可发起新会话。
class SessionListScreen extends ConsumerWidget {
  final String taskId;

  const SessionListScreen({super.key, required this.taskId});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final sessionsAsync = ref.watch(sessionsProvider(taskId));
    return Scaffold(
      appBar: AppBar(title: const Text('会话')),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: () => _startNewSession(context, ref),
        icon: const Icon(Icons.play_arrow),
        label: const Text('新会话'),
      ),
      body: sessionsAsync.when(
        loading: () => const LoadingIndicator(),
        error: (e, _) => ErrorView(
          message: e.toString(),
          onRetry: () => ref.invalidate(sessionsProvider(taskId)),
        ),
        data: (sessions) {
          if (sessions.isEmpty) {
            return const EmptyState(
              icon: Icons.chat_bubble_outline,
              title: '还没有会话',
              subtitle: '点击右下角开始一个新会话',
            );
          }
          return ListView.builder(
            padding: const EdgeInsets.all(8),
            itemCount: sessions.length,
            itemBuilder: (context, i) {
              final session = sessions[i];
              return _SessionCard(
                session: session,
                onTap: () => context.push('/sessions/${session.id}'),
              );
            },
          );
        },
      ),
    );
  }

  /// 发起新会话：查 task → 查 project → createSession → 跳转会话页（新建模式）。
  Future<void> _startNewSession(BuildContext context, WidgetRef ref) async {
    try {
      final task = await ref.read(taskProvider(taskId).future);
      Project? project;
      try {
        project = await ref.read(projectProvider(task.projectId).future);
      } catch (_) {
        // 获取 project 失败，用 pty 作为默认 cli
      }
      final cli = project?.defaultCli ?? CliKind.pty;
      final cwd = project?.cwd ?? '';
      final model = project?.defaultModel;
      final serverId = project?.serverId ?? '';

      final api = ref.read(apiClientProvider);
      final session = await api.createSession(
        taskId: taskId,
        cli: cli,
        model: model,
      );
      ref.invalidate(sessionsProvider(taskId));

      if (!context.mounted) return;
      final params = <String, String>{
        'taskId': taskId,
        'cli': cli.wire,
        'prompt': '',
      };
      if (cwd.isNotEmpty) params['cwd'] = cwd;
      if (model != null && model.isNotEmpty) params['model'] = model;
      if (serverId.isNotEmpty) params['serverId'] = serverId;
      final uri =
          Uri(path: '/sessions/${session.id}', queryParameters: params);
      context.push(uri.toString());
    } catch (e) {
      if (!context.mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('创建会话失败：$e')),
      );
    }
  }
}

/// 会话状态徽章（状态点 + 文案）。
class _SessionStatusBadge extends StatelessWidget {
  final SessionStatus status;
  const _SessionStatusBadge({required this.status});

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final color = _color(theme, status);
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Icon(Icons.circle, size: 8, color: color),
        const SizedBox(width: 4),
        Text(_label(status), style: TextStyle(color: color, fontSize: 12)),
      ],
    );
  }

  static Color _color(ThemeData theme, SessionStatus s) {
    switch (s) {
      case SessionStatus.running:
        return theme.colorScheme.primary;
      case SessionStatus.idle:
        return theme.colorScheme.tertiary;
      case SessionStatus.ended:
        return theme.colorScheme.outline;
      case SessionStatus.error:
        return theme.colorScheme.error;
    }
  }

  static String _label(SessionStatus s) {
    switch (s) {
      case SessionStatus.running:
        return '运行中';
      case SessionStatus.idle:
        return '空闲';
      case SessionStatus.ended:
        return '已结束';
      case SessionStatus.error:
        return '出错';
    }
  }
}

/// 会话卡片：展示 cli、创建时间、状态；点击恢复会话；PopupMenu 删除。
class _SessionCard extends ConsumerWidget {
  final Session session;
  final VoidCallback onTap;

  const _SessionCard({required this.session, required this.onTap});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Card(
      margin: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      child: ListTile(
        title: Text(session.cli.wire),
        subtitle: Text(
          DateFormat('yyyy-MM-dd HH:mm').format(session.createdAt.toLocal()),
        ),
        trailing: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            _SessionStatusBadge(status: session.status),
            PopupMenuButton<String>(
              onSelected: (v) => _onMenuSelected(context, ref, v),
              itemBuilder: (context) => const [
                PopupMenuItem(value: 'delete', child: Text('删除')),
              ],
            ),
          ],
        ),
        onTap: onTap,
      ),
    );
  }

  Future<void> _onMenuSelected(
      BuildContext context, WidgetRef ref, String value) async {
    if (value != 'delete') return;
    final ok = await _confirmDelete(context);
    if (ok != true) return;
    try {
      await ref.read(apiClientProvider).deleteSession(session.id);
      ref.invalidate(sessionsProvider(session.taskId));
    } catch (e) {
      if (context.mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('删除失败：$e')),
        );
      }
    }
  }

  Future<bool?> _confirmDelete(BuildContext context) {
    return showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('删除会话'),
        content: const Text('确定要删除这个会话吗？此操作不可撤销。'),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.of(context).pop(true),
            child: const Text('删除'),
          ),
        ],
      ),
    );
  }
}
