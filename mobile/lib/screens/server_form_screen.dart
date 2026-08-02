import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../models/server.dart';
import '../state/providers.dart';

/// 服务器表单页（新建/编辑）。
class ServerFormScreen extends ConsumerStatefulWidget {
  final String? serverId;

  const ServerFormScreen({super.key, required this.serverId});

  @override
  ConsumerState<ServerFormScreen> createState() => _ServerFormScreenState();
}

class _ServerFormScreenState extends ConsumerState<ServerFormScreen> {
  final _formKey = GlobalKey<FormState>();
  late final TextEditingController _nameCtrl;
  late final TextEditingController _hostCtrl;
  late final TextEditingController _portCtrl;
  late final TextEditingController _userCtrl;
  late final TextEditingController _passwordCtrl;
  late final TextEditingController _privateKeyCtrl;
  late final TextEditingController _passphraseCtrl;

  SshAuthKind _authKind = SshAuthKind.password;
  bool _saving = false;
  bool _testing = false;
  bool _loading = false;
  String? _loadError;

  bool get _isEdit => widget.serverId != null;

  @override
  void initState() {
    super.initState();
    _nameCtrl = TextEditingController();
    _hostCtrl = TextEditingController();
    _portCtrl = TextEditingController(text: '22');
    _userCtrl = TextEditingController();
    _passwordCtrl = TextEditingController();
    _privateKeyCtrl = TextEditingController();
    _passphraseCtrl = TextEditingController();
    if (_isEdit) {
      _load();
    }
  }

  @override
  void dispose() {
    _nameCtrl.dispose();
    _hostCtrl.dispose();
    _portCtrl.dispose();
    _userCtrl.dispose();
    _passwordCtrl.dispose();
    _privateKeyCtrl.dispose();
    _passphraseCtrl.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _loadError = null;
    });
    try {
      final api = ref.read(apiClientProvider);
      final s = await api.getServer(widget.serverId!);
      _nameCtrl.text = s.name;
      _hostCtrl.text = s.host;
      _portCtrl.text = s.port.toString();
      _userCtrl.text = s.username;
      _authKind = s.authKind;
    } catch (e) {
      _loadError = e.toString();
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _save() async {
    if (!_formKey.currentState!.validate()) return;
    setState(() => _saving = true);
    try {
      final api = ref.read(apiClientProvider);
      final name = _nameCtrl.text.trim();
      final host = _hostCtrl.text.trim();
      final port = int.tryParse(_portCtrl.text.trim()) ?? 22;
      final username = _userCtrl.text.trim();
      if (_isEdit) {
        SshAuth? auth;
        if (_authKind == SshAuthKind.password) {
          final pwd = _passwordCtrl.text;
          if (pwd.isNotEmpty) {
            auth = PasswordAuth(password: pwd);
          }
        } else if (_authKind == SshAuthKind.privateKey) {
          final key = _privateKeyCtrl.text;
          final pass = _passphraseCtrl.text;
          if (key.isNotEmpty) {
            auth = PrivateKeyAuth(
              privateKey: key,
              passphrase: pass.isEmpty ? null : pass,
            );
          }
        }
        await api.updateServer(
          widget.serverId!,
          name: name,
          host: host,
          port: port,
          username: username,
          auth: auth,
        );
      } else {
        final auth = _buildAuth();
        await api.createServer(
          name: name,
          host: host,
          port: port,
          username: username,
          auth: auth,
        );
      }
      bumpDataVersion(ref);
      ref.invalidate(serversProvider(null));
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(_isEdit ? '已保存' : '已创建')),
        );
        context.pop();
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('保存失败: $e')),
        );
      }
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  SshAuth _buildAuth() {
    switch (_authKind) {
      case SshAuthKind.password:
        return PasswordAuth(password: _passwordCtrl.text);
      case SshAuthKind.privateKey:
        final pass = _passphraseCtrl.text;
        return PrivateKeyAuth(
          privateKey: _privateKeyCtrl.text,
          passphrase: pass.isEmpty ? null : pass,
        );
      case SshAuthKind.agent:
        return const AgentAuth();
    }
  }

  Future<void> _test() async {
    if (!_formKey.currentState!.validate()) return;
    setState(() => _testing = true);
    try {
      final api = ref.read(apiClientProvider);
      final result = await api.testServer(widget.serverId!);
      if (!mounted) return;
      final msg = result.ok
          ? '连接成功，延迟 ${result.latencyMs ?? '-'} ms'
          : '连接失败: ${result.error ?? '未知错误'}';
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(msg)),
      );
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('测试失败: $e')),
        );
      }
    } finally {
      if (mounted) setState(() => _testing = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return Scaffold(
        appBar: AppBar(title: const Text('编辑服务器')),
        body: const Center(child: CircularProgressIndicator()),
      );
    }
    if (_loadError != null) {
      return Scaffold(
        appBar: AppBar(title: const Text('编辑服务器')),
        body: Center(child: Text('加载失败: $_loadError')),
      );
    }
    return Scaffold(
      appBar: AppBar(
        title: Text(_isEdit ? '编辑服务器' : '新增服务器'),
      ),
      body: Form(
        key: _formKey,
        child: ListView(
          padding: const EdgeInsets.all(16),
          children: [
            TextFormField(
              controller: _nameCtrl,
              decoration: const InputDecoration(
                labelText: '名称',
                prefixIcon: Icon(Icons.label),
                border: OutlineInputBorder(),
              ),
              validator: (v) =>
                  (v == null || v.trim().isEmpty) ? '必填' : null,
            ),
            const SizedBox(height: 12),
            TextFormField(
              controller: _hostCtrl,
              decoration: const InputDecoration(
                labelText: '主机地址',
                prefixIcon: Icon(Icons.dns),
                border: OutlineInputBorder(),
              ),
              validator: (v) =>
                  (v == null || v.trim().isEmpty) ? '必填' : null,
            ),
            const SizedBox(height: 12),
            TextFormField(
              controller: _portCtrl,
              decoration: const InputDecoration(
                labelText: '端口',
                prefixIcon: Icon(Icons.numbers),
                border: OutlineInputBorder(),
              ),
              keyboardType: TextInputType.number,
            ),
            const SizedBox(height: 12),
            TextFormField(
              controller: _userCtrl,
              decoration: const InputDecoration(
                labelText: '用户名',
                prefixIcon: Icon(Icons.person),
                border: OutlineInputBorder(),
              ),
              validator: (v) =>
                  (v == null || v.trim().isEmpty) ? '必填' : null,
            ),
            const SizedBox(height: 16),
            Text('认证方式',
                style: Theme.of(context).textTheme.titleSmall),
            const SizedBox(height: 8),
            SegmentedButton<SshAuthKind>(
              segments: const [
                ButtonSegment(
                  value: SshAuthKind.password,
                  label: Text('密码'),
                  icon: Icon(Icons.lock),
                ),
                ButtonSegment(
                  value: SshAuthKind.privateKey,
                  label: Text('私钥'),
                  icon: Icon(Icons.vpn_key),
                ),
                ButtonSegment(
                  value: SshAuthKind.agent,
                  label: Text('Agent'),
                  icon: Icon(Icons.usb),
                ),
              ],
              selected: {_authKind},
              onSelectionChanged: (s) => setState(() => _authKind = s.first),
            ),
            const SizedBox(height: 12),
            _buildAuthFields(),
            const SizedBox(height: 16),
            Row(
              children: [
                Expanded(
                  child: FilledButton.icon(
                    onPressed: _saving ? null : _save,
                    icon: _saving
                        ? const SizedBox(
                            width: 18,
                            height: 18,
                            child: CircularProgressIndicator(strokeWidth: 2),
                          )
                        : const Icon(Icons.save),
                    label: Text(_isEdit ? '保存修改' : '创建'),
                  ),
                ),
                if (_isEdit) ...[
                  const SizedBox(width: 12),
                  OutlinedButton.icon(
                    onPressed: _testing ? null : _test,
                    icon: _testing
                        ? const SizedBox(
                            width: 18,
                            height: 18,
                            child: CircularProgressIndicator(strokeWidth: 2),
                          )
                        : const Icon(Icons.electrical_services),
                    label: const Text('测试'),
                  ),
                ],
              ],
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildAuthFields() {
    switch (_authKind) {
      case SshAuthKind.password:
        return TextFormField(
          controller: _passwordCtrl,
          obscureText: true,
          decoration: InputDecoration(
            labelText: '密码',
            prefixIcon: const Icon(Icons.lock),
            border: const OutlineInputBorder(),
            hintText: _isEdit ? '留空表示不修改' : null,
          ),
          validator: (v) {
            if (_isEdit) return null;
            if (v == null || v.isEmpty) return '必填';
            return null;
          },
        );
      case SshAuthKind.privateKey:
        return Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            TextFormField(
              controller: _privateKeyCtrl,
              maxLines: 5,
              decoration: InputDecoration(
                labelText: '私钥',
                prefixIcon: const Icon(Icons.vpn_key),
                border: const OutlineInputBorder(),
                hintText: _isEdit ? '留空表示不修改' : '-----BEGIN OPENSSH PRIVATE KEY-----',
              ),
              validator: (v) {
                if (_isEdit) return null;
                if (v == null || v.isEmpty) return '必填';
                return null;
              },
            ),
            const SizedBox(height: 12),
            TextFormField(
              controller: _passphraseCtrl,
              obscureText: true,
              decoration: const InputDecoration(
                labelText: 'Passphrase（可选）',
                prefixIcon: Icon(Icons.password),
                border: OutlineInputBorder(),
              ),
            ),
          ],
        );
      case SshAuthKind.agent:
        return Padding(
          padding: const EdgeInsets.symmetric(vertical: 8),
          child: Text(
            '使用本机 SSH Agent 转发认证。',
            style: Theme.of(context).textTheme.bodyMedium,
          ),
        );
    }
  }
}
