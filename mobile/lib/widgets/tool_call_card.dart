import 'dart:convert';

import 'package:flutter/material.dart';

/// 工具调用卡片，展示工具名、输入参数、执行结果与状态。
///
/// 状态流转：执行中(tertiary) → 完成(primary) / 出错(error)。
/// 由 MessageBubble 内嵌或会话页独立渲染。
class ToolCallCard extends StatelessWidget {
  final String toolUseId;
  final String toolName;
  final Object? input;
  final String? result;
  final bool isError;
  final bool done;

  const ToolCallCard({
    super.key,
    required this.toolUseId,
    required this.toolName,
    required this.input,
    required this.result,
    required this.isError,
    required this.done,
  });

  /// 格式化 input：String 直接返回；其他用 JsonEncoder.withIndent 缩进输出。
  String _formatInput(Object? src) {
    if (src == null) return '';
    if (src is String) return src;
    try {
      return const JsonEncoder.withIndent('  ').convert(src);
    } catch (_) {
      return src.toString();
    }
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final cs = theme.colorScheme;

    // 状态颜色与文案
    final Color statusColor;
    final String statusLabel;
    if (!done) {
      statusColor = cs.tertiary;
      statusLabel = '执行中…';
    } else if (isError) {
      statusColor = cs.error;
      statusLabel = '出错';
    } else {
      statusColor = cs.primary;
      statusLabel = '完成';
    }

    final hasInput = input != null;
    final hasResult = result != null && result!.isNotEmpty;

    return Container(
      decoration: BoxDecoration(
        border: Border.all(color: theme.dividerColor),
        borderRadius: BorderRadius.circular(8),
      ),
      clipBehavior: Clip.antiAlias,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          // 顶部状态栏
          Container(
            color: statusColor.withValues(alpha: 0.1),
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
            child: Row(
              children: [
                Icon(Icons.build, size: 16, color: statusColor),
                const SizedBox(width: 6),
                Expanded(
                  child: Text(
                    toolName.isEmpty ? '工具结果' : toolName,
                    style: theme.textTheme.labelLarge?.copyWith(
                      fontWeight: FontWeight.bold,
                    ),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
                Text(
                  statusLabel,
                  style: theme.textTheme.labelSmall?.copyWith(color: statusColor),
                ),
              ],
            ),
          ),
          // input 区
          if (hasInput)
            Padding(
              padding: const EdgeInsets.all(8),
              child: SelectableText(
                _formatInput(input),
                style: theme.textTheme.bodySmall?.copyWith(
                  fontFamily: 'monospace',
                  fontFamilyFallback: const ['RobotoMono', 'Courier'],
                  fontSize: 12,
                ),
              ),
            ),
          // result 区
          if (hasResult)
            Container(
              margin: const EdgeInsets.fromLTRB(8, 0, 8, 8),
              padding: const EdgeInsets.all(8),
              decoration: BoxDecoration(
                color: isError
                    ? cs.errorContainer.withValues(alpha: 0.4)
                    : cs.surfaceContainerHighest,
                borderRadius: BorderRadius.circular(4),
              ),
              child: SelectableText(
                result!,
                style: theme.textTheme.bodySmall?.copyWith(
                  fontFamily: 'monospace',
                  fontFamilyFallback: const ['RobotoMono', 'Courier'],
                  fontSize: 12,
                  color: isError ? cs.onErrorContainer : null,
                ),
              ),
            ),
        ],
      ),
    );
  }
}
