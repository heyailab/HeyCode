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
  String? _testMsg;
  bool _testOk = false;

  @override
  void initState() {
    super.initState();
    final config = ref.read(configProvider);
    _urlCtrl = TextEditingController(
      text: config.baseUrl == AppConfig.defaultBaseUrl ? '' : config.baseUrl,
    );
  }

  @override
  void dispose() {
    _urlCtrl.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    final url = _urlCtrl.text.trim();
    final storage = ref.read(storageProvider);
    await storage.setBaseUrl(url);
    ref.read(configProvider.notifier).state =
        AppConfig.fromBaseUrl(url.isEmpty ? AppConfig.defaultBaseUrl : url);
    if (mounted) {
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('已保存')));
    }
  }

  Future<void> _test() async {
    await _save();
    try {
      final api = ref.read(apiClientProvider);
      final res = await api.health();
      final version = res['version'] ?? '?';
      setState(() {
        _testMsg = '连接成功（v$version）';
        _testOk = true;
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
