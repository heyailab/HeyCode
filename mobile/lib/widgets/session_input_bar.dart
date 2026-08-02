import 'package:flutter/material.dart';

/// 会话输入栏，包含自适应 TextField 与发送/中断按钮。
///
/// isRunning 时显示"中断"按钮(FilledButton.tonalIcon)，否则显示"发送"按钮(FilledButton.icon)。
/// 发送：controller.text 非空时调用 onSend，然后清空输入框。
class SessionInputBar extends StatefulWidget {
  final TextEditingController controller;
  final VoidCallback onSend;
  final VoidCallback onInterrupt;
  final bool isRunning;

  const SessionInputBar({
    super.key,
    required this.controller,
    required this.onSend,
    required this.onInterrupt,
    required this.isRunning,
  });

  @override
  State<SessionInputBar> createState() => _SessionInputBarState();
}

class _SessionInputBarState extends State<SessionInputBar> {
  final FocusNode _focus = FocusNode();

  @override
  void dispose() {
    _focus.dispose();
    super.dispose();
  }

  /// 发送逻辑：文本非空时回调 onSend，清空输入框并重新请求焦点。
  void _submit() {
    final text = widget.controller.text.trim();
    if (text.isEmpty) return;
    widget.onSend();
    widget.controller.clear();
    _focus.requestFocus();
  }

  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;
    return SafeArea(
      top: false,
      child: Material(
        elevation: 4,
        color: cs.surface,
        child: Padding(
          padding: const EdgeInsets.all(8),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.end,
            children: [
              Expanded(
                child: TextField(
                  controller: widget.controller,
                  focusNode: _focus,
                  minLines: 1,
                  maxLines: 5,
                  textInputAction: TextInputAction.newline,
                  decoration: InputDecoration(
                    hintText: '输入消息…',
                    filled: true,
                    fillColor: cs.surfaceContainerHighest,
                    contentPadding: const EdgeInsets.symmetric(
                      horizontal: 16,
                      vertical: 10,
                    ),
                    border: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(24),
                      borderSide: BorderSide.none,
                    ),
                    enabledBorder: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(24),
                      borderSide: BorderSide.none,
                    ),
                    focusedBorder: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(24),
                      borderSide: BorderSide.none,
                    ),
                  ),
                  onSubmitted: (_) => _submit(),
                ),
              ),
              const SizedBox(width: 8),
              widget.isRunning
                  ? FilledButton.tonalIcon(
                      onPressed: widget.onInterrupt,
                      icon: const Icon(Icons.stop),
                      label: const Text('中断'),
                    )
                  : FilledButton.icon(
                      onPressed: _submit,
                      icon: const Icon(Icons.send),
                      label: const Text('发送'),
                    ),
            ],
          ),
        ),
      ),
    );
  }
}
