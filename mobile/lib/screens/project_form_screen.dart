import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../models/unified_event.dart';
import '../state/providers.dart';

/// 项目表单页：在指定服务器下创建项目。
class ProjectFormScreen extends ConsumerStatefulWidget {
  final String serverId;

  const ProjectFormScreen({super.key, required this.serverId});

  @override
  ConsumerState<ProjectFormScreen> createState() => _ProjectFormScreenState();
}

class _ProjectFormScreenState extends ConsumerState<ProjectFormScreen> {
  final _formKey = GlobalKey<FormState>();
  final _nameCtrl = TextEditingController();
  final _cwdCtrl = TextEditingController();
  final _modelCtrl = TextEditingController();
  final _rulesCtrl = TextEditingController();

  CliKind _defaultCli = CliKind.claudeCode;
  bool _saving = false;

  @override
  void initState() {
    super.initState();
    _loadServer();
  }

  /// 加载服务器信息，默认将工作目录填为 /home/${username}。
  Future<void> _loadServer() async {
    try {
      final server = await ref.read(serverProvider(widget.serverId).future);
      if (!mounted) return;
      final username = server.username.isNotEmpty ? server.username : 'user';
      _cwdCtrl.text = '/home/$username';
    } catch (_) {
      // 加载失败时留空，让用户手动填写
    }
  }

  @override
  void dispose() {
    _nameCtrl.dispose();
    _cwdCtrl.dispose();
    _modelCtrl.dispose();
    _rulesCtrl.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    if (!_formKey.currentState!.validate()) return;
    setState(() => _saving = true);
    try {
      final api = ref.read(apiClientProvider);
      await api.createProject(
        serverId: widget.serverId,
        name: _nameCtrl.text.trim(),
        cwd: _cwdCtrl.text.trim(),
        defaultCli: _defaultCli,
        defaultModel:
            _modelCtrl.text.trim().isEmpty ? null : _modelCtrl.text.trim(),
        rules: _rulesCtrl.text.trim().isEmpty ? null : _rulesCtrl.text.trim(),
      );
      bumpDataVersion(ref);
      if (mounted) Navigator.of(context).pop();
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('创建失败：$e')),
        );
      }
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('新增项目')),
      body: Form(
        key: _formKey,
        child: ListView(
          padding: const EdgeInsets.all(16),
          children: [
            TextFormField(
              controller: _nameCtrl,
              decoration: const InputDecoration(
                labelText: '项目名称',
                prefixIcon: Icon(Icons.label),
                hintText: 'my-project',
              ),
              textInputAction: TextInputAction.next,
              validator: (v) =>
                  (v == null || v.trim().isEmpty) ? '请输入项目名称' : null,
            ),
            const SizedBox(height: 16),
            TextFormField(
              controller: _cwdCtrl,
              decoration: const InputDecoration(
                labelText: '工作目录',
                prefixIcon: Icon(Icons.folder),
                hintText: '/home/user/project',
              ),
              textInputAction: TextInputAction.next,
              validator: (v) =>
                  (v == null || v.trim().isEmpty) ? '请输入工作目录' : null,
            ),
            const SizedBox(height: 24),
            Text('默认 CLI', style: Theme.of(context).textTheme.titleSmall),
            const SizedBox(height: 8),
            DropdownButtonFormField<CliKind>(
              value: _defaultCli,
              decoration: const InputDecoration(
                prefixIcon: Icon(Icons.smart_toy),
              ),
              items: CliKind.values
                  .where((c) => c != CliKind.pty)
                  .map((c) => DropdownMenuItem(
                        value: c,
                        child: Text(c.wire),
                      ))
                  .toList(),
              onChanged: (v) {
                if (v != null) setState(() => _defaultCli = v);
              },
            ),
            const SizedBox(height: 16),
            TextFormField(
              controller: _modelCtrl,
              decoration: const InputDecoration(
                labelText: '默认模型（可选）',
                prefixIcon: Icon(Icons.memory),
                hintText: 'claude-sonnet-4-5 / gpt-4o / ...',
              ),
              textInputAction: TextInputAction.next,
            ),
            const SizedBox(height: 16),
            TextFormField(
              controller: _rulesCtrl,
              decoration: const InputDecoration(
                labelText: '规则（可选）',
                prefixIcon: Icon(Icons.rule),
                hintText: 'AGENTS.md / .claude/rules 等',
                alignLabelWithHint: true,
              ),
              maxLines: 5,
            ),
            const SizedBox(height: 24),
            FilledButton.icon(
              onPressed: _saving ? null : _save,
              icon: _saving
                  ? const SizedBox(
                      width: 16,
                      height: 16,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Icon(Icons.save),
              label: Text(_saving ? '创建中…' : '创建项目'),
            ),
          ],
        ),
      ),
    );
  }
}
