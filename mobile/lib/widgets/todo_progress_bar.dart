import 'package:flutter/material.dart';

import '../models/unified_event.dart';

/// 待办进度条，展示完成进度与任务清单。
///
/// 顶部：完成数/总数 + LinearProgressIndicator；下方：每个 TodoItem 的状态图标与内容。
/// 状态图标：pending→radio_button_unchecked(outline)，
/// inProgress→pending_actions(tertiary)，completed→check_circle(0xFF2E7D32)。
class TodoProgressBar extends StatelessWidget {
  final List<TodoItem> todos;

  const TodoProgressBar({super.key, required this.todos});

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final cs = theme.colorScheme;
    final total = todos.length;
    final completed =
        todos.where((t) => t.status == TodoStatus.completed).length;
    final value = total > 0 ? completed / total : 0.0;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: [
        Padding(
          padding: const EdgeInsets.all(12),
          child: Row(
            children: [
              Text('待办 $completed/$total', style: theme.textTheme.titleSmall),
              const SizedBox(width: 12),
              Expanded(
                child: ClipRRect(
                  borderRadius: BorderRadius.circular(8),
                  child: LinearProgressIndicator(
                    value: value,
                    minHeight: 8,
                  ),
                ),
              ),
            ],
          ),
        ),
        if (todos.isEmpty)
          Padding(
            padding: const EdgeInsets.all(12),
            child: Center(
              child: Text('暂无任务', style: TextStyle(color: cs.outline)),
            ),
          )
        else
          ListView(
            shrinkWrap: true,
            physics: const NeverScrollableScrollPhysics(),
            padding: const EdgeInsets.symmetric(horizontal: 12),
            children: todos.map((t) => _todoTile(context, t)).toList(),
          ),
      ],
    );
  }

  /// 单个待办项：状态图标 + 内容 + 可选进度百分比。
  Widget _todoTile(BuildContext context, TodoItem todo) {
    final theme = Theme.of(context);
    final isDone = todo.status == TodoStatus.completed;
    final (icon, iconColor) = _statusVisual(context, todo.status);
    return ListTile(
      dense: true,
      leading: Icon(icon, color: iconColor, size: 22),
      title: Text(
        todo.content,
        style: isDone
            ? theme.textTheme.bodyMedium?.copyWith(
                decoration: TextDecoration.lineThrough,
                color: theme.colorScheme.outline,
              )
            : theme.textTheme.bodyMedium,
      ),
      trailing: todo.progress != null
          ? Text(
              '${todo.progress}%',
              style: theme.textTheme.labelSmall?.copyWith(color: iconColor),
            )
          : null,
    );
  }

  /// 状态 → (图标, 颜色)。
  (IconData, Color) _statusVisual(BuildContext context, TodoStatus s) {
    final cs = Theme.of(context).colorScheme;
    switch (s) {
      case TodoStatus.pending:
        return (Icons.radio_button_unchecked, cs.outline);
      case TodoStatus.inProgress:
        return (Icons.pending_actions, cs.tertiary);
      case TodoStatus.completed:
        return (Icons.check_circle, const Color(0xFF2E7D32));
    }
  }
}
