/// 快照历史页：展示会话内全部文件变更，支持单条/全部回滚。
///
/// 路由：`/sessions/:id/snapshots?serverId=...&cwd=...`
library;

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../models/file_snapshot.dart';
import '../services/api_client.dart';
import '../state/providers.dart';
import '../widgets/empty_state.dart';
import '../widgets/error_view.dart';
import '../widgets/loading_indicator.dart';
import '../widgets/snapshot_card.dart';

class SnapshotHistoryScreen extends ConsumerStatefulWidget {
  final String sessionId;
  final String? serverId;
  final String? cwd;

  const SnapshotHistoryScreen({
    super.key,
    required this.sessionId,
    this.serverId,
    this.cwd,
  });

  @override
  ConsumerState<SnapshotHistoryScreen> createState() =>
      _SnapshotHistoryScreenState();
}

class _SnapshotHistoryScreenState extends ConsumerState<SnapshotHistoryScreen> {
  bool _rollingAll = false;

  @override
  Widget build(BuildContext context) {
    final snapshotsAsync = ref.watch(snapshotsProvider(widget.sessionId));

    return Scaffold(
      appBar: AppBar(
        title: const Text('文件变更历史'),
        actions: [
          snapshotsAsync.maybeWhen(
            data: (list) => list.isEmpty
                ? const SizedBox.shrink()
                : _rollingAll
                    ? const Padding(
                        padding: EdgeInsets.all(14),
                        child: SizedBox(
                          width: 18,
                          height: 18,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        ),
                      )
                    : IconButton(
                        tooltip: '全部回滚',
                        icon: const Icon(Icons.restore_page_outlined),
                        onPressed: _rollingAll
                            ? null
                            : () => _confirmRollbackAll(list),
                      ),
            orElse: () => const SizedBox.shrink(),
          ),
        ],
      ),
      body: snapshotsAsync.when(
        data: (list) {
          if (list.isEmpty) {
            return const EmptyState(
              icon: Icons.history_outlined,
              title: '无文件变更',
              subtitle: '本会话尚未产生文件快照',
            );
          }
          return RefreshIndicator(
            onRefresh: () async {
              ref.invalidate(snapshotsProvider(widget.sessionId));
            },
            child: ListView.builder(
              padding: const EdgeInsets.symmetric(vertical: 4),
              itemCount: list.length,
              itemBuilder: (ctx, i) {
                final s = list[i];
                return SnapshotCard(
                  key: ValueKey(s.id),
                  snapshot: s,
                  serverId: widget.serverId,
                  cwd: widget.cwd,
                  onRolledBack: () =>
                      ref.invalidate(snapshotsProvider(widget.sessionId)),
                );
              },
            ),
          );
        },
        loading: () => const LoadingIndicator(label: '加载快照…'),
        error: (e, _) => ErrorView(
          message: e.toString(),
          onRetry: () =>
              ref.invalidate(snapshotsProvider(widget.sessionId)),
        ),
      ),
    );
  }

  // ---- 全部回滚 ----

  Future<void> _confirmRollbackAll(List<FileSnapshot> list) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('全部回滚？'),
        content: Text(
          '将回滚本会话全部 ${list.length} 条文件变更。该操作通过 git 还原，可能不可撤销。',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: const Text('取消'),
          ),
          FilledButton.tonal(
            onPressed: () => Navigator.pop(ctx, true),
            child: const Text('全部回滚'),
          ),
        ],
      ),
    );
    if (confirmed != true) return;

    setState(() => _rollingAll = true);
    try {
      final api = ref.read(apiClientProvider);
      final results = await api.rollbackSession(
        widget.sessionId,
        serverId: widget.serverId,
        cwd: widget.cwd,
      );
      if (!mounted) return;
      final rolled = results.where((r) => r.rolled).length;
      final failed = results.length - rolled;
      String msg;
      if (failed == 0) {
        msg = '全部回滚成功（$rolled 条）';
      } else {
        msg = '回滚完成：成功 $rolled 条，失败 $failed 条';
      }
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
      ref.invalidate(snapshotsProvider(widget.sessionId));
    } on ApiException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('全部回滚失败: ${e.message}')),
      );
    } catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('全部回滚失败: $e')),
      );
    } finally {
      if (mounted) setState(() => _rollingAll = false);
    }
  }
}
