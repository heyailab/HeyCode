import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../models/project.dart';
import '../state/providers.dart';
import '../widgets/empty_state.dart';
import '../widgets/error_view.dart';
import '../widgets/loading_indicator.dart';

/// 项目列表页。
class ProjectsScreen extends ConsumerWidget {
  final String serverId;

  const ProjectsScreen({super.key, required this.serverId});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final projectsAsync = ref.watch(projectsProvider(serverId));
    return Scaffold(
      appBar: AppBar(
        title: const Text('项目'),
        actions: [
          IconButton(
            icon: const Icon(Icons.folder_open),
            tooltip: '文件浏览',
            onPressed: () => context.push('/servers/$serverId/files'),
          ),
        ],
      ),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: () => context.push('/servers/$serverId/projects/new'),
        icon: const Icon(Icons.add),
        label: const Text('新增项目'),
      ),
      body: projectsAsync.when(
        loading: () => const LoadingIndicator(),
        error: (e, _) => ErrorView(
          message: e.toString(),
          onRetry: () => ref.invalidate(projectsProvider(serverId)),
        ),
        data: (projects) {
          if (projects.isEmpty) {
            return const EmptyState(
              icon: Icons.folder_off_outlined,
              title: '还没有项目',
              subtitle: '为这台服务器添加一个工作目录',
            );
          }
          return RefreshIndicator(
            onRefresh: () async => ref.invalidate(projectsProvider(serverId)),
            child: ListView.builder(
              padding: const EdgeInsets.all(12),
              itemCount: projects.length,
              itemBuilder: (context, i) =>
                  _ProjectCard(serverId: serverId, project: projects[i]),
            ),
          );
        },
      ),
    );
  }
}

/// 项目卡片。
class _ProjectCard extends ConsumerWidget {
  final String serverId;
  final Project project;

  const _ProjectCard({required this.serverId, required this.project});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final cs = Theme.of(context).colorScheme;
    return Card(
      margin: const EdgeInsets.symmetric(vertical: 6),
      child: InkWell(
        borderRadius: BorderRadius.circular(12),
        onTap: () async {
          await ref.read(storageProvider).setLastProjectId(project.id);
          if (context.mounted) {
            context.push('/projects/${project.id}/tasks');
          }
        },
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
          child: Row(
            children: [
              CircleAvatar(
                backgroundColor: cs.secondaryContainer,
                child: Icon(Icons.folder, color: cs.onSecondaryContainer),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      project.name,
                      style: Theme.of(context).textTheme.titleMedium,
                    ),
                    const SizedBox(height: 2),
                    Text(
                      project.cwd,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: Theme.of(context).textTheme.bodySmall?.copyWith(
                            color: cs.outline,
                            fontFamily: 'monospace',
                          ),
                    ),
                    const SizedBox(height: 6),
                    Wrap(
                      spacing: 6,
                      runSpacing: 4,
                      children: [
                        _chip(context, project.defaultCli.wire, cs.primary),
                        if (project.defaultModel != null &&
                            project.defaultModel!.isNotEmpty)
                          _chip(
                              context, project.defaultModel!, cs.tertiary),
                      ],
                    ),
                  ],
                ),
              ),
              IconButton(
                icon: const Icon(Icons.chevron_right),
                onPressed: () async {
                  await ref
                      .read(storageProvider)
                      .setLastProjectId(project.id);
                  if (context.mounted) {
                    context.push('/projects/${project.id}/tasks');
                  }
                },
              ),
              PopupMenuButton<String>(
                onSelected: (v) async {
                  switch (v) {
                    case 'edit':
                      if (context.mounted) {
                        ScaffoldMessenger.of(context).showSnackBar(
                          const SnackBar(content: Text('待实现')),
                        );
                      }
                      break;
                    case 'delete':
                      await _confirmDelete(context, ref);
                      break;
                  }
                },
                itemBuilder: (context) => const [
                  PopupMenuItem(value: 'edit', child: Text('编辑')),
                  PopupMenuItem(value: 'delete', child: Text('删除')),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _chip(BuildContext context, String text, Color color) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(
        color: color.withOpacity(0.12),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Text(
        text,
        style: TextStyle(
          fontSize: 11,
          fontWeight: FontWeight.w600,
          color: color,
        ),
      ),
    );
  }

  Future<void> _confirmDelete(BuildContext context, WidgetRef ref) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('删除项目'),
        content: Text('确认删除「${project.name}」？关联的任务/会话可能受影响。'),
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
      await api.deleteProject(project.id);
      bumpDataVersion(ref);
      ref.invalidate(projectsProvider(serverId));
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
}
