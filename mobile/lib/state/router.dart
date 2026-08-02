/// go_router 完整路由表。
///
/// 路由跳转约定：
/// - context.go：栈根替换（splash→servers、settings→servers）
/// - context.push：入栈（逐层导航、表单）
/// - context.pop：出栈（表单保存后返回）
library;

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../screens/files_screen.dart';
import '../screens/project_form_screen.dart';
import '../screens/projects_screen.dart';
import '../screens/server_form_screen.dart';
import '../screens/servers_screen.dart';
import '../screens/session_list_screen.dart';
import '../screens/session_screen.dart';
import '../screens/settings_screen.dart';
import '../screens/snapshot_history_screen.dart';
import '../screens/splash_screen.dart';
import '../screens/task_form_screen.dart';
import '../screens/tasks_screen.dart';

final routerProvider = Provider<GoRouter>((ref) {
  return GoRouter(
    initialLocation: '/',
    routes: [
      GoRoute(path: '/', builder: (c, s) => const SplashScreen()),
      GoRoute(path: '/settings', builder: (c, s) => const SettingsScreen()),
      GoRoute(path: '/servers', builder: (c, s) => const ServersScreen()),
      GoRoute(
        path: '/servers/new',
        builder: (c, s) => const ServerFormScreen(serverId: null),
      ),
      GoRoute(
        path: '/servers/:id/edit',
        builder: (c, s) => ServerFormScreen(serverId: s.pathParameters['id']!),
      ),
      GoRoute(
        path: '/servers/:id/files',
        builder: (c, s) => FilesScreen(
          serverId: s.pathParameters['id']!,
          initialPath: s.uri.queryParameters['path'] ?? '/',
        ),
      ),
      GoRoute(
        path: '/servers/:id/projects',
        builder: (c, s) => ProjectsScreen(serverId: s.pathParameters['id']!),
      ),
      GoRoute(
        path: '/servers/:id/projects/new',
        builder: (c, s) => ProjectFormScreen(serverId: s.pathParameters['id']!),
      ),
      GoRoute(
        path: '/projects/:id/tasks',
        builder: (c, s) => TasksScreen(projectId: s.pathParameters['id']!),
      ),
      GoRoute(
        path: '/projects/:id/tasks/new',
        builder: (c, s) => TaskFormScreen(projectId: s.pathParameters['id']!),
      ),
      GoRoute(
        path: '/tasks/:id/sessions',
        builder: (c, s) => SessionListScreen(taskId: s.pathParameters['id']!),
      ),
      GoRoute(
        path: '/sessions/:id',
        builder: (c, s) => SessionScreen(
          sessionId: s.pathParameters['id']!,
          taskId: s.uri.queryParameters['taskId'],
          cliWire: s.uri.queryParameters['cli'],
          cwd: s.uri.queryParameters['cwd'],
          model: s.uri.queryParameters['model'],
          serverId: s.uri.queryParameters['serverId'],
          startPrompt: s.uri.queryParameters['prompt'],
        ),
      ),
      GoRoute(
        path: '/sessions/:id/snapshots',
        builder: (c, s) => SnapshotHistoryScreen(
          sessionId: s.pathParameters['id']!,
          serverId: s.uri.queryParameters['serverId'],
          cwd: s.uri.queryParameters['cwd'],
        ),
      ),
    ],
    errorBuilder: (c, s) => Scaffold(
      appBar: AppBar(title: const Text('路由错误')),
      body: Center(child: Text(s.error?.toString() ?? '未知路由')),
    ),
  );
});
