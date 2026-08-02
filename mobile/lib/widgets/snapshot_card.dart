/// 文件快照卡片。
///
/// 根据 SPEC §14.10 实现：展示单条文件变更（路径/动作/增删行数/时间），
/// 支持查看双栏 diff 与单条回滚。
library;

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';

import '../models/file_snapshot.dart';
import '../services/api_client.dart';
import '../state/providers.dart';
import 'diff_side_by_side.dart';

class SnapshotCard extends ConsumerStatefulWidget {
  final FileSnapshot snapshot;
  final String? serverId;
  final String? cwd;
  final VoidCallback? onRolledBack;

  const SnapshotCard({
    super.key,
    required this.snapshot,
    required this.serverId,
    required this.cwd,
    this.onRolledBack,
  });

  @override
  ConsumerState<SnapshotCard> createState() => _SnapshotCardState();
}

class _SnapshotCardState extends ConsumerState<SnapshotCard> {
  bool _rolling = false;
  bool _rolled = false; // 已成功回滚的标记

  FileSnapshot get s => widget.snapshot;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final action = s.action;
    final actionColor = _actionColor(action);
    final hasDiff = s.diff != null && s.diff!.isNotEmpty;

    return Card(
      margin: const EdgeInsets.symmetric(vertical: 4, horizontal: 8),
      child: ListTile(
        leading: Icon(_actionIcon(action), color: actionColor),
        title: SelectableText(
          s.path,
          style: theme.textTheme.bodyMedium?.copyWith(
            fontWeight: FontWeight.w600,
          ),
          maxLines: 2,
        ),
        subtitle: Padding(
          padding: const EdgeInsets.only(top: 4),
          child: Wrap(
            crossAxisAlignment: WrapCrossAlignment.center,
            spacing: 8,
            runSpacing: 4,
            children: [
              _actionChip(action, actionColor),
              if (s.addedLines != null && s.addedLines! > 0)
                Text(
                  '+${s.addedLines}',
                  style: const TextStyle(
                    color: Color(0xFF2E7D32),
                    fontSize: 12,
                  ),
                ),
              if (s.removedLines != null && s.removedLines! > 0)
                Text(
                  '-${s.removedLines}',
                  style: const TextStyle(
                    color: Color(0xFFC62828),
                    fontSize: 12,
                  ),
                ),
              Text(
                _fmtTime(s.createdAt),
                style: TextStyle(
                  color: theme.colorScheme.outline,
                  fontSize: 11,
                ),
              ),
              if (_rolled)
                Text(
                  '已回滚',
                  style: TextStyle(
                    color: theme.colorScheme.outline,
                    fontSize: 11,
                    fontStyle: FontStyle.italic,
                  ),
                ),
            ],
          ),
        ),
        trailing: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            if (hasDiff)
              IconButton(
                tooltip: '查看 diff',
                icon: const Icon(Icons.compare_arrows_outlined, size: 20),
                onPressed: _showDiff,
              ),
            IconButton(
              tooltip: '回滚',
              icon: _rolling
                  ? const SizedBox(
                      width: 18,
                      height: 18,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Icon(Icons.restore_outlined, size: 20),
              onPressed: (_rolling || _rolled) ? null : _confirmRollback,
            ),
          ],
        ),
      ),
    );
  }

  // ---- 辅助样式 ----

  IconData _actionIcon(String action) {
    switch (action) {
      case 'create':
        return Icons.add_circle_outline;
      case 'delete':
        return Icons.delete_outline;
      case 'edit':
      default:
        return Icons.edit_outlined;
    }
  }

  Color _actionColor(String action) {
    switch (action) {
      case 'create':
        return const Color(0xFF2E7D32);
      case 'delete':
        return const Color(0xFFC62828);
      case 'edit':
      default:
        return const Color(0xFF1565C0);
    }
  }

  Widget _actionChip(String action, Color color) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Text(
        action,
        style: TextStyle(
          color: color,
          fontSize: 11,
          fontWeight: FontWeight.w600,
        ),
      ),
    );
  }

  String _fmtTime(DateTime t) {
    return DateFormat('yyyy-MM-dd HH:mm').format(t);
  }

  // ---- diff 弹层 ----

  void _showDiff() {
    showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      showDragHandle: true,
      builder: (ctx) {
        final height = MediaQuery.of(ctx).size.height * 0.8;
        return SizedBox(
          height: height,
          child: Column(
            children: [
              Padding(
                padding: const EdgeInsets.symmetric(horizontal: 12),
                child: Row(
                  children: [
                    Expanded(
                      child: Text(
                        s.path,
                        style: const TextStyle(fontWeight: FontWeight.w600),
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                    IconButton(
                      icon: const Icon(Icons.close),
                      onPressed: () => Navigator.pop(ctx),
                    ),
                  ],
                ),
              ),
              const Divider(height: 1),
              Expanded(child: DiffSideBySide(diff: s.diff ?? '')),
            ],
          ),
        );
      },
    );
  }

  // ---- 单条回滚 ----

  Future<void> _confirmRollback() async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('回滚此变更？'),
        content: Text(
          '将回滚对以下文件的变更：\n${s.path}\n\n该操作会通过 git 还原文件，可能不可撤销。',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: const Text('取消'),
          ),
          FilledButton.tonal(
            onPressed: () => Navigator.pop(ctx, true),
            child: const Text('确认回滚'),
          ),
        ],
      ),
    );
    if (confirmed != true) return;

    setState(() => _rolling = true);
    try {
      final api = ref.read(apiClientProvider);
      final result = await api.rollbackSnapshot(
        s.id,
        serverId: widget.serverId,
        cwd: widget.cwd,
      );
      if (!mounted) return;
      if (result.rolled) {
        setState(() {
          _rolled = true;
          _rolling = false;
        });
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('已回滚：${result.path}（${result.method}）')),
        );
        widget.onRolledBack?.call();
      } else {
        setState(() => _rolling = false);
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('回滚失败：${result.message}')),
        );
      }
    } on ApiException catch (e) {
      if (!mounted) return;
      setState(() => _rolling = false);
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('回滚失败: ${e.message}')),
      );
    } catch (e) {
      if (!mounted) return;
      setState(() => _rolling = false);
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('回滚失败: $e')),
      );
    }
  }
}
