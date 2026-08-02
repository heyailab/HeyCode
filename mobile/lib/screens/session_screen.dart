/// 会话页：5 个 Tab（消息/工具/文件/命令/进度）+ 流式消息 + 状态横幅。
///
/// 参考 SPEC-FLUTTER-APP.md：
/// - §10 SessionController（启动入口、事件分发、流式聚合）
/// - §12.12 SessionScreen 概要
/// - §13 会话页详细规范（布局、AppBar、Tab、横幅、stats 行）
library;

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../models/session.dart' show SessionStatus;
import '../models/unified_event.dart';
import '../services/ws_client.dart' show WsConnectionState;
import '../state/providers.dart';
import '../state/session_controller.dart';
import '../widgets/command_log_card.dart';
import '../widgets/empty_state.dart';
import '../widgets/file_change_card.dart';
import '../widgets/message_bubble.dart';
import '../widgets/session_input_bar.dart';
import '../widgets/todo_progress_bar.dart';
import '../widgets/tool_call_card.dart';

class SessionScreen extends ConsumerStatefulWidget {
  /// path 参数。特殊值 'new' 表示新建模式。
  final String sessionId;
  final String? taskId;
  final String? cliWire;
  final String? cwd;
  final String? model;
  final String? serverId;

  /// 首条 prompt；非 null 时进入"新建会话模式"。
  final String? startPrompt;

  const SessionScreen({
    super.key,
    required this.sessionId,
    this.taskId,
    this.cliWire,
    this.cwd,
    this.model,
    this.serverId,
    this.startPrompt,
  });

  @override
  ConsumerState<SessionScreen> createState() => _SessionScreenState();
}

class _SessionScreenState extends ConsumerState<SessionScreen>
    with TickerProviderStateMixin {
  late final TabController _tabController;
  final TextEditingController _inputController = TextEditingController();
  final ScrollController _scrollController = ScrollController();
  bool _booted = false;

  /// 新建模式：sessionId == 'new' 或显式带 startPrompt。
  bool get _isStartMode =>
      widget.sessionId == 'new' || widget.startPrompt != null;

  @override
  void initState() {
    super.initState();
    _tabController = TabController(length: 5, vsync: this);
    WidgetsBinding.instance.addPostFrameCallback((_) => _boot());
  }

  @override
  void dispose() {
    _tabController.dispose();
    _inputController.dispose();
    _scrollController.dispose();
    super.dispose();
  }

  /// 启动会话引导。
  ///
  /// 新建模式 + sessionId=='new'：依赖 taskId，调 startSessionRaw（内部 createSession + 连 WS）。
  /// 新建模式 + 已有 sessionId：调 startExistingSession（连 WS + 发首条指令）。
  /// 恢复模式：调 resumeSession（拉历史事件回放 + 连 WS）。
  Future<void> _boot() async {
    if (_booted) return;
    _booted = true;
    final ctrl = ref.read(sessionControllerProvider.notifier);
    final cli = CliKind.fromWire(widget.cliWire);
    if (_isStartMode) {
      if (widget.sessionId == 'new') {
        if (widget.taskId == null || widget.taskId!.isEmpty) {
          // 缺少 taskId 无法新建会话
          return;
        }
        await ctrl.startSessionRaw(
          taskId: widget.taskId!,
          serverId: widget.serverId ?? '',
          cwd: widget.cwd ?? '',
          cli: cli,
          prompt: widget.startPrompt ?? '',
          model: widget.model,
        );
      } else {
        // session 记录已存在，仅连 WS + 发首条指令
        await ctrl.startExistingSession(
          sessionId: widget.sessionId,
          serverId: widget.serverId ?? '',
          cwd: widget.cwd ?? '',
          cli: cli,
          prompt: widget.startPrompt ?? '',
          model: widget.model,
        );
      }
    } else {
      await ctrl.resumeSession(sessionId: widget.sessionId);
    }
  }

  /// 消息列表新增时自动滚到底部。
  void _scrollToBottom() {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!_scrollController.hasClients) return;
      final max = _scrollController.position.maxScrollExtent;
      _scrollController.animateTo(
        max,
        duration: const Duration(milliseconds: 200),
        curve: Curves.easeOut,
      );
    });
  }

  void _onSend() {
    final text = _inputController.text.trim();
    if (text.isEmpty) return;
    ref.read(sessionControllerProvider.notifier).sendMessage(text);
    _inputController.clear();
  }

  void _onInterrupt() {
    ref.read(sessionControllerProvider.notifier).interruptSession();
  }

  void _onEnd() {
    ref.read(sessionControllerProvider.notifier).endSession();
  }

  void _openSnapshots() {
    // 新建模式下 sessionId 尚未确定，跳过
    final sid = widget.sessionId == 'new' ? '' : widget.sessionId;
    if (sid.isEmpty) return;
    final state = ref.read(sessionControllerProvider);
    final serverId = widget.serverId ?? '';
    final cwd = state.cwd ?? widget.cwd ?? '';
    final query = <String>[];
    if (serverId.isNotEmpty) query.add('serverId=${Uri.encodeQueryComponent(serverId)}');
    if (cwd.isNotEmpty) query.add('cwd=${Uri.encodeQueryComponent(cwd)}');
    final qs = query.isEmpty ? '' : '?${query.join('&')}';
    context.push('/sessions/$sid/snapshots$qs');
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(sessionControllerProvider);
    // 监听消息条数变化触发自动滚动
    ref.listen<SessionControllerState>(sessionControllerProvider, (prev, next) {
      if (prev?.messages.length != next.messages.length) {
        _scrollToBottom();
      }
    });

    final ended = state.status == SessionStatus.ended;
    return Scaffold(
      appBar: _buildAppBar(state),
      body: Column(
        children: [
          if (state.wsState == WsConnectionState.reconnecting)
            _reconnectingBanner(context),
          if (state.errorMessage != null)
            _errorBanner(context, state.errorMessage!),
          Expanded(
            child: TabBarView(
              controller: _tabController,
              children: [
                _buildMessagesTab(state),
                _buildToolsTab(state),
                _buildFilesTab(state),
                _buildCommandsTab(state),
                _buildProgressTab(state),
              ],
            ),
          ),
          if (ended)
            _endedBanner(context, state)
          else
            SessionInputBar(
              controller: _inputController,
              onSend: _onSend,
              onInterrupt: _onInterrupt,
              isRunning: state.status == SessionStatus.running,
            ),
        ],
      ),
    );
  }

  // ---- AppBar ----

  PreferredSizeWidget _buildAppBar(SessionControllerState state) {
    final theme = Theme.of(context);
    final cliWire = state.cli?.wire ?? widget.cliWire ?? '会话';
    return AppBar(
      title: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(cliWire),
          Row(
            children: [
              Icon(Icons.circle, size: 8, color: _statusColor(state)),
              const SizedBox(width: 6),
              Text(_statusLabel(state), style: theme.textTheme.labelSmall),
              if (state.model != null) ...[
                const SizedBox(width: 8),
                Text('· ${state.model}', style: theme.textTheme.labelSmall),
              ],
            ],
          ),
        ],
      ),
      actions: [
        PopupMenuButton<String>(
          tooltip: '更多操作',
          onSelected: (v) {
            if (v == 'snapshots') {
              _openSnapshots();
            } else if (v == 'end') {
              _onEnd();
            }
          },
          itemBuilder: (_) => const [
            PopupMenuItem(
              value: 'snapshots',
              child: Row(
                children: [
                  Icon(Icons.history_outlined),
                  SizedBox(width: 8),
                  Text('快照历史'),
                ],
              ),
            ),
            PopupMenuItem(
              value: 'end',
              child: Row(
                children: [
                  Icon(Icons.power_settings_new),
                  SizedBox(width: 8),
                  Text('结束会话'),
                ],
              ),
            ),
          ],
        ),
      ],
      bottom: TabBar(
        controller: _tabController,
        isScrollable: true,
        tabs: [
          Tab(text: '消息 (${state.messages.length})'),
          Tab(text: '工具 (${state.toolCalls.length})'),
          Tab(text: '文件 (${state.fileChanges.length})'),
          Tab(text: '命令 (${state.commandLogs.length})'),
          const Tab(text: '进度'),
        ],
      ),
    );
  }

  // ---- 状态颜色与文案 ----

  Color _statusColor(SessionControllerState state) {
    final theme = Theme.of(context);
    switch (state.status) {
      case SessionStatus.running:
        return const Color(0xFF2E7D32);
      case SessionStatus.idle:
        return theme.colorScheme.tertiary;
      case SessionStatus.ended:
        return theme.colorScheme.outline;
      case SessionStatus.error:
        return theme.colorScheme.error;
    }
  }

  String _statusLabel(SessionControllerState state) {
    switch (state.status) {
      case SessionStatus.running:
        return '运行中';
      case SessionStatus.idle:
        return '空闲';
      case SessionStatus.ended:
        return '已结束';
      case SessionStatus.error:
        return '出错';
    }
  }

  // ---- 状态横幅 ----

  /// 重连横幅：wsState == reconnecting 时显示。
  Widget _reconnectingBanner(BuildContext context) {
    final theme = Theme.of(context);
    return Container(
      color: theme.colorScheme.tertiaryContainer,
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
      child: Row(
        children: [
          const SizedBox(
            width: 14,
            height: 14,
            child: CircularProgressIndicator(strokeWidth: 2),
          ),
          const SizedBox(width: 8),
          Text(
            '正在重连…',
            style: TextStyle(color: theme.colorScheme.onTertiaryContainer),
          ),
        ],
      ),
    );
  }

  /// 错误横幅：errorMessage != null 时显示。
  Widget _errorBanner(BuildContext context, String msg) {
    final theme = Theme.of(context);
    return Container(
      color: theme.colorScheme.errorContainer,
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
      child: Row(
        children: [
          Icon(
            Icons.error_outline,
            size: 16,
            color: theme.colorScheme.onErrorContainer,
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              '错误: $msg',
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(color: theme.colorScheme.onErrorContainer),
            ),
          ),
        ],
      ),
    );
  }

  /// 会话结束横幅：替换输入栏，显示 "会话已结束" + stats 摘要。
  Widget _endedBanner(BuildContext context, SessionControllerState state) {
    final theme = Theme.of(context);
    return SafeArea(
      top: false,
      child: Container(
        width: double.infinity,
        padding: const EdgeInsets.all(12),
        color: theme.colorScheme.surfaceContainerHighest,
        child: Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(
              Icons.flag_outlined,
              size: 18,
              color: theme.colorScheme.outline,
            ),
            const SizedBox(width: 8),
            Text(
              '会话已结束',
              style: TextStyle(color: theme.colorScheme.outline),
            ),
            if (state.stats != null) ...[
              const SizedBox(width: 12),
              Text(
                _statsLine(state.stats!),
                style: theme.textTheme.labelSmall,
              ),
            ],
          ],
        ),
      ),
    );
  }

  /// stats 摘要行：用 ' · ' 连接非空字段。
  String _statsLine(SessionStats s) {
    final parts = <String>[];
    if (s.numTurns != null) parts.add('${s.numTurns} 轮');
    if (s.durationMs != null) {
      parts.add('${(s.durationMs! / 1000).toStringAsFixed(1)}s');
    }
    if (s.costUsd != null) {
      parts.add('\$${s.costUsd!.toStringAsFixed(4)}');
    }
    if (s.inputTokens != null || s.outputTokens != null) {
      parts.add('${s.inputTokens ?? 0}/${s.outputTokens ?? 0} tok');
    }
    return parts.join(' · ');
  }

  // ---- 5 个 Tab ----

  /// Tab 1 - 消息面板
  Widget _buildMessagesTab(SessionControllerState state) {
    if (state.messages.isEmpty) {
      return EmptyState(
        icon: Icons.chat_bubble_outline,
        title: '还没有消息',
        subtitle: state.status == SessionStatus.running
            ? '等待 AI 响应…'
            : '在底部输入消息开始对话',
      );
    }
    return ListView.builder(
      controller: _scrollController,
      padding: const EdgeInsets.symmetric(vertical: 8),
      itemCount: state.messages.length,
      itemBuilder: (_, i) {
        final m = state.messages[i];
        return MessageBubble(
          role: m.role,
          blocks: m.blocks,
          timestamp: m.timestamp,
        );
      },
    );
  }

  /// Tab 2 - 工具面板
  Widget _buildToolsTab(SessionControllerState state) {
    if (state.toolCalls.isEmpty) {
      return const EmptyState(
        icon: Icons.build_outlined,
        title: '还没有工具调用',
      );
    }
    return ListView.builder(
      padding: const EdgeInsets.all(8),
      itemCount: state.toolCalls.length,
      itemBuilder: (_, i) {
        final tc = state.toolCalls[i];
        return ToolCallCard(
          toolUseId: tc.toolUseId,
          toolName: tc.toolName,
          input: tc.input,
          result: tc.result,
          isError: tc.isError,
          done: tc.done,
        );
      },
    );
  }

  /// Tab 3 - 文件面板
  Widget _buildFilesTab(SessionControllerState state) {
    return Column(
      children: [
        if (widget.serverId != null && widget.cwd != null)
          Align(
            alignment: Alignment.centerRight,
            child: Padding(
              padding: const EdgeInsets.only(top: 4, right: 8),
              child: TextButton.icon(
                onPressed: _openSnapshots,
                icon: const Icon(Icons.history_outlined),
                label: const Text('查看变更历史'),
              ),
            ),
          ),
        Expanded(
          child: state.fileChanges.isEmpty
              ? const EmptyState(
                  icon: Icons.folder_off_outlined,
                  title: '没有文件变更',
                )
              : ListView.builder(
                  padding: const EdgeInsets.symmetric(vertical: 4),
                  itemCount: state.fileChanges.length,
                  itemBuilder: (_, i) =>
                      FileChangeCard(change: state.fileChanges[i]),
                ),
        ),
      ],
    );
  }

  /// Tab 4 - 命令面板
  Widget _buildCommandsTab(SessionControllerState state) {
    if (state.commandLogs.isEmpty) {
      return const EmptyState(icon: Icons.terminal, title: '没有命令日志');
    }
    return ListView.builder(
      padding: const EdgeInsets.symmetric(vertical: 4),
      itemCount: state.commandLogs.length,
      itemBuilder: (_, i) =>
          CommandLogCard(event: state.commandLogs[i]),
    );
  }

  /// Tab 5 - 进度面板：ProgressEvent + SessionStats + TodoProgressBar。
  Widget _buildProgressTab(SessionControllerState state) {
    final hasProgress = state.progress != null;
    final hasStats = state.stats != null;
    final hasTodos = state.todos.isNotEmpty;
    if (!hasProgress && !hasStats && !hasTodos) {
      return const EmptyState(
        icon: Icons.task_alt,
        title: '暂无进度信息',
      );
    }
    return Column(
      children: [
        if (hasProgress || hasStats)
          Padding(
            padding: const EdgeInsets.all(12),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                if (hasProgress) _buildProgressInfo(state.progress!),
                if (hasProgress && hasStats) const SizedBox(height: 8),
                if (hasStats)
                  Text(
                    _statsLine(state.stats!),
                    style: Theme.of(context).textTheme.labelSmall,
                  ),
              ],
            ),
          ),
        Expanded(
          child: hasTodos
              ? TodoProgressBar(todos: state.todos)
              : Center(
                  child: Text(
                    '暂无任务',
                    style: TextStyle(color: Theme.of(context).colorScheme.outline),
                  ),
                ),
        ),
      ],
    );
  }

  /// ProgressEvent 展示：message + 进度条 + step/total。
  Widget _buildProgressInfo(ProgressEvent p) {
    final theme = Theme.of(context);
    final hasBar = p.total != null && p.total! > 0;
    final value = hasBar ? ((p.step ?? 0) / p.total!).clamp(0.0, 1.0) : 0.0;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (p.message != null)
          Text(p.message!, style: theme.textTheme.titleSmall),
        if (hasBar) ...[
          if (p.message != null) const SizedBox(height: 8),
          Row(
            children: [
              Expanded(
                child: ClipRRect(
                  borderRadius: BorderRadius.circular(8),
                  child: LinearProgressIndicator(
                    value: value,
                    minHeight: 12,
                  ),
                ),
              ),
              const SizedBox(width: 12),
              Text(
                '${(value * 100).round()}%',
                style: theme.textTheme.titleMedium,
              ),
            ],
          ),
          const SizedBox(height: 4),
          Text(
            '步骤 ${p.step ?? 0} / ${p.total}',
            style: theme.textTheme.labelSmall
                ?.copyWith(color: theme.colorScheme.outline),
          ),
        ],
      ],
    );
  }
}
