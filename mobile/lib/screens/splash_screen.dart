import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../state/providers.dart';
import '../widgets/loading_indicator.dart';

/// 启动页：indigo 渐变背景 + 终端图标 + 健康检查自动引导。
///
/// _bootstrap 流程：
/// 1. 等待 initConfigProvider 加载配置
/// 2. 调 health() 健康检查
/// 3. 成功 → context.go('/servers')
/// 4. 失败 → context.go('/settings')
class SplashScreen extends ConsumerStatefulWidget {
  const SplashScreen({super.key});

  @override
  ConsumerState<SplashScreen> createState() => _SplashScreenState();
}

class _SplashScreenState extends ConsumerState<SplashScreen> {
  String _label = '正在连接后端…';

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => _bootstrap());
  }

  Future<void> _bootstrap() async {
    try {
      await ref.read(initConfigProvider.future);
      final config = ref.read(configProvider);
      if (config.baseUrl.isNotEmpty) {
        setState(() => _label = '连接 ${config.baseUrl}…');
      }
      final api = ref.read(apiClientProvider);
      await api.health();
      if (mounted) context.go('/servers');
    } catch (e) {
      if (mounted) context.go('/settings');
    }
  }

  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;
    return Scaffold(
      body: Container(
        decoration: BoxDecoration(
          gradient: LinearGradient(
            begin: Alignment.topCenter,
            end: Alignment.bottomCenter,
            colors: [cs.primary, cs.primaryContainer],
          ),
        ),
        child: Center(
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Icon(Icons.terminal, size: 80, color: cs.onPrimary),
              const SizedBox(height: 16),
              Text(
                'HeyCode',
                style: Theme.of(context).textTheme.headlineMedium?.copyWith(
                      fontWeight: FontWeight.bold,
                      color: cs.onPrimary,
                    ),
              ),
              const SizedBox(height: 32),
              LoadingIndicator(label: _label),
            ],
          ),
        ),
      ),
    );
  }
}
