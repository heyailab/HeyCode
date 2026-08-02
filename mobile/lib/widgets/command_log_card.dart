import 'package:flutter/material.dart';

import '../models/unified_event.dart';

/// 命令日志卡片，展示命令、退出码徽章与 stdout/stderr 终端风格输出。
///
/// 退出码：0 成功绿、非 0 错误红、null 运行中(outline)。
/// 输出区背景 0xFF1E1E1E，stdout 0xFFE0E0E0，stderr 0xFFEF9A9A。
class CommandLogCard extends StatelessWidget {
  final CommandExecEvent event;

  const CommandLogCard({super.key, required this.event});

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final cs = theme.colorScheme;
    final event = this.event;
    final hasStdout = event.stdout != null && event.stdout!.isNotEmpty;
    final hasStderr = event.stderr != null && event.stderr!.isNotEmpty;
    final hasOutput = hasStdout || hasStderr;

    // 退出码徽章：null 运行中(outline 描边)、0 成功绿、非 0 错误红
    final isRunning = event.exitCode == null;
    final Color badgeFg;
    final Color badgeBg;
    final String exitLabel;
    if (isRunning) {
      badgeFg = cs.outline;
      badgeBg = cs.surfaceContainerHighest;
      exitLabel = '运行中';
    } else if (event.exitCode == 0) {
      badgeFg = const Color(0xFF2E7D32);
      badgeBg = const Color(0xFF2E7D32).withValues(alpha: 0.12);
      exitLabel = 'exit 0';
    } else {
      badgeFg = const Color(0xFFC62828);
      badgeBg = const Color(0xFFC62828).withValues(alpha: 0.12);
      exitLabel = 'exit ${event.exitCode}';
    }

    return Card(
      margin: const EdgeInsets.symmetric(vertical: 6),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            // 顶部：终端图标 + 命令 + 退出码徽章
            Row(
              children: [
                Icon(Icons.terminal, size: 20, color: cs.primary),
                const SizedBox(width: 8),
                Expanded(
                  child: SelectableText(
                    event.command,
                    style: theme.textTheme.bodyMedium?.copyWith(
                      fontFamily: 'monospace',
                      fontFamilyFallback: const ['RobotoMono', 'Courier'],
                      fontWeight: FontWeight.bold,
                    ),
                    maxLines: 2,
                  ),
                ),
                const SizedBox(width: 8),
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                  decoration: BoxDecoration(
                    color: badgeBg,
                    borderRadius: BorderRadius.circular(10),
                    border: isRunning
                        ? Border.all(color: cs.outline, width: 0.8)
                        : null,
                  ),
                  child: Text(
                    exitLabel,
                    style: theme.textTheme.labelSmall?.copyWith(
                      color: badgeFg,
                      fontWeight: FontWeight.w600,
                      fontFamily: 'monospace',
                      fontFamilyFallback: const ['RobotoMono', 'Courier'],
                    ),
                  ),
                ),
              ],
            ),
            if (hasOutput) ...[
              const SizedBox(height: 8),
              Container(
                decoration: BoxDecoration(
                  color: const Color(0xFF1E1E1E),
                  borderRadius: BorderRadius.circular(6),
                ),
                constraints: const BoxConstraints(maxHeight: 280),
                padding: const EdgeInsets.all(8),
                child: SingleChildScrollView(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      if (hasStdout) ...[
                        const Text(
                          'stdout',
                          style: TextStyle(
                            color: Color(0xFF9E9E9E),
                            fontSize: 11,
                          ),
                        ),
                        const SizedBox(height: 2),
                        SelectableText(
                          event.stdout!,
                          style: const TextStyle(
                            color: Color(0xFFE0E0E0),
                            fontFamily: 'monospace',
                            fontFamilyFallback: ['RobotoMono', 'Courier'],
                            fontSize: 12,
                            height: 1.3,
                          ),
                        ),
                      ],
                      if (hasStderr) ...[
                        if (hasStdout) const SizedBox(height: 8),
                        const Text(
                          'stderr',
                          style: TextStyle(
                            color: Color(0xFF9E9E9E),
                            fontSize: 11,
                          ),
                        ),
                        const SizedBox(height: 2),
                        SelectableText(
                          event.stderr!,
                          style: const TextStyle(
                            color: Color(0xFFEF9A9A),
                            fontFamily: 'monospace',
                            fontFamilyFallback: ['RobotoMono', 'Courier'],
                            fontSize: 12,
                            height: 1.3,
                          ),
                        ),
                      ],
                    ],
                  ),
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }
}
