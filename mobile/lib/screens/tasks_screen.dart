import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../models/task.dart';
import '../state/providers.dart';
import '../widgets/empty_state.dart';
import '../widgets/error_view.dart';
import '../widgets/loading_indicator.dart';

/// 任务列表页：展示某项目下的所有任务。
class TasksScreen extends ConsumerWidget {
  final String projectId;

  const TasksScreen({super.key, required this.projectId});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final tasksAsync = ref.watch(tasksProvider(projectId));
    return Scaffold(
      appBar: AppBar(title: const Text('任务')),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: () => context.push('/projects/$projectId/tasks/new'),
        icon: const Icon(Icons.add),
        label: const Text('新建任务'),
      ),
      body: tasksAsync.when(
        loading: () => const LoadingIndicator(),
        error: (e, _) => ErrorView(
          message: e.toString(),
          onRetry: () => ref.invalidate(tasksProvider(projectId)),
        ),
        data: (tasks) {
          if (tasks.isEmpty) {
            return EmptyState(
              icon: Icons.task_alt,
              title: '还没有任务',
              subtitle: '为这个项目创建一个任务',
              action: FilledButton.icon(
                onPressed: () =>
                    context.push('/projects/$projectId/tasks/new'),
                icon: const Icon(Icons.add),
                label: const Text('新建任务'),
              ),
            );
          }
          return ListView.builder(
            padding: const EdgeInsets.all(8),
            itemCount: tasks.length,
            itemBuilder: (context, i) {
              final task = tasks[i];
              return _TaskCard(
                task: task,
                onTap: () => context.push('/tasks/${task.id}/sessions'),
              );
            },
          );
        },
      ),
    );
  }
}

/// 任务状态徽章（状态点 + 文案）。
class _TaskStatusBadge extends StatelessWidget {
  final TaskStatus status;
  const _TaskStatusBadge({required this.status});

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

  static Color _color(ThemeData theme, TaskStatus s) {
    switch (s) {
      case TaskStatus.planned:
        return theme.colorScheme.outline;
      case TaskStatus.inProgress:
        return theme.colorScheme.tertiary;
      case TaskStatus.done:
        return const Color(0xFF2E7D32);
      case TaskStatus.archived:
        return theme.colorScheme.outline;
    }
  }

  static String _label(TaskStatus s) {
    switch (s) {
      case TaskStatus.planned:
        return '已规划';
      case TaskStatus.inProgress:
        return '进行中';
      case TaskStatus.done:
        return '已完成';
      case TaskStatus.archived:
        return '已归档';
    }
  }
}

/// 任务卡片：展示标题、描述、状态；点击进入会话列表；PopupMenu 删除。
class _TaskCard extends ConsumerWidget {
  final Task task;
  final VoidCallback onTap;

  const _TaskCard({required this.task, required this.onTap});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Card(
      margin: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      child: ListTile(
        title: Text(task.title),
        subtitle: (task.description != null && task.description!.isNotEmpty)
            ? Text(
                task.description!,
                maxLines: 2,
                overflow: TextOverflow.ellipsis,
              )
            : null,
        trailing: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            _TaskStatusBadge(status: task.status),
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
      await ref.read(apiClientProvider).deleteTask(task.id);
      bumpDataVersion(ref);
      ref.invalidate(tasksProvider(task.projectId));
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
        title: const Text('删除任务'),
        content: const Text('确定要删除这个任务吗？此操作不可撤销。'),
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