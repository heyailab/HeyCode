import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../config.dart';
import '../models/unified_event.dart';
import '../state/providers.dart';
import '../widgets/loading_indicator.dart';

/// 设置页：后端地址配置 + API Key 管理。
class SettingsScreen extends ConsumerStatefulWidget {
  const SettingsScreen({super.key});

  @override
  ConsumerState<SettingsScreen> createState() => _SettingsScreenState();
}

class _SettingsScreenState extends ConsumerState<SettingsScreen> {
  late final TextEditingController _urlCtrl;
  late final TextEditingController _tokenCtrl;
  bool _obscureToken = true;
  String? _testMsg;
  bool _testOk = false;

  @override
  void initState() {
    super.initState();
    final config = ref.read(configProvider);
    _urlCtrl = TextEditingController(
      text: config.baseUrl == AppConfig.defaultBaseUrl ? '' : config.baseUrl,
    );
    _tokenCtrl = TextEditingController(text: config.authToken);
  }

  @override
  void dispose() {
    _urlCtrl.dispose();
    _tokenCtrl.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    final url = _urlCtrl.text.trim();
    final token = _tokenCtrl.text.trim();
    final storage = ref.read(storageProvider);
    await storage.setBaseUrl(url);
    await storage.setAuthToken(token);
    ref.read(configProvider.notifier).state = AppConfig.fromBaseUrl(
      url.isEmpty ? AppConfig.defaultBaseUrl : url,
      authToken: token,
    );
    if (mounted) {
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('已保存')));
    }
  }

  Future<void> _test() async {
    await _save();
    try {
      final api = ref.read(apiClientProvider);
      // 用 verifyAuth 而非 health：同时校验 URL 可达性与 token 正确性。
      // 后端鉴权未启用时 token 可空，verifyAuth 仍返回 ok:true, authEnabled:false。
      final res = await api.verifyAuth();
      final ok = res['ok'] == true;
      final authEnabled = res['authEnabled'] == true;
      final version = res['version'] ?? '?';
      setState(() {
        _testOk = ok;
        if (ok) {
          _testMsg = authEnabled
              ? '连接成功（v$version，鉴权已启用）'
              : '连接成功（v$version，鉴权未启用 - 仅本地调试可用）';
        } else {
          _testMsg = '连接失败';
        }
      });
    } catch (e) {
      setState(() {
        _testMsg = '连接失败: $e';
        _testOk = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;
    return Scaffold(
      appBar: AppBar(
        title: const Text('设置'),
        leading: IconButton(
          icon: const Icon(Icons.arrow_back),
          onPressed: () => context.go('/servers'),
        ),
      ),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          Text('后端地址', style: Theme.of(context).textTheme.titleMedium),
          const SizedBox(height: 8),
          TextField(
            controller: _urlCtrl,
            decoration: const InputDecoration(
              hintText: 'http://localhost:8787',
              prefixIcon: Icon(Icons.dns),
              border: OutlineInputBorder(),
            ),
            keyboardType: TextInputType.url,
            autocorrect: false,
          ),
          const SizedBox(height: 16),
          Text('鉴权 Token', style: Theme.of(context).textTheme.titleMedium),
          const SizedBox(height: 4),
          Text(
            '后端配置了 AUTH_TOKEN 时必填，否则请求会被拒绝。'
            '留空表示后端鉴权未启用（仅本地调试）。',
            style: Theme.of(context).textTheme.bodySmall,
          ),
          const SizedBox(height: 8),
          TextField(
            controller: _tokenCtrl,
            obscureText: _obscureToken,
            decoration: InputDecoration(
              hintText: 'openssl rand -hex 32 生成的 token',
              prefixIcon: const Icon(Icons.lock),
              border: const OutlineInputBorder(),
              suffixIcon: IconButton(
                icon: Icon(_obscureToken ? Icons.visibility : Icons.visibility_off),
                onPressed: () => setState(() => _obscureToken = !_obscureToken),
              ),
            ),
            autocorrect: false,
            enableSuggestions: false,
          ),
          const SizedBox(height: 12),
          Row(
            children: [
              FilledButton.icon(
                onPressed: _save,
                icon: const Icon(Icons.save),
                label: const Text('保存'),
              ),
              const SizedBox(width: 12),
              OutlinedButton.icon(
                onPressed: _test,
                icon: const Icon(Icons.wifi_find),
                label: const Text('测试连接'),
              ),
            ],
          ),
          if (_testMsg != null) ...[
            const SizedBox(height: 8),
            Text(
              _testMsg!,
              style: TextStyle(
                color: _testOk ? const Color(0xFF2E7D32) : cs.error,
              ),
            ),
          ],
          const Divider(height: 40),
          Row(
            children: [
              Text('API Key 管理', style: Theme.of(context).textTheme.titleMedium),
              const Spacer(),
              IconButton(
                icon: const Icon(Icons.refresh),
                onPressed: () => ref.invalidate(apiKeysProvider),
              ),
            ],
          ),
          _apiKeyList(),
          const SizedBox(height: 24),
          // 版本号：从 pubspec 运行时读取，发版自动同步。
          Center(
            child: ref.watch(appVersionProvider).when(
                  data: (v) => Text(
                    'HeyCode App v$v',
                    style: Theme.of(context).textTheme.bodySmall?.copyWith(
                          color: Theme.of(context).colorScheme.outline,
                        ),
                  ),
                  loading: () => const SizedBox.shrink(),
                  error: (_, __) => const SizedBox.shrink(),
                ),
          ),
        ],
      ),
    );
  }

  Widget _apiKeyList() {
    final keysAsync = ref.watch(apiKeysProvider);
    return keysAsync.when(
      loading: () => const SizedBox(
        height: 200,
        child: LoadingIndicator(label: '加载 API Key…'),
      ),
      error: (e, _) => Text('加载失败: $e', style: TextStyle(color: Theme.of(context).colorScheme.error)),
      data: (keys) {
        final keyMap = {for (final k in keys) k.cli: k};
        final clis = CliKind.values.where((c) => c != CliKind.pty).toList();
        return Column(
          children: clis.map((cli) {
            final meta = keyMap[cli];
            final hasKey = meta?.hasKey ?? false;
            return Card(
              margin: const EdgeInsets.symmetric(vertical: 4),
              child: ListTile(
                leading: Icon(Icons.key, color: Theme.of(context).colorScheme.primary),
                title: Text(cli.wire),
                subtitle: Text(hasKey ? '已设置 ····${meta!.last4 ?? ''}' : '未设置'),
                trailing: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    IconButton(
                      icon: const Icon(Icons.edit),
                      onPressed: () => _editKey(cli),
                    ),
                    if (hasKey)
                      IconButton(
                        icon: const Icon(Icons.delete_outline),
                        onPressed: () => _deleteKey(cli),
                      ),
                  ],
                ),
              ),
            );
          }).toList(),
        );
      },
    );
  }

  Future<void> _editKey(CliKind cli) async {
    final ctrl = TextEditingController();
    final result = await showDialog<String>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text('设置 ${cli.wire} API Key'),
        content: TextField(
          controller: ctrl,
          obscureText: true,
          decoration: const InputDecoration(
            labelText: 'API Key',
            border: OutlineInputBorder(),
          ),
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx), child: const Text('取消')),
          FilledButton(
            onPressed: () => Navigator.pop(ctx, ctrl.text.trim()),
            child: const Text('保存'),
          ),
        ],
      ),
    );
    ctrl.dispose();
    if (result == null || result.isEmpty) return;
    try {
      final api = ref.read(apiClientProvider);
      await api.upsertApiKey(cli: cli, key: result);
      ref.invalidate(apiKeysProvider);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('已保存')));
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('保存失败: $e')));
      }
    }
  }

  Future<void> _deleteKey(CliKind cli) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text('删除 ${cli.wire} API Key'),
        content: const Text('确定要删除此 API Key 吗？'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('取消')),
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
      await api.deleteApiKey(cli);
      ref.invalidate(apiKeysProvider);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('已删除')));
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('删除失败: $e')));
      }
    }
  }
}
