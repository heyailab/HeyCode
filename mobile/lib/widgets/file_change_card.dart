import 'package:flutter/material.dart';

import '../models/unified_event.dart';

/// 文件变更卡片，展示路径、动作(create/edit/delete)、增删行数，可展开查看 diff。
///
/// diff 按行着色：+ 行透明绿背景，- 行透明红背景，@@ 行信息蓝，其余原样。
class FileChangeCard extends StatefulWidget {
  final FileChange change;

  const FileChangeCard({super.key, required this.change});

  @override
  State<FileChangeCard> createState() => _FileChangeCardState();
}

class _FileChangeCardState extends State<FileChangeCard> {
  bool _expanded = false;

  (IconData, Color) _actionVisual(FileChangeAction action) {
    switch (action) {
      case FileChangeAction.create:
        return (Icons.add_circle_outline, const Color(0xFF2E7D32));
      case FileChangeAction.edit:
        return (Icons.edit_outlined, const Color(0xFF1565C0));
      case FileChangeAction.delete:
        return (Icons.delete_outline, const Color(0xFFC62828));
    }
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final change = widget.change;
    final (icon, actionColor) = _actionVisual(change.action);
    final hasDiff = change.diff != null && change.diff!.isNotEmpty;

    return Card(
      margin: const EdgeInsets.symmetric(vertical: 6),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          ListTile(
            leading: Icon(icon, color: actionColor),
            title: SelectableText(
              change.path,
              style: theme.textTheme.bodyMedium?.copyWith(
                fontFamily: 'monospace',
                fontFamilyFallback: const ['RobotoMono', 'Courier'],
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
                  _actionChip(change.action.wire, actionColor),
                  if (change.addedLines != null)
                    Text(
                      '+${change.addedLines}',
                      style: const TextStyle(
                        color: Color(0xFF2E7D32),
                        fontSize: 12,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                  if (change.removedLines != null)
                    Text(
                      '-${change.removedLines}',
                      style: const TextStyle(
                        color: Color(0xFFC62828),
                        fontSize: 12,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                ],
              ),
            ),
            trailing: hasDiff
                ? IconButton(
                    tooltip: _expanded ? '收起 diff' : '查看 diff',
                    icon: Icon(_expanded ? Icons.expand_less : Icons.expand_more),
                    onPressed: () => setState(() => _expanded = !_expanded),
                  )
                : null,
            onTap: hasDiff ? () => setState(() => _expanded = !_expanded) : null,
          ),
          if (hasDiff && _expanded) _diffView(change.diff!),
        ],
      ),
    );
  }

  /// 动作标签：半透明背景 + 动作色文字。
  Widget _actionChip(String label, Color color) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Text(
        label,
        style: TextStyle(
          color: color,
          fontSize: 11,
          fontWeight: FontWeight.w600,
        ),
      ),
    );
  }

  /// diff 视图：单栏内联，按行着色。
  Widget _diffView(String diff) {
    final theme = Theme.of(context);
    final lines = diff.split('\n');
    return Container(
      margin: const EdgeInsets.fromLTRB(8, 0, 8, 8),
      padding: const EdgeInsets.all(8),
      decoration: BoxDecoration(
        color: theme.colorScheme.surfaceContainerHighest,
        borderRadius: BorderRadius.circular(6),
      ),
      constraints: const BoxConstraints(maxHeight: 320),
      child: SingleChildScrollView(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: lines.map((l) => _diffLine(l)).toList(),
        ),
      ),
    );
  }

  /// 单行 diff 渲染，依据行首前缀着色。
  Widget _diffLine(String text) {
    final theme = Theme.of(context);
    Color fg;
    Color? bg;
    if (text.startsWith('+++') || text.startsWith('---')) {
      fg = const Color(0xFF616161);
    } else if (text.startsWith('@@')) {
      fg = const Color(0xFF1565C0);
    } else if (text.startsWith('+')) {
      fg = const Color(0xFF2E7D32);
      bg = const Color(0x1A2E7D32);
    } else if (text.startsWith('-')) {
      fg = const Color(0xFFC62828);
      bg = const Color(0x1AC62828);
    } else {
      fg = const Color(0xFF424242);
    }
    return Container(
      color: bg,
      padding: const EdgeInsets.symmetric(horizontal: 4),
      child: Text(
        text,
        style: theme.textTheme.bodySmall?.copyWith(
          fontFamily: 'monospace',
          fontFamilyFallback: const ['RobotoMono', 'Courier'],
          fontSize: 12,
          height: 1.3,
          color: fg,
        ),
        softWrap: false,
      ),
    );
  }
}
