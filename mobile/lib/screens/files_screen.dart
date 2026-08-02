import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../models/file_entry.dart';
import '../state/providers.dart';
import '../widgets/empty_state.dart';
import '../widgets/error_view.dart';
import '../widgets/loading_indicator.dart';

/// 文件浏览页。
class FilesScreen extends ConsumerStatefulWidget {
  final String serverId;
  final String initialPath;

  const FilesScreen({
    super.key,
    required this.serverId,
    required this.initialPath,
  });

  @override
  ConsumerState<FilesScreen> createState() => _FilesScreenState();
}

class _FilesScreenState extends ConsumerState<FilesScreen> {
  late String _cwd;
  final List<String> _stack = [];
  bool _loading = false;
  String? _error;
  List<FileEntry> _entries = const [];

  @override
  void initState() {
    super.initState();
    _cwd = widget.initialPath;
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final api = ref.read(apiClientProvider);
      final listing = await api.listFiles(widget.serverId, _cwd);
      // 目录优先，再按名称排序。
      _entries = [...listing.entries]
        ..sort((a, b) {
          if (a.isDir != b.isDir) return a.isDir ? -1 : 1;
          return a.name.compareTo(b.name);
        });
    } catch (e) {
      _error = e.toString();
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  void _enter(FileEntry e) {
    if (e.isDir) {
      _stack.add(_cwd);
      _cwd = e.path;
      _load();
    } else {
      Navigator.of(context).push(
        MaterialPageRoute(
          builder: (_) => _FileEditorScreen(
            serverId: widget.serverId,
            file: e,
          ),
        ),
      );
    }
  }

  bool _back() {
    if (_stack.isEmpty) return false;
    _cwd = _stack.removeLast();
    _load();
    return true;
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        leading: IconButton(
          icon: const Icon(Icons.arrow_back),
          onPressed: () {
            if (!_back()) Navigator.of(context).pop();
          },
        ),
        title: Text(
          _cwd,
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
        ),
        actions: [
          IconButton(
            icon: const Icon(Icons.refresh),
            tooltip: '刷新',
            onPressed: _load,
          ),
        ],
      ),
      body: _buildBody(),
    );
  }

  Widget _buildBody() {
    if (_loading) return const LoadingIndicator();
    if (_error != null) {
      return ErrorView(message: _error!, onRetry: _load);
    }
    if (_entries.isEmpty) {
      return const EmptyState(icon: Icons.folder_open, title: '空目录');
    }
    return ListView.builder(
      itemCount: _entries.length,
      itemBuilder: (context, i) {
        final e = _entries[i];
        return ListTile(
          leading: Icon(
            e.isDir ? Icons.folder : Icons.insert_drive_file,
            color: e.isDir ? Theme.of(context).colorScheme.primary : null,
          ),
          title: Text(e.name),
          subtitle: e.isDir
              ? null
              : Text(_formatSize(e.size),
                  style: Theme.of(context).textTheme.labelSmall),
          onTap: () => _enter(e),
        );
      },
    );
  }
}

/// 内嵌文件编辑器。
class _FileEditorScreen extends ConsumerStatefulWidget {
  final String serverId;
  final FileEntry file;

  const _FileEditorScreen({required this.serverId, required this.file});

  @override
  ConsumerState<_FileEditorScreen> createState() => _FileEditorScreenState();
}

class _FileEditorScreenState extends ConsumerState<_FileEditorScreen> {
  final _ctrl = TextEditingController();
  bool _loading = true;
  bool _saving = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    try {
      final api = ref.read(apiClientProvider);
      final content = await api.readFile(widget.serverId, widget.file.path);
      _ctrl.text = content.content;
    } catch (e) {
      _error = e.toString();
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _save() async {
    setState(() => _saving = true);
    try {
      final api = ref.read(apiClientProvider);
      await api.writeFile(widget.serverId, widget.file.path, _ctrl.text);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('已保存')),
        );
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

  Future<void> _delete() async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('删除文件'),
        content: Text('确认删除「${widget.file.name}」？该操作不可撤销。'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: const Text('取消'),
          ),
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
      await api.deleteFile(widget.serverId, widget.file.path);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('已删除')),
        );
        Navigator.of(context).pop();
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('删除失败: $e')),
        );
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: Text(widget.file.name, maxLines: 1, overflow: TextOverflow.ellipsis),
        actions: [
          IconButton(
            icon: const Icon(Icons.delete_outline),
            tooltip: '删除',
            onPressed: _loading || _saving ? null : _delete,
          ),
          IconButton(
            icon: _saving
                ? const SizedBox(
                    width: 20,
                    height: 20,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : const Icon(Icons.save),
            tooltip: '保存',
            onPressed: _loading || _saving ? null : _save,
          ),
        ],
      ),
      body: _buildBody(),
    );
  }

  Widget _buildBody() {
    if (_loading) {
      return const LoadingIndicator(label: '加载文件…');
    }
    if (_error != null) {
      return ErrorView(message: _error!, onRetry: _load);
    }
    return Padding(
      padding: const EdgeInsets.all(8),
      child: TextField(
        controller: _ctrl,
        maxLines: null,
        expands: true,
        style: const TextStyle(fontFamily: 'monospace', fontSize: 13),
        decoration: const InputDecoration(
          border: InputBorder.none,
          isDense: true,
          contentPadding: EdgeInsets.zero,
        ),
      ),
    );
  }
}

/// 格式化文件大小。
String _formatSize(int bytes) {
  if (bytes < 1024) return '$bytes B';
  if (bytes < 1024 * 1024) return '${(bytes / 1024).toStringAsFixed(1)} KB';
  return '${(bytes / (1024 * 1024)).toStringAsFixed(1)} MB';
}
