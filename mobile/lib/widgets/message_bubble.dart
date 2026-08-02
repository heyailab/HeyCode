import 'dart:convert';

import 'package:flutter/material.dart';

import '../models/unified_event.dart';
import 'tool_call_card.dart';

/// 消息气泡，按 role(user/assistant) 左右对齐，渲染 ContentBlock 列表。
///
/// user：右对齐，primaryContainer 背景，onPrimaryContainer 前景，右下角圆角 4。
/// assistant：左对齐，surfaceContainerHighest 背景，onSurface 前景，左下角圆角 4。
/// 最大宽度为屏幕宽度的 80%。
class MessageBubble extends StatelessWidget {
  final String role;
  final List<ContentBlock> blocks;
  final int timestamp;

  const MessageBubble({
    super.key,
    required this.role,
    required this.blocks,
    required this.timestamp,
  });

  bool get _isUser => role == 'user';

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final cs = theme.colorScheme;
    final isUser = _isUser;
    final screenWidth = MediaQuery.of(context).size.width;

    final bg = isUser ? cs.primaryContainer : cs.surfaceContainerHighest;
    final fg = isUser ? cs.onPrimaryContainer : cs.onSurface;
    // 圆角：user 右下 4，assistant 左下 4
    final radius = isUser
        ? const BorderRadius.only(
            topLeft: Radius.circular(16),
            topRight: Radius.circular(16),
            bottomLeft: Radius.circular(16),
            bottomRight: Radius.circular(4),
          )
        : const BorderRadius.only(
            topLeft: Radius.circular(16),
            topRight: Radius.circular(16),
            bottomLeft: Radius.circular(4),
            bottomRight: Radius.circular(16),
          );

    return Align(
      alignment: isUser ? Alignment.centerRight : Alignment.centerLeft,
      child: ConstrainedBox(
        constraints: BoxConstraints(maxWidth: screenWidth * 0.8),
        child: Container(
          decoration: BoxDecoration(color: bg, borderRadius: radius),
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
          child: DefaultTextStyle.merge(
            style: theme.textTheme.bodyMedium?.copyWith(color: fg) ??
                TextStyle(color: fg),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: _buildBlocks(theme),
            ),
          ),
        ),
      ),
    );
  }

  /// 按 ContentBlock 类型渲染子项，block 间留 6px 间距。
  List<Widget> _buildBlocks(ThemeData theme) {
    final widgets = <Widget>[];
    for (final b in blocks) {
      Widget w;
      switch (b) {
        case TextBlock(:final text):
          // 继承 DefaultTextStyle(bodyMedium + 前景色)
          w = SelectableText(text);
        case ThinkingBlock(:final text):
          w = SelectableText(
            text,
            style: theme.textTheme.bodySmall?.copyWith(
              color: theme.colorScheme.outline,
              fontStyle: FontStyle.italic,
            ),
          );
        case ImageBlock(:final dataB64):
          w = Image.memory(base64Decode(dataB64));
        case ToolUseBlock(:final toolUseId, :final toolName, :final input):
          w = ToolCallCard(
            toolUseId: toolUseId,
            toolName: toolName,
            input: input,
            result: null,
            isError: false,
            done: false,
          );
        case ToolResultBlock(:final toolUseId, :final isError):
          w = ToolCallCard(
            toolUseId: toolUseId,
            toolName: '',
            input: null,
            result: b.outputAsString,
            isError: isError ?? false,
            done: true,
          );
      }
      if (widgets.isNotEmpty) widgets.add(const SizedBox(height: 6));
      widgets.add(w);
    }
    if (widgets.isEmpty) widgets.add(const SizedBox.shrink());
    return widgets;
  }
}
