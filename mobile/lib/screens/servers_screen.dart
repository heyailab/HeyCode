import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../models/server.dart';
import '../state/providers.dart';
import '../widgets/empty_state.dart';
import '../widgets/error_view.dart';
import '../widgets/loading_indicator.dart';

/// 服务器列表页。
class ServersScreen extends ConsumerWidget {
  const ServersScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final serversAsync = ref.watch(serversProvider(null));
    return Scaffold(
      appBar: AppBar(
        title: const Text('服务器'),
        actions: [
          IconButton(
            icon: const Icon(Icons.settings),
            tooltip: '设置',
            onPressed: () => context.push('/settings'),
          ),
        ],
      ),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: () => context.push('/servers/new'),
        icon: const Icon(Icons.add),
        label: const Text('新增服务器'),
      ),
      body: serversAsync.when(
        loading: () => const LoadingIndicator(),
        error: (e, _) => ErrorView(
          message: e.toString(),
          onRetry: () => ref.invalidate(serversProvider(null)),
        ),
        data: (servers) {
          if (servers.isEmpty) {
            return EmptyState(
              icon: Icons.dns_outlined,
              title: '还没有服务器',
              subtitle: '点击右下角添加你的第一台 SSH 服务器',
              action: FilledButton.icon(
                onPressed: () => context.push('/servers/new'),
                icon: const Icon(Icons.add),
                label: const Text('新增服务器'),
              ),
            );
          }
          return RefreshIndicator(
            onRefresh: () async => ref.invalidate(serversProvider(null)),
            child: ListView.builder(
              padding: const EdgeInsets.all(12),
              itemCount: servers.length,
              itemBuilder: (context, i) => _ServerCard(server: servers[i]),
            ),
          );
        },
      ),
    );
  }
}

/// 服务器卡片。
class _ServerCard extends ConsumerWidget {
  final Server server;

  const _ServerCard({required this.server});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final cs = Theme.of(context).colorScheme;
    final statusColor = _statusColor(server.lastStatus, cs);
    final statusLabel = _statusLabel(server.lastStatus);
    return Card(
      margin: const EdgeInsets.symmetric(vertical: 6),
      child: InkWell(
        borderRadius: BorderRadius.circular(12),
        onTap: () async {
          await ref.read(storageProvider).setLastServerId(server.id);
          if (context.mounted) {
            context.push('/servers/${server.id}/projects');
          }
        },
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
          child: Row(
            children: [
              CircleAvatar(
                backgroundColor: cs.primaryContainer,
                child: Icon(Icons.dns, color: cs.primary),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      server.name,
                      style: Theme.of(context).textTheme.titleMedium,
                    ),
                    const SizedBox(height: 2),
                    Text(
                      '${server.username}@${server.host}:${server.port}',
                      style: Theme.of(context).textTheme.bodySmall?.copyWith(
                            color: cs.outline,
                            fontFamily: 'monospace',
                          ),
                    ),
                    const SizedBox(height: 4),
                    Row(
                      children: [
                        Icon(Icons.circle, size: 8, color: statusColor),
                        const SizedBox(width: 6),
                        Text(statusLabel,
                            style: Theme.of(context).textTheme.labelSmall),
                        const SizedBox(width: 8),
                        Text(server.authKind.wire,
                            style: Theme.of(context).textTheme.labelSmall),
                      ],
                    ),
                  ],
                ),
              ),
              PopupMenuButton<String>(
                onSelected: (v) async {
                  switch (v) {
                    case 'edit':
                      context.push('/servers/${server.id}/edit');
                      break;
                    case 'files':
                      await ref
                          .read(storageProvider)
                          .setLastServerId(server.id);
                      if (context.mounted) {
                        context.push('/servers/${server.id}/files');
                      }
                      break;
                    case 'delete':
                      await _confirmDelete(context, ref);
                      break;
                  }
                },
                itemBuilder: (context) => const [
                  PopupMenuItem(value: 'edit', child: Text('编辑')),
                  PopupMenuItem(value: 'files', child: Text('文件浏览')),
                  PopupMenuItem(value: 'delete', child: Text('删除')),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }

  Future<void> _confirmDelete(BuildContext context, WidgetRef ref) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('删除服务器'),
        content: Text('确认删除「${server.name}」？关联的项目/任务可能受影响。'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: const Text('取消'),
          ),
          FilledButton.tonal(
            onPressed: () => Navigator.pop(ctx, true),
            child: const Text('删除'),
          ),
        ],
      ),
    );
    if (confirmed != true) return;
    try {
      final api = ref.read(apiClientProvider);
      await api.deleteServer(server.id);
      bumpDataVersion(ref);
      ref.invalidate(serversProvider(null));
      if (context.mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('已删除')),
        );
      }
    } catch (e) {
      if (context.mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('删除失败: $e')),
        );
      }
    }
  }

  Color _statusColor(ServerStatus s, ColorScheme cs) {
    switch (s) {
      case ServerStatus.ok:
        return const Color(0xFF2E7D32);
      case ServerStatus.fail:
        return const Color(0xFFC62828);
      case ServerStatus.unknown:
        return cs.outline;
    }
  }

  String _statusLabel(ServerStatus s) {
    switch (s) {
      case ServerStatus.ok:
        return '在线';
      case ServerStatus.fail:
        return '不可达';
      case ServerStatus.unknown:
        return '未知';
    }
  }
}
