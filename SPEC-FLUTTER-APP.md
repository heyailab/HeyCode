# HeyCode App 开发文档

> 面向 AI 代码生成助手的完整开发规范，从零开始实现一个 Flutter 移动端 App。
> 本文档自包含，按章节顺序实现即可交付完整产品。

---

## 目录

- [§1 项目概述](#1-项目概述)
- [§2 技术栈与依赖](#2-技术栈与依赖)
- [§3 项目目录结构](#3-项目目录结构)
- [§4 主题与全局样式](#4-主题与全局样式)
- [§5 数据模型层](#5-数据模型层)
- [§6 服务层：REST 客户端](#6-服务层rest-客户端)
- [§7 服务层：WebSocket 客户端](#7-服务层websocket-客户端)
- [§8 服务层：本地存储](#8-服务层本地存储)
- [§9 状态管理层（Riverpod）](#9-状态管理层riverpod)
- [§10 会话状态控制器](#10-会话状态控制器)
- [§11 路由配置](#11-路由配置)
- [§12 屏幕规范](#12-屏幕规范)
- [§13 会话页详细规范](#13-会话页详细规范)
- [§14 组件规范](#14-组件规范)
- [§15 工具类：diff 解析与着色](#15-工具类diff-解析与着色)
- [§16 Android 平台配置](#16-android-平台配置)
- [§17 代码规范与 lint](#17-代码规范与-lint)
- [§18 构建与发布](#18-构建与发布)
- [§19 GitHub Actions CI](#19-github-actions-ci)
- [§20 任务分解（Milestones）](#20-任务分解milestones)
- [附录 A：REST 端点速查](#附录-arest-端点速查)
- [附录 B：WebSocket 协议速查](#附录-bwebsocket-协议速查)
- [附录 C：事件类型速查](#附录-c事件类型速查)

---

## §1 项目概述

### 1.1 产品定位

HeyCode App 是 **AI 编程助手远程接管系统**的移动端控制台。用户在手机上配置远程 SSH 服务器，通过自托管的后端服务接管服务器上已安装的 AI 编程 CLI（claude-code / codex / gemini / trae / opencode / lingma），在移动端实时查看 AI 的对话、工具调用、文件变更、命令执行与任务进度，并可随时下发新指令或回滚文件变更。

### 1.2 核心功能

1. **后端连接配置**：填入自托管后端地址，支持连通性测试
2. **API Key 管理**：为每种 CLI 配置 API Key（加密存储于后端）
3. **服务器管理**：CRUD SSH 服务器（密码 / 私钥 / Agent 三种认证），支持连通性测试
4. **SFTP 文件浏览与编辑**：浏览远程目录、读写文本文件
5. **项目管理**：在服务器下创建项目（绑定工作目录、默认 CLI、默认模型、规则）
6. **任务管理**：在项目下创建任务
7. **会话管理**：在任务下发起会话，与 AI CLI 实时交互
8. **实时会话视图**：5 个 Tab（消息/工具/文件/命令/进度）+ 流式消息 + WebSocket 实时推送
9. **文件变更历史与回滚**：查看会话内全部文件快照、单条/全部回滚（git 还原）

### 1.3 非功能要求

- **平台**：Android（minSdk 24，targetSdk 36），未来可扩展 iOS
- **语言**：Dart 3（使用 sealed class、模式匹配）
- **状态管理**：Riverpod 2.x
- **路由**：go_router 14.x
- **网络**：Dio（REST）+ web_socket_channel（WS）
- **持久化**：SharedPreferences（轻量配置）
- **支持明文 HTTP**：开发期连 `http://192.168.x.x:port`，AndroidManifest 开启 `usesCleartextTraffic`
- **字体**：Noto Sans SC（中文友好）
- **主题**：Material 3，亮色为主

### 1.4 与后端的协议契约

- **REST 基础地址**：用户配置，如 `http://192.168.1.10:8787`
- **WebSocket 地址**：由 REST 基础地址推导，`http://` → `ws://`，`https://` → `wss://`，路径 `/ws`
- **鉴权**：当前无鉴权（单用户自托管场景），保留 API Key 字段供未来扩展
- **数据格式**：REST 用 JSON，WS 用 JSON 文本帧
- **事件协议**：服务端推送 `ServerEnvelope`（含 `eventId` + `event`），客户端下发 `ClientCommand`

---

## §2 技术栈与依赖

### 2.1 pubspec.yaml 完整依赖

```yaml
name: heycode_app
description: HeyCode 移动端 - AI 编程助手远程接管控制台
publish_to: 'none'
version: 1.0.0+1

environment:
  sdk: '>=3.4.0 <4.0.0'
  flutter: '>=3.22.0'

dependencies:
  flutter:
    sdk: flutter
  # 状态管理
  flutter_riverpod: ^2.5.1
  riverpod_annotation: ^2.3.5
  # 路由
  go_router: ^14.2.0
  # 网络
  dio: ^5.7.0
  web_socket_channel: ^3.0.1
  # 本地存储
  shared_preferences: ^2.3.2
  # 字体
  google_fonts: ^6.2.1
  # 国际化/日期格式
  intl: ^0.19.0

dev_dependencies:
  flutter_test:
    sdk: flutter
  flutter_lints: ^4.0.0
```

### 2.2 关键库职责

| 库 | 用途 |
|---|---|
| `flutter_riverpod` | 全局状态管理（Provider/StateProvider/FutureProvider/StateNotifierProvider） |
| `go_router` | 声明式路由，支持路径参数与查询参数 |
| `dio` | HTTP 客户端，拦截器、超时、错误转换 |
| `web_socket_channel` | WebSocket 客户端，`IOWebSocketChannel` 用于移动平台 |
| `shared_preferences` | 键值对本地存储（baseUrl / last.serverId / last.projectId） |
| `google_fonts` | 运行时加载 Noto Sans SC 字体 |
| `intl` | 日期格式化（`yyyy-MM-dd HH:mm`） |

### 2.3 开发环境要求

- Flutter SDK ≥ 3.22.0（stable channel）
- Dart SDK ≥ 3.4.0
- JDK 17（Android 构建）
- Android SDK：compileSdk 36，minSdk 24
- AGP（Android Gradle Plugin）8.7.3
- Kotlin 2.0.20

---

## §3 项目目录结构

```
heycode_app/
├── lib/
│   ├── main.dart                      # App 入口，ProviderScope + MaterialApp.router
│   ├── config.dart                    # AppConfig（baseUrl + wsUrl 推导）
│   ├── models/                        # 数据模型（全部不可变，fromJson/toJson）
│   │   ├── server.dart                # Server, SshAuth, FileListing, FileContent, ServerTestResult
│   │   ├── project.dart               # Project
│   │   ├── task.dart                  # Task, TaskStatus
│   │   ├── session.dart               # Session, SessionStatus
│   │   ├── file_entry.dart            # FileEntry
│   │   ├── file_snapshot.dart         # FileSnapshot, RollbackResult
│   │   └── unified_event.dart         # UnifiedEvent（sealed）, ContentBlock, ClientCommand, ServerEnvelope, CliKind
│   ├── services/                      # 服务层
│   │   ├── api_client.dart            # Dio REST 客户端 + ApiException + ApiKeyMeta
│   │   ├── ws_client.dart             # WebSocket 客户端（状态机 + 重连 + 心跳）
│   │   └── storage.dart               # SharedPreferences 包装
│   ├── state/                         # 状态层
│   │   ├── providers.dart             # 全部 Riverpod providers
│   │   ├── router.dart                # go_router 配置
│   │   └── session_controller.dart    # SessionController + SessionControllerState
│   ├── screens/                       # 屏幕层（每屏一个文件）
│   │   ├── splash_screen.dart
│   │   ├── settings_screen.dart
│   │   ├── servers_screen.dart
│   │   ├── server_form_screen.dart
│   │   ├── files_screen.dart          # 含内嵌 _FileEditorScreen
│   │   ├── projects_screen.dart
│   │   ├── project_form_screen.dart
│   │   ├── tasks_screen.dart
│   │   ├── task_form_screen.dart
│   │   ├── session_list_screen.dart
│   │   ├── session_screen.dart
│   │   └── snapshot_history_screen.dart
│   ├── widgets/                       # 可复用组件
│   │   ├── message_bubble.dart
│   │   ├── tool_call_card.dart
│   │   ├── file_change_card.dart
│   │   ├── command_log_card.dart
│   │   ├── todo_progress_bar.dart
│   │   ├── session_input_bar.dart
│   │   ├── snapshot_card.dart
│   │   ├── diff_side_by_side.dart
│   │   ├── loading_indicator.dart
│   │   ├── empty_state.dart
│   │   └── error_view.dart
│   └── utils/
│       └── diff_painter.dart          # parseDiff + parseDiffSideBySide
├── android/                           # Android 平台代码
│   ├── app/
│   │   ├── build.gradle.kts
│   │   ├── proguard-rules.pro
│   │   └── src/main/
│   │       ├── AndroidManifest.xml
│   │       ├── kotlin/com/heycode/app/MainActivity.kt
│   │       └── res/
│   │           ├── values/styles.xml
│   │           ├── drawable/launch_background.xml
│   │           └── mipmap-*/ic_launcher.png
│   ├── build.gradle.kts
│   ├── settings.gradle.kts
│   ├── gradle.properties
│   ├── key.properties.example
│   └── local.properties               # 由 flutter 自动生成
├── test/
│   └── widget_test.dart
├── analysis_options.yaml
├── pubspec.yaml
├── build-android.sh                   # 本地构建辅助脚本
└── README.md
```

---

## §4 主题与全局样式

### 4.1 main.dart 入口

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:google_fonts/google_fonts.dart';
import 'state/router.dart';

void main() {
  runApp(const ProviderScope(child: HeyCodeApp()));
}

class HeyCodeApp extends ConsumerWidget {
  const HeyCodeApp({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final router = ref.watch(routerProvider);
    return MaterialApp.router(
      title: 'HeyCode',
      debugShowCheckedModeBanner: false,
      theme: _buildTheme(),
      routerConfig: router,
    );
  }

  ThemeData _buildTheme() {
    return ThemeData(
      useMaterial3: true,
      colorScheme: ColorScheme.fromSeed(
        seedColor: const Color(0xFF3F51B5),  // indigo
        brightness: Brightness.light,
      ),
      textTheme: GoogleFonts.notoSansScTextTheme(),
      appBarTheme: const AppBarTheme(
        centerTitle: false,
        elevation: 0,
        scrolledUnderElevation: 1,
      ),
      cardTheme: const CardThemeData(
        elevation: 0,
        margin: EdgeInsets.zero,
      ),
    );
  }
}
```

### 4.2 颜色方案

- **种子色**：`Color(0xFF3F51B5)`（indigo）
- **亮度**：仅亮色（`Brightness.light`）
- **ColorScheme.fromSeed** 自动生成全套语义色

### 4.3 语义硬色（不依赖 ColorScheme 的固定色值）

以下色值在各 widget 内硬编码，必须严格使用：

| 语义 | 色值 | 用途 |
|---|---|---|
| 成功绿 | `0xFF2E7D32` | 状态点 ok、create action、exit 0、+N 行、completed todo |
| 错误红 | `0xFFC62828` | 失败状态、delete action、exit 非 0、-N 行 |
| 信息蓝 | `0xFF1565C0` | edit action、diff hunk 头、双栏 header 文字 |
| 中性灰 | `0xFF616161` | diff header 文字 |
| 中性深灰 | `0xFF424242` | diff context 文字 |
| 终端背景 | `0xFF1E1E1E` | 命令日志输出区背景 |
| 终端 stdout | `0xFFE0E0E0` | stdout 文字 |
| 终端 stderr | `0xFFEF9A9A` | stderr 文字 |
| 透明绿背景 | `0x1A2E7D32` | diff add 行背景（10% alpha） |
| 透明红背景 | `0x1AC62828` | diff remove 行背景（10% alpha） |
| 极浅灰背景 | `0x0D000000` | 双栏 diff context 行背景 |
| 浅蓝背景 | `0xFFE3F2FD` | 双栏 diff header 行背景 |

### 4.4 ColorScheme 字段使用约定

| ColorScheme 字段 | 用途 |
|---|---|
| `primary` / `onPrimary` | 主操作色、发送按钮 |
| `primaryContainer` / `onPrimaryContainer` | user 消息气泡、CircleAvatar 背景 |
| `secondaryContainer` / `onSecondaryContainer` | 项目卡片头像 |
| `tertiary` / `tertiaryContainer` / `onTertiaryContainer` | "执行中"状态、idle 会话、重连横幅 |
| `surfaceContainerHighest` | assistant 消息气泡、输入栏 fillColor、命令日志卡背景、结束横幅 |
| `error` / `errorContainer` / `onErrorContainer` | 错误状态、错误横幅、错误工具结果 |
| `outline` | 副文本、未测试状态、空状态图标 |
| `dividerColor` | 工具卡片边框 |

### 4.5 字体配置

- **全局 textTheme**：`GoogleFonts.notoSansScTextTheme()`（思源黑体）
- **等宽字体**（用于 host、cwd、command、diff、tool input/output）：
  ```dart
  TextStyle(
    fontFamily: 'monospace',
    fontFamilyFallback: ['RobotoMono', 'Courier'],
    fontSize: 12,  // 或 13，视组件而定
  )
  ```

### 4.6 全局样式约定

- **AppBar**：`centerTitle: false`、`elevation: 0`、`scrolledUnderElevation: 1`
- **Card**：`elevation: 0`、`margin: EdgeInsets.zero`（业务代码内用 `margin: symmetric(vertical: 6)` 覆盖）
- **ListView padding**：列表统一 `EdgeInsets.all(12)` 或 `symmetric(vertical: 4)`
- **加载中按钮样式**：用 `SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2))` 替换 icon
- **错误反馈**：`ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('xxx失败: $e')))`
- **成功反馈**：`SnackBar(content: Text('已保存'))` 等简短文案
- **下拉刷新**：所有列表页用 `RefreshIndicator` + `ref.invalidate(provider)`

---

## §5 数据模型层

### 5.1 通用规约

- 所有 model **不可变**（`final` 字段）
- 所有 `fromJson` 对缺失字段**有默认回退**（`?? ''`、`?? 0`、`?? false`、枚举 `fromWire` 默认值、DateTime `?? DateTime.now()`），**永不抛异常**
- 枚举统一用 `wire` 字符串字段 + `fromWire` 静态方法 + `firstWhere(orElse:)` 回退模式
- 可空字段在 `toJson` 中用 `if (field != null) 'field': field` 条件输出
- **Dart 3 sealed class** 用于 union 类型：`UnifiedEvent`、`ContentBlock`、`SshAuth`、`ClientCommand`
- 仅 `TodoItem` 提供 `copyWith`；持久化 model 都不提供

### 5.2 CliKind 枚举（全局共用）

```dart
enum CliKind {
  claudeCode('claude-code'),
  codex('codex'),
  gemini('gemini'),
  trae('trae'),
  opencode('opencode'),
  lingma('lingma'),
  pty('pty');

  final String wire;
  const CliKind(this.wire);

  static CliKind fromWire(String? v) =>
      values.firstWhere((e) => e.wire == v, orElse: () => CliKind.pty);
}
```

**默认回退**：`pty`。

### 5.3 Server 与 SshAuth

**`SshAuthKind` 枚举**：`password` / `privateKey` / `agent`，默认回退 `password`。

**`SshAuth` sealed class**：

```dart
sealed class SshAuth {
  const SshAuth();
  Map<String, dynamic> toJson();

  factory SshAuth.fromJson(Map<String, dynamic> j) {
    final kind = j['kind'] as String?;
    switch (kind) {
      case 'password':
        return PasswordAuth(password: j['password'] as String? ?? '');
      case 'privateKey':
        return PrivateKeyAuth(
          privateKey: j['privateKey'] as String? ?? '',
          passphrase: j['passphrase'] as String?,
        );
      case 'agent':
        return const AgentAuth();
      default:
        return PasswordAuth(password: '');
    }
  }
}

class PasswordAuth extends SshAuth {
  final String password;
  const PasswordAuth({required this.password});
  @override
  Map<String, dynamic> toJson() => {'kind': 'password', 'password': password};
}

class PrivateKeyAuth extends SshAuth {
  final String privateKey;
  final String? passphrase;
  const PrivateKeyAuth({required this.privateKey, this.passphrase});
  @override
  Map<String, dynamic> toJson() => {
        'kind': 'privateKey',
        'privateKey': privateKey,
        if (passphrase != null) 'passphrase': passphrase,
      };
}

class AgentAuth extends SshAuth {
  const AgentAuth();
  @override
  Map<String, dynamic> toJson() => {'kind': 'agent'};
}
```

**`ServerStatus` 枚举**：`ok` / `fail` / `unknown`，默认回退 `unknown`（`fromWire(null)` 也回退 unknown）。

**`Server` 字段**：

| 字段 | 类型 | 默认/回退 |
|---|---|---|
| `id` | String | `''` |
| `name` | String | `''` |
| `host` | String | `''` |
| `port` | int | `22` |
| `username` | String | `''` |
| `authKind` | SshAuthKind | `password` |
| `createdAt` | DateTime | `DateTime.now()`（解析失败时） |
| `lastStatus` | ServerStatus | `unknown` |
| `lastCheckedAt` | DateTime? | null |

`toJson` 中 `lastStatus != unknown` 才输出，`lastCheckedAt != null` 才输出。**无 copyWith**。

**辅助类**：

```dart
class FileListing {
  final String path;
  final List<FileEntry> entries;
  // fromJson 从 j['entries'] 解析
}

class FileContent {
  final String path;
  final String content;
  final int size;  // 默认 0
}

class ServerTestResult {
  final bool ok;        // 默认 false
  final int? latencyMs;
  final String? error;
}
```

### 5.4 Project

| 字段 | 类型 | 默认/回退 |
|---|---|---|
| `id` | String | `''` |
| `serverId` | String | `''` |
| `name` | String | `''` |
| `cwd` | String | `''` |
| `defaultCli` | CliKind | `pty` |
| `defaultModel` | String? | null |
| `rules` | String? | null |
| `createdAt` | DateTime | `DateTime.now()` |

`toJson` 中 `defaultModel` / `rules` 非 null 才输出。**无 copyWith**。

### 5.5 Task

**`TaskStatus` 枚举**：`planned` / `inProgress('in_progress')` / `done` / `archived`，默认回退 `planned`。

| 字段 | 类型 | 默认/回退 |
|---|---|---|
| `id` | String | `''` |
| `projectId` | String | `''` |
| `title` | String | `''` |
| `description` | String? | null |
| `status` | TaskStatus | `planned` |
| `createdAt` | DateTime | `DateTime.now()` |
| `updatedAt` | DateTime | `DateTime.now()` |

**无 copyWith**。

### 5.6 Session

**`SessionStatus` 枚举**：`running` / `idle` / `ended` / `error`，默认回退 `idle`。

| 字段 | 类型 | 默认/回退 |
|---|---|---|
| `id` | String | `''` |
| `taskId` | String | `''` |
| `cliSessionId` | String? | null |
| `cli` | CliKind | `pty` |
| `model` | String? | null |
| `status` | SessionStatus | `idle` |
| `createdAt` | DateTime | `DateTime.now()` |
| `endedAt` | DateTime? | null |

**无 copyWith**。

### 5.7 FileEntry

| 字段 | 类型 | 默认/回退 |
|---|---|---|
| `name` | String | `''` |
| `path` | String | `''` |
| `isDir` | bool | `false` |
| `size` | int | `0` |
| `modifiedAt` | DateTime | `DateTime.now()` |

**无 copyWith**。

### 5.8 FileSnapshot 与 RollbackResult

**`FileSnapshot`**：

| 字段 | 类型 | 默认/回退 |
|---|---|---|
| `id` | String | `''` |
| `sessionId` | String | `''` |
| `path` | String | `''` |
| `action` | String | `'edit'`（**注意：这里是裸 String，不是枚举**） |
| `diff` | String? | null |
| `addedLines` | int? | null |
| `removedLines` | int? | null |
| `createdAt` | DateTime | `DateTime.now()` |

只有 `fromJson`，**无 toJson，无 copyWith**。

**`RollbackResult`**：

| 字段 | 类型 | 默认/回退 |
|---|---|---|
| `snapshotId` | String | `''` |
| `path` | String | `''` |
| `action` | String | `'edit'` |
| `rolled` | bool | `false` |
| `method` | String | `'skip'`（取值：`git-checkout` / `git-clean` / `skip`） |
| `message` | String | `''` |

只有 `fromJson`。

### 5.9 UnifiedEvent（核心，sealed union）

**`ContentBlock` sealed class**（消息块）：

| 子类 | type wire | 字段 |
|---|---|---|
| `TextBlock` | `text` | `text: String` |
| `ThinkingBlock` | `thinking` | `text: String`, `signature: String?` |
| `ImageBlock` | `image` | `mimeType: String`, `dataB64: String` |
| `ToolUseBlock` | `tool_use` | `toolUseId: String`, `toolName: String`, `input: Object?` |
| `ToolResultBlock` | `tool_result` | `toolUseId: String`, `output: Object?`, `isError: bool?` |

`ToolResultBlock.outputAsString` getter：String 直接返回；Map 且 `type=='json'` → 缩进 JSON；`type=='image'` → `[图片]`；其它 `toString()`。未知 type → `TextBlock(text: '<未知 block: $type>')`。

**`FileChangeAction` 枚举**：`create` / `edit` / `delete`，默认 `edit`。
**`TodoStatus` 枚举**：`pending` / `inProgress('in_progress')` / `completed`，默认 `pending`。

**`FileChange`**：`path` + `action: FileChangeAction` + `diff: String?` + `addedLines: int?` + `removedLines: int?`。

**`TodoItem`**：`id` + `content` + `status: TodoStatus` + `progress: int?`。**有 copyWith**（唯一一个 model 提供 copyWith 的）。

**`SessionStats`**：`costUsd: double?`、`durationMs: int?`、`numTurns: int?`、`inputTokens: int?`、`outputTokens: int?`。只有 fromJson（无 toJson）。

**`UnifiedEvent` sealed class**：

所有事件基类，含 `final int timestamp` 和 `String get type`。`fromJson` 按 `type` 字符串 switch 分发，未知 type 返回 `ErrorEvent(message: '未知事件类型: $type', recoverable: true)`。timestamp 缺省回退 `DateTime.now().millisecondsSinceEpoch`。

子类（均 `extends UnifiedEvent`，override `type` getter）：

| 类 | type wire | 特有字段 |
|---|---|---|
| `SessionInitEvent` | `session.init` | sessionId, cliSessionId?, cli, model?, cwd |
| `MessageEvent` | `message` | role, blocks: List<ContentBlock> |
| `StreamingDeltaEvent` | `streaming.delta` | messageId, textDelta? |
| `StreamingDoneEvent` | `streaming.done` | messageId |
| `ToolUseEvent` | `tool.use` | toolUseId, toolName, input |
| `ToolResultEvent` | `tool.result` | toolUseId, output: String, isError? |
| `FileChangeEvent` | `file.change` | change: FileChange, toolUseId? |
| `CommandExecEvent` | `command.exec` | command, cwd?, exitCode?, stdout?, stderr?, toolUseId? |
| `TodoUpdateEvent` | `todo.update` | todos: List<TodoItem> |
| `ThinkingEvent` | `thinking` | text |
| `ProgressEvent` | `progress` | step?, total?, message? |
| `ErrorEvent` | `error` | message, recoverable?, cli? |
| `SessionEndEvent` | `session.end` | stats: SessionStats? |

**注意**：`ToolResultEvent.output` 在事件层是 `String`（与 `ToolResultBlock.output` 的 `Object?` 不同，事件层已序列化为字符串）。

### 5.10 ClientCommand（客户端 → 服务端，sealed）

| 类 | kind wire | 字段 |
|---|---|---|
| `SessionStartCommand` | `session.start` | serverId, cwd, cli, prompt, model?, resumeCliSessionId?, allowedTools? |
| `SessionSendCommand` | `session.send` | prompt |
| `SessionInterruptCommand` | `session.interrupt` | — |
| `SessionEndCommand` | `session.end` | — |
| `SessionResyncCommand` | `session.resync` | sinceEventId: String |

`toJson` 序列化为 `{kind: ..., ...fields}`，枚举用 `.wire`，可空字段非 null 才输出。

### 5.11 ServerEnvelope（服务端 → 客户端信封）

```dart
class ServerEnvelope {
  final int eventId;       // 默认 0
  final String sessionId;  // 默认 ''
  final UnifiedEvent event;

  // fromJson 从 j['event'] 解析（cast 为 Map<String, dynamic>）
}
```

### 5.12 ApiKeyMeta

```dart
class ApiKeyMeta {
  final CliKind cli;        // 默认 pty
  final bool hasKey;        // 默认 false
  final String? last4;
  final DateTime? updatedAt;  // DateTime.tryParse
  // 只有 fromJson
}
```

---

## §6 服务层：REST 客户端

### 6.1 Dio 配置

```dart
class ApiClient {
  late final Dio _dio;

  ApiClient(AppConfig config) {
    _dio = Dio(BaseOptions(
      baseUrl: config.baseUrl,
      connectTimeout: const Duration(seconds: 10),
      receiveTimeout: const Duration(seconds: 30),
      headers: {'Accept': 'application/json'},
    ));
    _dio.interceptors.add(LogInterceptor(
      requestBody: false,
      responseBody: false,
      error: true,
    ));
  }

  Dio get dio => _dio;
}
```

- `baseUrl` 来自 `AppConfig.baseUrl`
- 连接超时 10s，接收超时 30s
- 默认 header `Accept: application/json`
- 拦截器：仅 `LogInterceptor`，只记录 error（requestBody/responseBody 都关）

### 6.2 错误处理（ApiException）

```dart
class ApiException implements Exception {
  final int? statusCode;
  final String message;
  ApiException(this.message, {this.statusCode});

  @override
  String toString() => 'ApiException($statusCode): $message';
}

Never _err(DioException e) {
  String msg = e.message ?? '网络错误';
  if (e.response != null) {
    final data = e.response?.data;
    if (data is Map<String, dynamic> && data['error'] != null) {
      msg = data['error'].toString();
    } else if (data is String && data.isNotEmpty) {
      msg = data;
    }
  }
  throw ApiException(msg, statusCode: e.response?.statusCode);
}
```

`_err` 是 `Never` 返回（抛 ApiException），所以方法能声明具体返回类型。

### 6.3 响应解析（_asMap / _asList）

```dart
T _asMap<T>(dynamic data) {
  if (data is! Map<String, dynamic>) {
    throw ApiException('响应格式错误：期望对象，得到 ${data.runtimeType}');
  }
  return data as T;
}

List<Map<String, dynamic>> _asList(dynamic data) {
  if (data is! List) {
    throw ApiException('响应格式错误：期望数组');
  }
  return data.cast<Map<String, dynamic>>();
}
```

### 6.4 所有方法签名与端点

| 方法 | HTTP | 端点 | 返回 |
|---|---|---|---|
| `health()` | GET | `/api/health` | `Map<String, dynamic>` |
| `listServers({String? projectId})` | GET | `/api/servers?projectId=` | `List<Server>` |
| `createServer({name, host, port, username, auth})` | POST | `/api/servers` | `Server` |
| `getServer(String id)` | GET | `/api/servers/$id` | `Server` |
| `updateServer(id, {name, host, port, username, auth})` | PATCH | `/api/servers/$id` | `Server` |
| `deleteServer(String id)` | DELETE | `/api/servers/$id` | void |
| `testServer(String id)` | POST | `/api/servers/$id/test` | `ServerTestResult` |
| `listFiles(serverId, path)` | GET | `/api/servers/$serverId/files?path=` | `FileListing` |
| `readFile(serverId, path)` | GET | `/api/servers/$serverId/files/content?path=` | `FileContent` |
| `writeFile(serverId, path, content)` | PUT | `/api/servers/$serverId/files/content` | `FileContent` |
| `deleteFile(serverId, path)` | DELETE | `/api/servers/$serverId/files` | void |
| `listProjects({String? serverId})` | GET | `/api/projects?serverId=` | `List<Project>` |
| `createProject({serverId, name, cwd, defaultCli, defaultModel, rules})` | POST | `/api/projects` | `Project` |
| `getProject(String id)` | GET | `/api/projects/$id` | `Project` |
| `updateProject(id, Map body)` | PATCH | `/api/projects/$id` | `Project` |
| `deleteProject(String id)` | DELETE | `/api/projects/$id` | void |
| `listTasks(String projectId)` | GET | `/api/projects/$projectId/tasks` | `List<Task>` |
| `createTask({projectId, title, description})` | POST | `/api/tasks` | `Task` |
| `getTask(String id)` | GET | `/api/tasks/$id` | `Task` |
| `updateTask(id, Map body)` | PATCH | `/api/tasks/$id` | `Task` |
| `deleteTask(String id)` | DELETE | `/api/tasks/$id` | void |
| `listSessions(String taskId)` | GET | `/api/tasks/$taskId/sessions` | `List<Session>` |
| `createSession({taskId, cli, model})` | POST | `/api/sessions` | `Session` |
| `getSession(String id)` | GET | `/api/sessions/$id` | `Session` |
| `listSessionEvents(id, {int? since})` | GET | `/api/sessions/$id/events?since=` | `List<ServerEnvelope>`（从 `data['events']` 解析） |
| `deleteSession(String id)` | DELETE | `/api/sessions/$id` | void |
| `listSnapshots(String sessionId)` | GET | `/api/sessions/$sessionId/snapshots` | `List<FileSnapshot>`（从 `data['snapshots']` 解析） |
| `rollbackSnapshot(snapshotId, {serverId, cwd})` | POST | `/api/snapshots/$snapshotId/rollback` | `RollbackResult`（从 `data['result']` 解析） |
| `rollbackSession(sessionId, {serverId, cwd})` | POST | `/api/sessions/$sessionId/rollback` | `List<RollbackResult>`（从 `data['results']` 解析） |
| `listApiKeys()` | GET | `/api/api-keys` | `List<ApiKeyMeta>` |
| `upsertApiKey({cli, key})` | POST | `/api/api-keys` | `ApiKeyMeta` |
| `deleteApiKey(CliKind cli)` | DELETE | `/api/api-keys/${cli.wire}` | void |

**约定**：所有方法用 `try { ... } on DioException catch (e) { _err(e); }`。

---

## §7 服务层：WebSocket 客户端

### 7.1 连接管理

```dart
enum WsConnectionState { disconnected, connecting, connected, reconnecting }

class WsClient {
  WebSocketChannel? _channel;
  StreamSubscription? _socketSub;
  Timer? _heartbeatTimer;
  Timer? _reconnectTimer;
  String? _sessionId;
  int _lastEventId = 0;
  bool _disposed = false;
  bool _initialized = false;  // 是否已收到 session.init

  final _stateController = StreamController<WsConnectionState>.broadcast();
  final _envelopeController = StreamController<ServerEnvelope>.broadcast();

  Stream<WsConnectionState> get stateStream => _stateController.stream;
  Stream<ServerEnvelope> get envelopeStream => _envelopeController.stream;
}
```

### 7.2 状态机

```
disconnected → connecting (connect 调用) → connected (_doConnect 成功)
                                          ↓
                                       断线
                                          ↓
                              reconnecting (_scheduleReconnect)
                                          ↓
                              3s 后 → connecting (_doConnect)
                                          ↓
                              成功 connected / 失败再次 reconnecting
```

dispose 后强制 `disconnected`，不再转换。

### 7.3 connect 方法

```dart
Future<void> connect(String sessionId) async {
  // 清理旧连接
  _reconnectTimer?.cancel();
  _stopHeartbeat();
  _socketSub?.cancel();
  await _channel?.sink.close();

  _initialized = false;  // 保留 _lastEventId（不重置）
  _sessionId = sessionId;
  _state = WsConnectionState.connecting;
  _doConnect();
}
```

**关键设计**：切换会话时**不清零** `_lastEventId`（依赖服务端 eventId 全局递增）。

### 7.4 重连策略

固定 3 秒间隔，无指数退避：

```dart
void _scheduleReconnect() {
  if (_disposed) return;
  _state = WsConnectionState.reconnecting;
  _reconnectTimer = Timer(const Duration(seconds: 3), () {
    if (!_disposed) _doConnect();
  });
}
```

触发时机：`_onError`、`_onDone`（且未 dispose）、`_doConnect` 的 try-catch。

重连后若 `_initialized && _lastEventId > 0`，发 `SessionResyncCommand(sinceEventId: '$_lastEventId')` 请求增量。

### 7.5 心跳机制

应用层 ping，15 秒一次（同时协议层 pingInterval 20s）：

```dart
void _startHeartbeat() {
  _stopHeartbeat();
  _heartbeatTimer = Timer.periodic(const Duration(seconds: 15), (_) {
    _sendRaw({'type': 'ping'});
  });
}
```

双层心跳：协议层 20s + 应用层 15s。服务端回 `{"type":"pong"}` 或 `{"type":"ping"}`，`_onData` 中直接忽略。

### 7.6 消息收发

**发送**：
```dart
void _sendRaw(Map payload) {
  _channel?.sink.add(jsonEncode(payload));
}

void sendCommand(ClientCommand cmd) => _sendRaw(cmd.toJson());
```

**接收** `_onData(dynamic data)`：
1. 非 String 直接 return
2. `jsonDecode` 失败 return
3. `type == 'pong' || type == 'ping'` return（心跳）
4. `ServerEnvelope.fromJson(json)` 解析
5. 若 `env.eventId > _lastEventId` → 更新 `_lastEventId`
6. 若 `env.event is SessionInitEvent` → `_initialized = true`
7. `_envelopeController.add(env)` 推给订阅者

### 7.7 dispose 与 close

```dart
void dispose() {
  _disposed = true;
  _reconnectTimer?.cancel();
  _stopHeartbeat();
  _socketSub?.cancel();
  _channel?.sink.close();
  _state = WsConnectionState.disconnected;
}

void close() {
  dispose();
  _stateController.close();
  _envelopeController.close();
}
```

- `dispose()`：主动断开，不再重连
- `close()`：dispose + 关闭两个 StreamController（页面销毁时调用）

### 7.8 WebSocket 工厂

```dart
WebSocketChannel buildWebSocketChannel(Uri uri) {
  if (Platform.isAndroid || Platform.isIOS || Platform.isMacOS ||
      Platform.isLinux || Platform.isWindows) {
    return IOWebSocketChannel.connect(uri, pingInterval: const Duration(seconds: 20));
  }
  return WebSocketChannel.connect(uri);
}
```

移动端用 `IOWebSocketChannel`（更稳定）。

---

## §8 服务层：本地存储

### 8.1 Storage 类

```dart
class Storage {
  final SharedPreferences _sp;
  Storage(this._sp);

  static const _kBaseUrl = 'backend.baseUrl';
  static const _kLastServerId = 'last.serverId';
  static const _kLastProjectId = 'last.projectId';

  Future<String> getBaseUrl() async => _sp.getString(_kBaseUrl) ?? '';
  Future<void> setBaseUrl(String url) => _sp.setString(_kBaseUrl, url);

  Future<String?> getLastServerId() async => _sp.getString(_kLastServerId);
  Future<void> setLastServerId(String? id) async {
    if (id == null) {
      await _sp.remove(_kLastServerId);
    } else {
      await _sp.setString(_kLastServerId, id);
    }
  }

  Future<String?> getLastProjectId() async => _sp.getString(_kLastProjectId);
  Future<void> setLastProjectId(String? id) async {
    if (id == null) {
      await _sp.remove(_kLastProjectId);
    } else {
      await _sp.setString(_kLastProjectId, id);
    }
  }
}
```

### 8.2 键名表

| 键名 | 类型 | 含义 | 默认 |
|---|---|---|---|
| `backend.baseUrl` | String | 后端 REST/WS 根地址 | `''`（空，回退 defaultBaseUrl） |
| `last.serverId` | String | 最近选中的服务器 id | null |
| `last.projectId` | String | 最近选中的项目 id | null |

### 8.3 AppConfig

```dart
class AppConfig {
  final String baseUrl;
  final String wsUrl;

  const AppConfig({required this.baseUrl, required this.wsUrl});

  factory AppConfig.fromBaseUrl(String baseUrl) {
    final ws = baseUrl
        .replaceFirst('https://', 'wss://')
        .replaceFirst('http://', 'ws://');
    return AppConfig(baseUrl: baseUrl, wsUrl: '$ws/ws');
  }

  static const defaultBaseUrl = 'http://localhost:8787';
}
```

---

## §9 状态管理层（Riverpod）

### 9.1 Provider 总览

| Provider 名称 | 类型 | 用途 |
|---|---|---|
| `storageProvider` | `Provider<Storage>` | SharedPreferences 包装单例 |
| `configProvider` | `StateProvider<AppConfig>` | 当前后端配置 |
| `initConfigProvider` | `FutureProvider<void>` | 启动时从存储读取 baseUrl 写入 configProvider |
| `apiClientProvider` | `Provider<ApiClient>` | Dio REST 客户端单例（config 变更自动重建） |
| `wsClientProvider` | `Provider<WsClient>` | WebSocket 客户端单例（config 变更自动重建） |
| `serversProvider` | `FutureProvider.family<List<Server>, String?>` | 服务器列表（参数 projectId，null=全部） |
| `projectsProvider` | `FutureProvider.family<List<Project>, String?>` | 项目列表（参数 serverId，null=全部） |
| `tasksProvider` | `FutureProvider.family<List<Task>, String>` | 某项目下任务列表 |
| `serverProvider` | `FutureProvider.family<Server, String>` | 单个服务器 |
| `projectProvider` | `FutureProvider.family<Project, String>` | 单个项目 |
| `taskProvider` | `FutureProvider.family<Task, String>` | 单个任务 |
| `apiKeysProvider` | `FutureProvider<List<ApiKeyMeta>>` | API Key 列表 |
| `dataVersionProvider` | `StateProvider<int>` | 配置变更后刷新列表的机制 |
| `sessionControllerProvider` | `StateNotifierProvider.autoDispose<SessionController, SessionControllerState>` | 会话状态控制器 |

### 9.2 关键实现

```dart
final storageProvider = Provider<Storage>((ref) {
  // 需要在 main 中先 SharedPreferences.getInstance() 后覆写
  throw UnimplementedError();
});

final configProvider = StateProvider<AppConfig>((ref) {
  return AppConfig.fromBaseUrl(AppConfig.defaultBaseUrl);
});

final initConfigProvider = FutureProvider<void>((ref) async {
  final storage = ref.read(storageProvider);
  final url = await storage.getBaseUrl();
  final cfg = AppConfig.fromBaseUrl(url.isEmpty ? AppConfig.defaultBaseUrl : url);
  ref.read(configProvider.notifier).state = cfg;
});

final apiClientProvider = Provider<ApiClient>((ref) {
  final config = ref.watch(configProvider);
  final client = ApiClient(config);
  ref.onDispose(client.dio.close);
  return client;
});

final wsClientProvider = Provider<WsClient>((ref) {
  final config = ref.watch(configProvider);
  final client = WsClient(config);
  ref.onDispose(() => client.close());
  return client;
});

final serversProvider =
    FutureProvider.family<List<Server>, String?>((ref, projectId) async {
  final api = ref.watch(apiClientProvider);
  return api.listServers(projectId: projectId);
});

// ... 其他 family provider 同理

final dataVersionProvider = StateProvider<int>((ref) => 0);

void bumpDataVersion(WidgetRef ref) {
  ref.read(dataVersionProvider.notifier).state++;
}
```

### 9.3 autoDispose 使用场景

- **`sessionControllerProvider`**：唯一显式加 `autoDispose`。会话页面离开时自动销毁 `SessionController`，触发 `dispose()` 取消 WS 订阅并断开 WS（保证后台不留长连接）
- `apiClientProvider` / `wsClientProvider` **没有** `autoDispose`（全局共享单例），通过 `ref.watch(configProvider)` 在配置变更时自动重建
- 各 `FutureProvider.family` **没有** `autoDispose`，列表数据会缓存。需要刷新时通过 `dataVersionProvider` 机制或 `ref.invalidate(provider(args))` 手动失效

### 9.4 dataVersionProvider 刷新机制

```dart
void bumpDataVersion(WidgetRef ref) {
  ref.read(dataVersionProvider.notifier).state++;
}
```

**约定用法**：在任何写操作（create/update/delete）后调用 `bumpDataVersion(ref)`。列表页面需自行 watch `dataVersionProvider` 并在变化时 invalidate 对应 family provider。配置变更（baseUrl）则通过 `configProvider` 驱动 `apiClientProvider`/`wsClientProvider` 重建。

---

## §10 会话状态控制器

### 10.1 State 结构

```dart
class SessionControllerState {
  final String? sessionId;
  final SessionStatus status;  // idle | running | ended | error
  final CliKind? cli;
  final String? model;
  final String? cwd;
  final List<ChatMessage> messages;
  final List<ToolCall> toolCalls;
  final List<FileChange> fileChanges;
  final List<CommandExecEvent> commandLogs;
  final List<TodoItem> todos;
  final ProgressEvent? progress;
  final String? errorMessage;
  final WsConnectionState wsState;
  final SessionStats? stats;

  // copyWith 支持 clearError: true 显式清空错误
  SessionControllerState copyWith({...});
}
```

**辅助数据类**：

```dart
class ChatMessage {
  final String role;  // 'user' | 'assistant'
  final List<ContentBlock> blocks;
  final int timestamp;
  ChatMessage copyWith({List<ContentBlock>? blocks});
}

class ToolCall {
  final String toolUseId;
  final String toolName;
  final Object? input;
  final String? result;
  final bool isError;
  final bool done;
  final int timestamp;
  ToolCall copyWith({String? result, bool? isError, bool? done});
}
```

### 10.2 初始化流程

构造函数调用 `_init()`，订阅两条 WS 流：

```dart
void _init() {
  _stateSub = _ws.stateStream.listen((s) {
    state = state.copyWith(wsState: s);
  });
  _envSub = _ws.envelopeStream.listen(_onEnvelope);
}
```

四种启动入口：

#### (a) startSessionRaw（先 REST 创建 session，再 WS 连接 + 发 session.start）

1. 重置 state 为 `running` + `connecting`，记录 cwd/cli/model
2. `await _api.createSession(taskId, cli, model)` —— REST 创建 session 记录
3. `state.copyWith(sessionId: session.id)`
4. `await _ws.connect(session.id)`
5. `_ws.sendCommand(SessionStartCommand(serverId, cwd, cli, prompt, model, resumeCliSessionId))`
6. catch：`status = error`，`errorMessage = e.toString()`

#### (b) startSessionWithTask（带 Project 对象的便捷版本）

从 `Project` 取 `cwd` 和 `defaultCli`、`defaultModel`，转调 `startSessionRaw`。

#### (c) startExistingSession（session 记录已由上层 REST 创建，这里只连 WS + 发首条指令）

1. state 已知 `sessionId`，置 `running` + `connecting`
2. `await _ws.connect(sessionId)`
3. `_ws.sendCommand(SessionStartCommand(...))`
4. catch 同上

**对应路由场景**：`SessionScreen` 在 `_isStartMode`（`sessionId == 'new'` 或 `startPrompt != null`）时调用本方法。

#### (d) resumeSession（恢复已有会话）

1. state 置 `running` + `connecting`
2. `final envelopes = await _api.listSessionEvents(sessionId)` —— 拉历史事件
3. `for (final env in envelopes) _onEnvelope(env, replay: true)` —— **回放**历史事件
4. `await _ws.connect(sessionId)` —— 连 WS 接收增量

### 10.3 WS 事件分发逻辑（_onEnvelope）

通过 Dart 3 的 `switch` 模式匹配按事件类型分支：

| 事件类型 | 处理 |
|---|---|
| `SessionInitEvent` | 更新 sessionId/cli/model/cwd，status=running |
| `MessageEvent` | 构造 `ChatMessage`，追加到 `messages` |
| `StreamingDeltaEvent` | 调 `_applyDelta`（按 messageId 聚合增量） |
| `StreamingDoneEvent` | 移除 `_streamingIndex[messageId]`，status=idle |
| `ToolUseEvent` | 新建 `ToolCall(done:false)` 追加到 `toolCalls` |
| `ToolResultEvent` | 在 `toolCalls` 中按 `toolUseId` 配对，更新 result/isError/done=true |
| `FileChangeEvent` | 追加 `event.change` 到 `fileChanges` |
| `CommandExecEvent` | 追加到 `commandLogs` |
| `TodoUpdateEvent` | 整体替换 `todos` 为 `event.todos` |
| `ThinkingEvent` | 作为一条 assistant 消息（含 `ThinkingBlock`）追加到 `messages` |
| `ProgressEvent` | 替换 `state.progress` |
| `ErrorEvent` | 设 `errorMessage`；若 `recoverable == false` 则 status=error，否则保持原状态 |
| `SessionEndEvent` | status=ended，存 `stats` |

### 10.4 流式增量聚合算法（_applyDelta）

```dart
final Map<String, int> _streamingIndex = {};  // messageId → messages 列表下标

void _applyDelta(StreamingDeltaEvent e) {
  final delta = e.textDelta;
  if (delta == null || delta.isEmpty) return;

  final idx = _streamingIndex[e.messageId];
  if (idx == null || idx >= state.messages.length) {
    // 新建一条 assistant 消息
    final msg = ChatMessage(
      role: 'assistant',
      blocks: [TextBlock(text: delta)],
      timestamp: e.timestamp,
    );
    _streamingIndex[e.messageId] = state.messages.length;
    state = state.copyWith(messages: [...state.messages, msg]);
    return;
  }

  // 已存在，追加到 TextBlock
  final msg = state.messages[idx];
  final blocks = [...msg.blocks];
  final last = blocks.isNotEmpty ? blocks.last : null;
  if (last is TextBlock) {
    blocks[blocks.length - 1] = TextBlock(text: last.text + delta);
  } else {
    blocks.add(TextBlock(text: delta));
  }
  final newMsg = msg.copyWith(blocks: blocks);
  state = state.copyWith(messages: [...state.messages]..[idx] = newMsg);
}
```

### 10.5 用户操作

```dart
void sendMessage(String prompt) {
  // 先立即把 user 消息加到 messages（体验优化）
  final userMsg = ChatMessage(
    role: 'user',
    blocks: [TextBlock(text: prompt)],
    timestamp: DateTime.now().millisecondsSinceEpoch,
  );
  state = state.copyWith(
    messages: [...state.messages, userMsg],
    status: SessionStatus.running,
    clearError: true,
  );
  _ws.sendCommand(SessionSendCommand(prompt));
}

void interruptSession() {
  _ws.sendCommand(const SessionInterruptCommand());
}

void endSession() {
  _ws.sendCommand(const SessionEndCommand());
}
```

### 10.6 状态机转换

- 启动时 → `running`
- 收到 `StreamingDoneEvent` → `idle`（一轮流式结束）
- `sendMessage` → `running`（新一轮开始）
- 收到 `ErrorEvent` 且 `recoverable == false` → `error`
- 启动失败（catch） → `error`
- 收到 `SessionEndEvent` → `ended`

### 10.7 dispose 清理逻辑

```dart
@override
void dispose() {
  _envSub?.cancel();
  _stateSub?.cancel();
  _ws.dispose();  // 主动断开 WS（WsClient 是共享单例，dispose 仅断连不释放流）
  super.dispose();
}
```

**注意**：`WsClient` 是共享单例，`dispose()` 只断连不关闭流；`close()` 才关闭 StreamController。`autoDispose` 保证离开 SessionScreen 自动触发。

---

## §11 路由配置

### 11.1 完整路由表

| 路径 | 屏幕 | 参数来源 |
|---|---|---|
| `/` | `SplashScreen` | — |
| `/settings` | `SettingsScreen` | — |
| `/servers` | `ServersScreen` | — |
| `/servers/new` | `ServerFormScreen(serverId: null)` | — |
| `/servers/:id/edit` | `ServerFormScreen(serverId: id)` | path `id` |
| `/servers/:id/files` | `FilesScreen(serverId, initialPath)` | path `id` + query `path` |
| `/servers/:id/projects` | `ProjectsScreen(serverId)` | path `id` |
| `/servers/:id/projects/new` | `ProjectFormScreen(serverId)` | path `id` |
| `/projects/:id/tasks` | `TasksScreen(projectId)` | path `id` |
| `/projects/:id/tasks/new` | `TaskFormScreen(projectId)` | path `id` |
| `/tasks/:id/sessions` | `SessionListScreen(taskId)` | path `id` |
| `/sessions/:id` | `SessionScreen` | path `id` + 多个 query |

错误处理：`errorBuilder` 返回 `Scaffold` + AppBar "路由错误" + 显示 `state.error`。

### 11.2 /sessions/:id 路由的 query 参数

```dart
GoRoute(
  path: '/sessions/:id',
  builder: (context, state) => SessionScreen(
    sessionId: state.pathParameters['id']!,
    taskId: state.uri.queryParameters['taskId'],
    cliWire: state.uri.queryParameters['cli'],
    cwd: state.uri.queryParameters['cwd'],
    model: state.uri.queryParameters['model'],
    serverId: state.uri.queryParameters['serverId'],
    startPrompt: state.uri.queryParameters['prompt'],
  ),
),
```

- `sessionId`：path 参数，必填。特殊值 `'new'` 表示新建会话模式
- `taskId` / `cli` / `cwd` / `model` / `serverId` / `prompt`：均为可选 query
- 当 `sessionId == 'new'` 或 `startPrompt != null` 时进入"新建会话模式"（`_isStartMode`），调用 `ctrl.startExistingSession(...)`
- 否则进入"恢复模式"，调用 `ctrl.resumeSession(...)`

### 11.3 路由跳转方式

- `context.go('/servers')` / `context.go('/settings')`：**栈根替换**。用于 SplashScreen 启动校验后跳转、SettingsScreen 保存后跳转
- `context.push('/...')`：**入栈**。绝大多数导航都用 push（如 servers→projects→tasks→sessions 逐层入栈，新建表单也用 push）
- `context.pop()`：**出栈**。表单保存成功后返回
- 没有使用 `context.replace`

### 11.4 路由守卫（splash 校验）

`SplashScreen` 是 `initialLocation: '/'`，在 `initState` 调用 `_bootstrap()`：

1. `await ref.read(initConfigProvider.future)` —— 等待配置从存储加载
2. `await api.health()` —— 调用 `/api/health` 健康检查
3. 成功：`context.go('/servers')`（进入主界面）
4. 失败（catch）：`context.go('/settings')`（让用户配置后端地址）

UI 上显示 indigo 渐变背景 + 终端图标 + LoadingIndicator（显示 "正在连接后端…" 或 "连接 {baseUrl}…"）。

### 11.5 routerProvider 实现

```dart
final routerProvider = Provider<GoRouter>((ref) {
  return GoRouter(
    initialLocation: '/',
    routes: [
      GoRoute(path: '/', builder: (c, s) => const SplashScreen()),
      GoRoute(path: '/settings', builder: (c, s) => const SettingsScreen()),
      GoRoute(path: '/servers', builder: (c, s) => const ServersScreen()),
      GoRoute(path: '/servers/new', builder: (c, s) => const ServerFormScreen(serverId: null)),
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
      // ... 其他路由
    ],
    errorBuilder: (c, s) => Scaffold(
      appBar: AppBar(title: const Text('路由错误')),
      body: Center(child: Text(s.error?.toString() ?? '未知路由')),
    ),
  );
});
```

---

## §12 屏幕规范

### 12.1 启动页 SplashScreen

- **路由**：`/`
- **AppBar**：无
- **主体布局**：
  - `Container` 全屏渐变背景：`LinearGradient(topCenter → bottomCenter, primary → primaryContainer)`
  - `Column(mainAxisAlignment: center)`：
    1. `Icon(Icons.terminal, size: 80, color: onPrimary)`
    2. `SizedBox(height: 16)`
    3. `Text('HeyCode', headlineMedium + bold + onPrimary)`
    4. `SizedBox(height: 32)`
    5. `LoadingIndicator(label: ...)`
- **加载状态文案**：`baseUrl` 为空 → `'正在连接后端…'`；否则 → `'连接 ${baseUrl}…'`
- **关键交互**：无交互，纯自动引导（`initState` 中 `_bootstrap()`）

### 12.2 设置页 SettingsScreen

- **路由**：`/settings`
- **AppBar**：`title: Text('设置')`、`leading: IconButton(Icons.arrow_back)` → `context.go('/servers')`
- **主体布局**：`ListView(padding: EdgeInsets.all(16))`，从上到下：
  1. 标题 `Text('后端地址', titleMedium)`
  2. `TextField`：hint `'http://localhost:8787'`、`OutlineInputBorder`、`prefixIcon: Icon(Icons.dns)`、`keyboardType: url`、`autocorrect: false`
  3. 按钮行：
     - `FilledButton.icon(Icons.save, '保存')` → `_save()`
     - `OutlinedButton.icon(Icons.wifi_find, '测试连接')` → `_test()`
  4. （可选）`_testMsg` 文本，颜色根据是否以"连接成功"开头切换：成功用 `0xFF2E7D32`，失败用 `colorScheme.error`
  5. `Divider(height: 40)`
  6. API Key 管理标题行：`Text('API Key 管理', titleMedium)` + 右侧 `IconButton(Icons.refresh)` 刷新
  7. `keysAsync.when(...)` 渲染 API Key 列表
- **API Key 列表项**（`_keyList`）：
  - 遍历所有 `CliKind.values`（排除 `pty`），保证每个 CLI 都列出
  - `Card(margin: symmetric(vertical: 4))` + `ListTile`：
    - `leading: Icon(Icons.key, color: primary)`
    - `title: Text(cli.wire)`
    - `subtitle`：未设置 → `'未设置'`；已设置 → `'已设置 ····${last4}'`
    - `trailing`：`IconButton(Icons.edit)` + （已设置时）`IconButton(Icons.delete_outline)`
- **关键交互**：
  - 保存：写入 storage，更新 `configProvider`，SnackBar `'已保存'`
  - 测试连接：先保存再 `health()`，显示版本号
  - 编辑 Key：弹 `AlertDialog`（`TextField(obscureText: true)`），保存后 `ref.invalidate(apiKeysProvider)`
  - 删除 Key：弹确认 `AlertDialog`（`FilledButton.tonal('删除')`）
- **状态展示**：
  - loading：`SizedBox(height: 200, child: LoadingIndicator(label: '加载 API Key…'))`
  - error：`Text('加载失败: $e', color: error)`

### 12.3 服务器列表 ServersScreen

- **路由**：`/servers`
- **AppBar**：`title: Text('服务器')`、`actions: [IconButton(Icons.settings) → context.push('/settings')]`
- **FAB**：`FloatingActionButton.extended(Icons.add, '新增服务器')` → `/servers/new`
- **主体**：`serversAsync.when(data/loading/error)`
  - data + 空：`EmptyState(icon: dns_outlined, title: '还没有服务器', subtitle: '点击右下角添加你的第一台 SSH 服务器', action: FilledButton.icon(add, '新增服务器'))`
  - data + 非空：`RefreshIndicator` 包 `ListView.builder(padding: all(12))`
  - loading：`LoadingIndicator()`
  - error：`ErrorView(message, onRetry: ref.invalidate)`
- **服务器卡片 `_ServerCard`**：
  - `Card(margin: symmetric(vertical: 6))` + `InkWell(borderRadius: 12)`
  - `Row`：
    - `CircleAvatar(primaryContainer)` + `Icon(Icons.dns, primary)`
    - `Expanded(Column)`：
      - `Text(name, titleMedium)`
      - `Text('${username}@${host}:${port}', bodySmall + outline + monospace)`
      - `Row`：状态点（`Icon(Icons.circle, size: 8)`）+ 状态文案 + `Text(authKind.wire, labelSmall)`
    - `PopupMenuButton<String>`：编辑 / 文件浏览 / 删除
- **状态颜色**：`ok → 0xFF2E7D32` / `fail → 0xFFC62828` / `unknown → outline`
- **关键交互**：
  - 点击卡片：`setLastServerId` 后 `context.push('/servers/${id}/projects')`
  - PopupMenu `edit` → `/servers/${id}/edit`
  - PopupMenu `files` → `setLastServerId` + `/servers/${id}/files`
  - PopupMenu `delete` → 弹确认 `AlertDialog`，确认后调 `deleteServer` + `bumpDataVersion` + `ref.invalidate`

### 12.4 服务器表单 ServerFormScreen

- **路由**：新建 `/servers/new`（`serverId == null`）；编辑 `/servers/:id/edit`
- **AppBar**：`title: Text(_isEdit ? '编辑服务器' : '新增服务器')`
- **主体**：`Form` + `ListView(padding: all(16))`
- **字段**（全部 `OutlineInputBorder` + 前缀 Icon）：
  1. 名称 `TextFormField(Icons.label)`，validator 必填
  2. 主机地址 `TextFormField(Icons.dns)`，必填
  3. 端口 `TextFormField(Icons.numbers, keyboardType: number)`，默认 `'22'`
  4. 用户名 `TextFormField(Icons.person)`，必填
  5. 认证方式标题 `Text('认证方式', titleSmall)`
  6. `SegmentedButton<SshAuthKind>`：密码 / 私钥 / Agent
  7. 根据选择切换：
     - `password` → `TextFormField(Icons.lock, obscureText: true, hintText: '编辑时留空表示不修改')`
     - `privateKey` → `TextFormField(Icons.vpn_key, maxLines: 5)` + `TextFormField(Icons.password, obscureText: true)` (Passphrase)
     - `agent` → `Text('使用本机 SSH Agent 转发认证。')`
  8. `Row`：
     - `Expanded(FilledButton.icon(Icons.save, _isEdit ? '保存修改' : '创建'))`
     - 仅编辑模式：`OutlinedButton.icon(Icons.electrical_services, '测试')`
- **校验规则**：
  - 名称、主机、用户名必填（`v.trim().isEmpty ? '必填' : null`）
  - 端口用 `int.tryParse ?? 22` 兜底
  - 新建模式必须填认证；编辑模式留空表示不修改
- **关键交互**：
  - 编辑模式 `initState` 调 `_load()` 拉取 server 详情回填
  - 测试：先保存才能测试，调 `testServer(id)`，SnackBar 显示 `连接成功，延迟 ${latencyMs} ms` 或失败原因

### 12.5 文件浏览 FilesScreen

- **路由**：`/servers/:id/files?path=...`
- **AppBar**：
  - `leading: IconButton(Icons.arrow_back)`：先尝试目录栈回退（`_back()`），栈空则 `context.pop()`
  - `title: Text(_cwd, maxLines: 1, overflow: ellipsis)`
  - `actions: [IconButton(Icons.refresh) → _load()]`
- **主体**：
  - `_loading` → `LoadingIndicator()`
  - `_error != null` → `ErrorView(onRetry: _load)`
  - 空目录 → `EmptyState(icon: folder_open, title: '空目录')`
  - 列表 → `ListView.builder`，每项 `ListTile`：
    - `leading: Icon(folder | insert_drive_file)`，目录用 `primary` 色
    - `title: Text(name)`
    - `subtitle`（文件）：`Text(_formatSize(size), labelSmall)`
    - `onTap: _enter(e)`
- **关键交互**：
  - 进入目录：`_stack.push(_cwd)` → `_cwd = e.path` → `_load()`
  - 点击文件：`Navigator.push` 到内嵌 `_FileEditorScreen`
  - 返回键优先回退目录栈
- **文件大小格式**：`<1024 → 'N B'`、`<1024*1024 → 'X.X KB'`、否则 `'X.X MB'`
- **内嵌编辑器 `_FileEditorScreen`**：
  - AppBar：`title: Text(basename)`、`actions: IconButton(Icons.save | CircularProgressIndicator)`
  - 主体：`TextField(maxLines: null, expands: true, style: monospace fontSize 13)`
  - 保存：调 `writeFile`，SnackBar `'已保存'`

### 12.6 项目列表 ProjectsScreen

- **路由**：`/servers/:id/projects`
- **AppBar**：
  - `title: Text('项目')`
  - `actions: [IconButton(Icons.folder_open, tooltip: '文件浏览') → /servers/$serverId/files]`
- **FAB**：`FloatingActionButton.extended(Icons.add, '新增项目')` → `/servers/$serverId/projects/new`
- **主体**：`projectsAsync.when(...)` 同服务器列表
  - 空：`EmptyState(icon: folder_off_outlined, title: '还没有项目', subtitle: '为这台服务器添加一个工作目录')`（无 action 按钮）
- **项目卡片 `_ProjectCard`**：
  - `Card` + `InkWell(borderRadius: 12)`
  - `Row`：
    - `CircleAvatar(secondaryContainer)` + `Icon(Icons.folder, onSecondaryContainer)`
    - `Expanded(Column)`：name (titleMedium) + cwd (bodySmall + monospace + ellipsis) + `Wrap` chips：
      - `defaultCli.wire`（primary 色 chip）
      - `defaultModel`（tertiary 色 chip，可选）
    - `IconButton(Icons.chevron_right)`
- **Chip 样式**：`Container(padding: horizontal 8 + vertical 2, color: color.withOpacity(0.12), radius: 12)`，文字 `fontSize: 11, fontWeight: w600`
- **关键交互**：点击卡片 / chevron：`setLastProjectId` + `context.push('/projects/${id}/tasks')`

### 12.7 项目表单 ProjectFormScreen

- **路由**：`/servers/:id/projects/new`（仅新建，无编辑）
- **AppBar**：`title: Text('新增项目')`
- **主体**：`Form` + `ListView(padding: all(16))`
- **字段**：
  1. 项目名称 `TextFormField(Icons.label)`，必填
  2. 工作目录 `TextFormField(Icons.folder, hintText: '/home/user/project')`，必填；`initState` 调 `_loadServer()` 默认填 `/home/${username}`
  3. 默认 CLI 标题 `Text('默认 CLI', titleSmall)`
  4. `DropdownButtonFormField<CliKind>(Icons.smart_toy)`：列出所有 `CliKind.values`，默认 `claudeCode`
  5. 默认模型 `TextFormField(Icons.memory, hintText: 'claude-sonnet-4-5 / gpt-4o / ...')`，可选
  6. 规则文件内容 `TextFormField(Icons.rule, maxLines: 5, hintText: 'AGENTS.md / .claude/rules 等')`，可选
  7. `FilledButton.icon(Icons.save, '创建项目')`

### 12.8 任务列表 TasksScreen

- **路由**：`/projects/:id/tasks`
- **AppBar**：
  - `title`：`projectAsync.when` 显示项目名（loading/error 时显示 `'任务'`）
  - `actions: [IconButton(Icons.add, tooltip: '新建任务') → /projects/$projectId/tasks/new]`
- **FAB**：无（用 AppBar 右上角 + 按钮）
- **主体**：`tasksAsync.when(...)`
  - 空：`EmptyState(icon: task_alt, title: '还没有任务', subtitle: '为这个项目创建一个任务', action: FilledButton.icon(add, '新建任务'))`
- **任务卡片 `_TaskCard`**：
  - `Card` + `InkWell(borderRadius: 12)`
  - `Row`：
    - `Icon(Icons.task, color: statusColor)`
    - `Expanded(Column)`：
      - title (titleMedium)
      - description（最多 2 行，省略号，bodySmall + outline，可选）
      - `Row`：状态点 + 状态文案
    - `Icon(Icons.chevron_right)`
- **状态颜色**：
  - `planned → outline`（已规划）
  - `inProgress → primary`（进行中）
  - `done → 0xFF2E7D32`（已完成）
  - `archived → outline`（已归档）
- **关键交互**：点击卡片 → `context.push('/tasks/${id}/sessions')`

### 12.9 任务表单 TaskFormScreen

- **路由**：`/projects/:id/tasks/new`
- **AppBar**：`title: Text('新建任务')`
- **主体**：`Form` + `ListView(padding: all(16))`
- **字段**：
  1. 标题 `TextFormField(Icons.title)`，必填
  2. 描述 `TextFormField(Icons.description, maxLines: 5)`，可选（空则传 `null`）
  3. `FilledButton.icon(Icons.save, '创建任务')`

### 12.10 会话列表 SessionListScreen

- **路由**：`/tasks/:id/sessions`
- **AppBar**：
  - `title: Text('会话')`
  - `actions: [IconButton(Icons.refresh) → ref.invalidate(_provider)]`
- **FAB**：`FloatingActionButton.extended(Icons.play_arrow, '开始新会话')` → `_startNewSession`
- **主体**：`sessionsAsync.when(...)`
  - 空：`EmptyState(icon: chat_bubble_outline, title: '还没有会话', subtitle: '点击右下角开始一个新会话')`（无 action）
- **会话卡片 `_SessionCard`**：
  - `Card` + `InkWell(borderRadius: 12)`
  - `Row`：
    - `Icon(Icons.chat, color: statusColor)`
    - `Expanded(Column)`：cli.wire (titleMedium) + model/'默认模型' (bodySmall + outline) + createdAt (labelSmall + outline，格式 `YYYY-MM-DD HH:mm`)
    - `Column(end)`：状态点 + 状态文案 (fontSize 11)
    - `Icon(Icons.chevron_right)`
- **状态颜色**：`running → primary` / `idle → tertiary` / `ended → outline` / `error → error`
- **状态文案**：运行中 / 空闲 / 已结束 / 出错
- **关键交互**：
  - 点击卡片 → `context.push('/sessions/${id}?taskId=...&cli=...&model=...')`
  - 开始新会话：
    1. 弹 `AlertDialog`：`TextField(autofocus, maxLines: 4, hintText: '告诉 AI 要做什么…')`，按钮 `TextButton('取消')` + `FilledButton('开始')`
    2. 取首条 prompt → 查 task → 查 project → `createSession(taskId, cli: project.defaultCli, model: project.defaultModel)`
    3. `ref.invalidate(_provider)` 后跳 `/sessions/${session.id}?taskId=...&cli=...&cwd=...&serverId=...&model=...&prompt=...`

---

## §13 会话页详细规范

### 13.1 整体布局

```
Scaffold
├─ AppBar
│  ├─ title: Column
│  │  ├─ Text(cli.wire 或 '会话')           // 主标题
│  │  └─ Row: 状态点 + 状态文案 + ('· ${model}') // labelSmall
│  ├─ actions: [运行中显示 IconButton(Icons.stop_circle_outlined, tooltip: '中断')]
│  └─ bottom: TabBar(isScrollable: true, length: 5)
├─ body: Column
│  ├─ (条件) 重连横幅 _reconnectingBanner
│  ├─ (条件) 错误横幅 _errorBanner
│  ├─ Expanded(TabBarView)
│  │  ├─ 消息面板
│  │  ├─ 工具面板
│  │  ├─ 文件面板
│  │  ├─ 命令面板
│  │  └─ 进度面板
│  └─ (二选一)
│     ├─ ended: SafeArea 顶部不撑，Container(surfaceContainerHighest, padding: all(12))
│     │        内 Row(center): Icon(flag_outlined) + Text('会话已结束') + (可选 stats 行)
│     └─ !ended: SessionInputBar(running, onSend, onInterrupt)
```

- **路由**：`/sessions/:id?taskId=&cli=&cwd=&model=&serverId=&prompt=`
- **状态来源**：`ref.watch(sessionControllerProvider)`（`StateNotifierProvider.autoDispose`）
- **混入**：`TickerProviderStateMixin`（用于 TabController）

### 13.2 AppBar 设计

- `title`：`Column(crossAxisAlignment: start)`
  - 第一行：`Text(state.cli?.wire ?? (widget.cliWire ?? '会话'))`
  - 第二行：`Row`：
    - `_statusDot(state)` —— `Icon(Icons.circle, size: 8, color: ...)`：
      - `running → 0xFF2E7D32` / `idle → tertiary` / `ended → outline` / `error → error`
    - `SizedBox(width: 6)`
    - `Text(_statusLabel(state), labelSmall)`：运行中 / 空闲 / 已结束 / 出错
    - 若 `model != null`：`SizedBox(width: 8)` + `Text('· ${model}', labelSmall)`
- `actions`：仅 `running` 时显示 `IconButton(Icons.stop_circle_outlined, tooltip: '中断')` → `interruptSession()`
- `bottom`：`TabBar(controller, isScrollable: true, tabs: [...])`

### 13.3 5 个 Tab 的内容与切换逻辑

`TabController(length: 5, vsync: this)`。Tab 文案带计数（"进度" Tab 除外）：

| Tab | 文案 | 内容 |
|---|---|---|
| 1 | `消息 (${messages.length})` | `_messagesPanel` |
| 2 | `工具 (${toolCalls.length})` | `_toolsPanel` |
| 3 | `文件 (${fileChanges.length})` | `_filesPanel` |
| 4 | `命令 (${commandLogs.length})` | `_commandsPanel` |
| 5 | `进度` | `_progressPanel` |

#### Tab 1 - 消息面板 `_messagesPanel`

- 空：`EmptyState(icon: chat_bubble_outline, title: '还没有消息', subtitle: running ? '等待 AI 响应…' : '在底部输入消息开始对话')`
- 非空：`ListView.builder(controller: _scrollController, padding: symmetric(vertical: 8))`
  - 每项：`MessageBubble(role: m.role, blocks: m.blocks, timestamp: m.timestamp)`

#### Tab 2 - 工具面板 `_toolsPanel`

- 空：`EmptyState(icon: build_outlined, title: '还没有工具调用')`（无 subtitle）
- 非空：`ListView.builder(padding: all(8))`
  - 每项：`ToolCallCard(toolUseId, toolName, input, result, isError, done)`

#### Tab 3 - 文件面板 `_filesPanel`

- 顶部右上角 `Padding` + `Align(centerRight)` + `TextButton.icon(Icons.history_outlined, '查看变更历史')`：
  - 仅当 `serverId != null && cwd != null` 时可点 → `_openSnapshotHistory(serverId, cwd)`（`Navigator.push` 到 `SnapshotHistoryScreen`）
- 主体 `Expanded`：
  - 空：`EmptyState(icon: folder_off_outlined, title: '没有文件变更')`
  - 非空：`ListView.builder(padding: symmetric(vertical: 4))`，每项 `FileChangeCard(change: ...)`

#### Tab 4 - 命令面板 `_commandsPanel`

- 空：`EmptyState(icon: terminal, title: '没有命令日志')`
- 非空：`ListView.builder(padding: symmetric(vertical: 4))`，每项 `CommandLogCard(event: ...)`

#### Tab 5 - 进度面板 `_progressPanel`

- 直接渲染 `TodoProgressBar(todos: state.todos, progress: state.progress)`

### 13.4 事件如何映射到 UI

事件分发由 `SessionController._onEnvelope` 完成，UI 仅监听 state。映射关系：

| 事件类型 | state 字段影响 | UI 渲染 |
|---|---|---|
| `session.init` | sessionId / cli / model / cwd / status=running | AppBar 标题与状态点 |
| `message` | messages 追加一条 ChatMessage | 消息 Tab 中 `MessageBubble` |
| `streaming.delta` | 按 messageId 聚合到 assistant 消息的 TextBlock | 消息 Tab 中累积显示 |
| `streaming.done` | status=idle，清理 _streamingIndex | AppBar 状态点变灰；输入栏切回发送按钮 |
| `tool.use` | toolCalls 追加一条 done=false 的 ToolCall | 工具 Tab 中 `ToolCallCard(done=false)` |
| `tool.result` | toolCalls 中按 toolUseId 配对，更新 result/isError/done=true | 工具 Tab 中卡片转为完成/出错状态 |
| `file.change` | fileChanges 追加 FileChange | 文件 Tab 中 `FileChangeCard` |
| `command.exec` | commandLogs 追加事件 | 命令 Tab 中 `CommandLogCard` |
| `todo.update` | todos = event.todos（整列表替换） | 进度 Tab 中 `TodoProgressBar` 列表 |
| `thinking` | messages 追加 assistant + ThinkingBlock | 消息 Tab 中渲染为半透明 italic |
| `progress` | progress = event | 进度 Tab 顶部 message + 进度条 |
| `error` | errorMessage = event.message；recoverable=false 时 status=error | 顶部错误横幅 |
| `session.end` | status=ended, stats=event.stats | 底部输入栏替换为"会话已结束"横幅 + stats 行 |

### 13.5 流式消息累积显示

`SessionController._applyDelta(StreamingDeltaEvent e)`：

1. `delta` 为空直接返回
2. 查 `_streamingIndex[messageId]`：
   - 不存在 → 在 messages 末尾新建一条 `ChatMessage(role: 'assistant', blocks: [TextBlock(text: delta)])`，记录索引
   - 存在 → 取出该消息，复制 blocks：
     - 若最后一个 block 是 `TextBlock` → 替换为 `TextBlock(text: last.text + delta)`
     - 否则 → 追加新的 `TextBlock(text: delta)`
3. `_streamingDoneEvent` 触发时：`_streamingIndex.remove(messageId)` + `status = idle`

UI 自动滚动到底部：`ref.listen` 监听 state 变化 → `_scrollToBottom()`（`addPostFrameCallback` + `animateTo(maxScrollExtent, 200ms, easeOut)`）

### 13.6 重连横幅显示规则

`_reconnectingBanner`：
- **显示条件**：`state.wsState == WsConnectionState.reconnecting`
- **样式**：`Container(color: tertiaryContainer, padding: horizontal 12 + vertical 6)` + `Row`：
  - `SizedBox(14x14, CircularProgressIndicator(strokeWidth: 2))`
  - `SizedBox(width: 8)`
  - `Text('正在重连…', color: onTertiaryContainer)`

### 13.7 错误横幅显示规则

`_errorBanner`：
- **显示条件**：`state.errorMessage != null`
- **样式**：`Container(color: errorContainer, padding: horizontal 12 + vertical 6)` + `Row`：
  - `Icon(Icons.error_outline, size: 16, color: onErrorContainer)`
  - `SizedBox(width: 8)`
  - `Expanded(Text(msg, maxLines: 1, ellipsis, color: onErrorContainer))`

### 13.8 会话结束时的底部展示（stats 行）

- **显示条件**：`state.status == SessionStatus.ended`
- **替换输入栏**：用 `SafeArea(top: false)` 包 `Container(width: infinity, padding: all(12), color: surfaceContainerHighest)`
- **内容**：`Row(mainAxisAlignment: center)`：
  - `Icon(Icons.flag_outlined, size: 18, color: outline)`
  - `SizedBox(width: 8)`
  - `Text('会话已结束', color: outline)`
  - （stats != null）`SizedBox(width: 12)` + `Text(_statsLine, labelSmall)`
- **`_statsLine` 格式**（用 ` · ` 连接）：
  - `numTurns != null` → `'${numTurns} 轮'`
  - `durationMs != null` → `'${(durationMs/1000).toStringAsFixed(1)}s'`
  - `costUsd != null` → `'\$${costUsd.toStringAsFixed(4)}'`
  - `inputTokens || outputTokens` → `'${inputTokens ?? 0}/${outputTokens ?? 0} tok'`

### 13.9 启动模式判定与会话引导

`_isStartMode = widget.sessionId == 'new' || widget.startPrompt != null`

`_boot()` 在 `addPostFrameCallback` 中执行：
- `_isStartMode`：调 `startExistingSession(sessionId, serverId, cwd, cli, prompt, model)` —— WS 连接 + 发 `session.start`
- 否则（恢复）：调 `resumeSession(sessionId, cli, model, cwd)` —— 先拉历史事件回放，再连 WS

`_booted` 标志位防止重复引导。

---

## §14 组件规范

### 14.1 LoadingIndicator

- **API**：`LoadingIndicator({String? label})`
- **结构**：`Center(Column(mainAxisSize: min))`：
  - `CircularProgressIndicator()`
  - （label != null）`SizedBox(height: 12)` + `Text(label, bodyMedium)`
- **使用场景**：启动页、设置页 API Key 加载、列表页 loading 态、文件浏览加载态、快照历史初载

### 14.2 EmptyState

- **API**：`EmptyState({required IconData icon, required String title, String? subtitle, Widget? action})`
- **结构**：`Center(Padding(all: 32, Column(min)))`：
  - `Icon(icon, size: 64, color: outline)`
  - `SizedBox(height: 16)`
  - `Text(title, titleMedium)`
  - （subtitle != null）`SizedBox(height: 8)` + `Text(subtitle, bodyMedium + outline + textAlign: center)`
  - （action != null）`SizedBox(height: 16)` + action
- **常用 icon 约定**：
  - 服务器：`dns_outlined`
  - 项目：`folder_off_outlined`
  - 任务：`task_alt`
  - 会话：`chat_bubble_outline`
  - 文件目录：`folder_open`
  - 文件变更：`folder_off_outlined`
  - 工具：`build_outlined`
  - 命令：`terminal`
  - 快照：`history_outlined`

### 14.3 ErrorView

- **API**：`ErrorView({required String message, VoidCallback? onRetry})`
- **结构**：`Center(Padding(all: 24, Column(min)))`：
  - `Icon(Icons.error_outline, size: 56, color: error)`
  - `SizedBox(height: 12)`
  - `Text('出错了', titleMedium + error)`
  - `SizedBox(height: 8)`
  - `Text(message, bodyMedium + outline + textAlign: center)`
  - （onRetry != null）`SizedBox(height: 16)` + `FilledButton.tonalIcon(Icons.refresh, '重试')`

### 14.4 MessageBubble

- **API**：`MessageBubble({required String role, required List<ContentBlock> blocks, required int timestamp})`
- **布局**：
  - `user`：右对齐，背景 `primaryContainer`，前景 `onPrimaryContainer`，圆角 16（右下角 4）
  - `assistant`：左对齐，背景 `surfaceContainerHighest`，前景 `onSurface`，圆角 16（左下角 4）
- **blocks 渲染**：
  - `TextBlock`：`SelectableText(text)`，bodyMedium
  - `ThinkingBlock`：`SelectableText(text)`，bodySmall + italic + outline
  - `ImageBlock`：`Image.memory(base64Decode(dataB64))`
  - `ToolUseBlock`：内嵌 `ToolCallCard(done: false)`（mini 版）
  - `ToolResultBlock`：内嵌 `ToolCallCard(done: true)`（mini 版）
- **最大宽度**：`ConstrainedBox(constraints: BoxConstraints(maxWidth: screenWidth * 0.8))`

### 14.5 ToolCallCard

- **API**：`ToolCallCard({required String toolUseId, required String toolName, required Object? input, required String? result, required bool isError, required bool done})`
- **状态字段**：`done: bool`、`isError: bool`、`result: String?`
- **状态颜色与文案**：
  - `!done` → `tertiary`，文案 `'执行中…'`
  - `done && isError` → `error`，文案 `'出错'`
  - `done && !isError` → `primary`，文案 `'完成'`
- **结构**：
  - 外层 `Container(border: Border.all(dividerColor), radius: 8)`
  - 顶部 `Container(color: statusColor.withOpacity(0.1), padding: horizontal 10 + vertical 6)`：`Row`：`Icon(Icons.build, size: 16)` + `Expanded(Text(toolName 或 '工具结果', labelLarge + bold))` + `Text(statusLabel, labelSmall)`
  - input 区（hasInput）：`SelectableText(_formatInput, bodySmall + monospace)`，padding `all(8)`
    - `_formatInput`：String 直接返回；其他用 `JsonEncoder.withIndent('  ').convert`
  - result 区（hasResult）：`Container(color: isError ? errorContainer.withOpacity(0.4) : surfaceContainerHighest, radius: 4)`，内含 `SelectableText(result, bodySmall + monospace, color: isError ? onErrorContainer : null)`
- **状态流转**：
  1. 收到 `tool.use` → 创建卡片 `done=false`，只显示 toolName + input，状态"执行中…"
  2. 收到 `tool.result` → 同 toolUseId 的卡片更新 `result/isError/done=true`，显示 result 区，状态切到"完成"或"出错"

### 14.6 FileChangeCard

- **API**：`FileChangeCard({required FileChange change})`
- **结构**：`Card` + `Column`：`ListTile` + （展开时）`_diffView`
- **ListTile**：
  - `leading: Icon(actionIcon, color: actionColor)`
    - create → `add_circle_outline` + `0xFF2E7D32`
    - edit → `edit_outlined` + `0xFF1565C0`
    - delete → `delete_outline` + `0xFFC62828`
  - `title: SelectableText(path, bodyMedium + w600, maxLines: 2)`
  - `subtitle: Row`：`_actionChip(action.wire)` + `+addedLines`(0xFF2E7D32) + `-removedLines`(0xFFC62828)
  - `trailing`（hasDiff 时）：`IconButton(Icons.expand_less | expand_more)`
  - `onTap`（hasDiff 时）：切换 `_expanded`
- **actionChip**：`Container(color: color.withOpacity(0.12), radius: 4, padding: 6,2)`，文字 `fontSize: 11, w600`
- **diff 视图 `_diffView`**（单栏内联展示）：
  - `Container(surfaceContainerHighest, radius: 6, padding: all(8), maxHeight: 320)` + `SingleChildScrollView(Column)`
  - 每行 `Container(color: backgroundFor(kind), padding: horizontal 4)` + `Text(l.text, monospace, fontSize: 12, height: 1.3, color: colorFor(kind))`
- **diff 着色规则**（单栏 `parseDiff`）：

| 行首 | kind | 文字色 | 背景色 |
|---|---|---|---|
| `+++` / `---` | header | `0xFF616161` (grey-700) | transparent |
| `@@` | hunk | `0xFF1565C0` (blue-800) | transparent |
| `+` | add | `0xFF2E7D32` (green-800) | `0x1A2E7D32` |
| `-` | remove | `0xFFC62828` (red-800) | `0x1AC62828` |
| 其他 | context | `0xFF424242` (grey-800) | transparent |

### 14.7 CommandLogCard

- **API**：`CommandLogCard({required CommandExecEvent event})`
- **结构**：`Card` + `Column`：`ListTile` + （展开时）`_outputView`
- **ListTile**：
  - `leading: Icon(Icons.terminal, color: primary)`
  - `title: SelectableText(command, bodyMedium + monospace + w600)`
  - `subtitle: Row`：cwd（`labelSmall + outline + ellipsis`） + `Text('exit ${exitCode}', color: exitColor)`
    - `exitColor`：`exitCode == null → outline` / `== 0 → 0xFF2E7D32` / `!= 0 → 0xFFC62828`
  - `trailing`（hasOutput 时）：`IconButton(expand_less | expand_more)`
  - `onTap`（hasOutput 时）：切换 `_expanded`
- **输出视图 `_outputView`**（终端风格）：
  - `Container(color: 0xFF1E1E1E, radius: 6, padding: all(8), maxHeight: 280)` + `SingleChildScrollView(Column)`
  - stdout 区：`Text('stdout', grey, fontSize 11)` + `SelectableText(stdout, monospace, fontSize 12, color: 0xFFE0E0E0)`
  - stderr 区：`Text('stderr', grey, fontSize 11)` + `SelectableText(stderr, monospace, fontSize 12, color: 0xFFEF9A9A)`

### 14.8 TodoProgressBar

- **API**：`TodoProgressBar({required List<TodoItem> todos, required ProgressEvent? progress})`
- **结构**：`ListView(padding: all(12))`，从上到下：
  1. （可选）`Text(progress.message, titleSmall)` + `SizedBox(height: 8)`
  2. `Row`：
     - `Expanded(ClipRRect(radius: 8, LinearProgressIndicator(value, minHeight: 12)))`
     - `SizedBox(width: 12)`
     - `Text('${pct}%', titleMedium)`，`pct = (value * 100).round()`
  3. （可选）`Text('步骤 ${step} / ${total}', labelSmall + outline)`
  4. `SizedBox(height: 16)` + `Text('任务清单', titleSmall)` + `SizedBox(height: 8)`
  5. todos 为空：`Center(Text('暂无任务', outline))`；否则 `...todos.map(_todoTile)`
- **百分比计算 `_computeProgress`**：
  1. 优先：`progress.total > 0` 时 → `(step / total).clamp(0.0, 1.0)`
  2. 兜底：`todos` 非空时 → `doneCount / todos.length`（doneCount = status==completed 的数量）
  3. todos 为空且无 progress → `0`
- **Todo 单项 `_todoTile`**：

| status | icon | iconColor | 文字样式 |
|---|---|---|---|
| completed | `check_circle` | `0xFF2E7D32` | `lineThrough + outline` |
| inProgress | `radio_button_checked` | `primary` | 默认 |
| pending | `radio_button_unchecked` | `outline` | 默认 |

  - `trailing`（progress != null）：`Text('${progress}%', labelSmall + iconColor)`
  - `ListTile(leading, title: Text(content))`

### 14.9 SessionInputBar

- **API**：`SessionInputBar({required bool running, required ValueChanged<String> onSend, required VoidCallback onInterrupt, required bool enabled})`
- **容器**：`SafeArea(top: false)` + `Material(elevation: 4, color: surface)` + `Padding(fromLTRB(8,8,8,8))`
- **布局**：`Row(crossAxisAlignment: end)`
  - `Expanded(TextField)`：
    - `minLines: 1, maxLines: 5`
    - `textInputAction: TextInputAction.newline`
    - `enabled: widget.enabled`
    - `decoration`：`OutlineInputBorder(radius: 24, borderSide: none)` + `filled: true` + `fillColor: surfaceContainerHighest` + `contentPadding: horizontal 16, vertical 10`
    - `hintText`：`running ? 'AI 正在思考…' : '输入消息…'`
    - `onSubmitted: _submit`
  - `SizedBox(width: 8)`
  - 按钮切换：
    - `running` → `FilledButton.tonalIcon(Icons.stop, '中断')`，`onPressed: enabled ? onInterrupt : null`
    - `!running` → `FilledButton(Icons.send, shape: CircleBorder, padding: all(14))`，`onPressed: enabled ? _submit : null`
- **`_submit` 逻辑**：
  - 取 `_controller.text.trim()`，空或 running 时直接 return
  - 调 `widget.onSend(text)` → `_controller.clear()` → `_focus.requestFocus()`

### 14.10 SnapshotCard

- **API**：`SnapshotCard({required FileSnapshot snapshot, required String? serverId, required String? cwd, VoidCallback? onRolledBack})`
- **结构**：`Card(margin: symmetric(vertical: 4, horizontal: 8))` + `ListTile`
- **ListTile**：
  - `leading: Icon(_actionIcon(action), color: _actionColor(action))`
    - create → `add_circle_outline` + `0xFF2E7D32`
    - delete → `delete_outline` + `0xFFC62828`
    - edit（default） → `edit_outlined` + `0xFF1565C0`
  - `title: SelectableText(path, bodyMedium + w600, maxLines: 2)`
  - `subtitle: Padding(top: 4)` + `Wrap(crossAxisAlignment: center, spacing: 8, runSpacing: 4)`：
    - `_actionChip(action, actionColor)`
    - （可选）`Text('+${addedLines}', 0xFF2E7D32, fontSize 12)`
    - （可选）`Text('-${removedLines}', 0xFFC62828, fontSize 12)`
    - `Text(_fmtTime(createdAt), outline, fontSize 11)`（格式 `YYYY-MM-DD HH:mm`）
  - `trailing: Row(mainAxisSize: min)`：
    - （hasDiff）`IconButton(tooltip: '查看 diff', Icons.compare_arrows_outlined, size: 20) → _showDiff`
    - `IconButton(tooltip: '回滚', _rolling ? CircularProgressIndicator(strokeWidth: 2, 18x18) : Icons.restore_outlined, size: 20, onPressed: _rolling ? null : _confirmRollback)`

### 14.11 DiffSideBySide

- **触发**：`SnapshotCard._showDiff` → `showModalBottomSheet(isScrollControlled: true, showDragHandle: true)`：
  - 高度：`MediaQuery.height * 0.8`
  - 顶部 `Row`：`Expanded(Text(path, w600, ellipsis))` + `IconButton(Icons.close)`
  - `Divider(height: 1)`
  - `Expanded(DiffSideBySide(diff: s.diff ?? ''))`
- **DiffSideBySide 布局**：
  - 外层 `SingleChildScrollView`（纵向） + `LayoutBuilder` 计算视口宽度
  - 内层 `SingleChildScrollView(scrollDirection: horizontal)` + `SizedBox(width: rowWidth)` + `Column`
    - `rowWidth = max(总需宽, 视口宽)`，总需宽 = `(lineNoW + 24 + maxChars * charW) * 2`
  - 常量：`_mono = monospace fontSize 12 height 1.4`、`_rowH = 20.0`、`_lineNoW = 40.0`、`_charW = 7.2`
- **行渲染 `_buildRow`**：
  - `header` 类型：单行 `Container(color: 0xFFE3F2FD 浅蓝, padding: horizontal 6)` + `Text(leftText, _mono + 0xFF1565C0, softWrap: false, maxLines: 1, clip)`
  - 普通行：`SizedBox(height: _rowH)` + `Row(stretch)`：
    - `Expanded(_side(leftText, leftLineNo, sideBackgroundLeft(kind)))`
    - `Expanded(_side(rightText, rightLineNo, sideBackgroundRight(kind)))`
- **`_side` 单元格**：`DecoratedBox(color: bg)` + `Padding(horizontal: 4)` + `Row`：
  - `SizedBox(width: _lineNoW, Text(lineNo, right, _mono + 0xFF9E9E9E + fontSize 11))`
  - `SizedBox(width: 6)`
  - `Expanded(Text(text, _mono, softWrap: false, maxLines: 1, clip))`

### 14.12 确认对话框（约定模式）

代码中无独立组件，统一用 `showDialog<bool>` + `AlertDialog`，约定如下：

- **结构**：`AlertDialog(title, content, actions: [TextButton('取消'), FilledButton.tonal('删除/确认')])`
- **取消按钮**：`TextButton(onPressed: () => Navigator.pop(ctx, false), child: Text('取消'))`
- **确认按钮**（destructive 用 tonal）：`FilledButton.tonal(onPressed: () => Navigator.pop(ctx, true), child: Text('删除' | '确认回滚' | '全部回滚'))`
- **非 destructive 确认**（如设置 API Key）：用 `FilledButton` + `Text('保存' | '开始')`
- **使用场景**：
  - 删除服务器：`'删除服务器'` + `'确认删除「${name}」？关联的项目/任务可能受影响。'`
  - 删除 API Key：`'删除 ${cli.wire} Key'` + `'确认删除 ${cli.wire} 的 API Key？'`
  - 编辑 API Key：`'设置 ${cli.wire} API Key'` + `TextField(obscureText: true)`
  - 开始新会话：`'开始新会话'` + `TextField(autofocus, maxLines: 4)`
  - 单条回滚：`'回滚此变更？'` + `'将回滚对以下文件的变更：\n${path}\n\n该操作会通过 git 还原文件，可能不可撤销。'`
  - 全部回滚：`'全部回滚？'` + `'将回滚本会话全部 ${n} 条文件变更。该操作通过 git 还原，可能不可撤销。'`

---

## §15 工具类：diff 解析与着色

### 15.1 单栏 diff（parseDiff）

```dart
enum DiffLineKind { context, add, remove, hunk, header }

class DiffLine {
  final String text;
  final DiffLineKind kind;
  const DiffLine(this.text, this.kind);
}

List<DiffLine> parseDiff(String diff) {
  final lines = diff.split('\n');
  final result = <DiffLine>[];
  for (final line in lines) {
    if (line.startsWith('+++') || line.startsWith('---')) {
      result.add(DiffLine(line, DiffLineKind.header));
    } else if (line.startsWith('@@')) {
      result.add(DiffLine(line, DiffLineKind.hunk));
    } else if (line.startsWith('+')) {
      result.add(DiffLine(line, DiffLineKind.add));
    } else if (line.startsWith('-')) {
      result.add(DiffLine(line, DiffLineKind.remove));
    } else {
      result.add(DiffLine(line, DiffLineKind.context));
    }
  }
  return result;
}
```

**着色规则**（注意判定顺序，前缀越长的先判）：
1. `+++` 或 `---` → `header`（文件头）
2. `@@` → `hunk`（hunk 头）
3. `+`（单个）→ `add`
4. `-`（单个）→ `remove`
5. 其它（含空行、` ` 单空格行）→ `context`

**颜色** `colorFor(kind)`：
- add → `0xFF2E7D32`（green-800）
- remove → `0xFFC62828`（red-800）
- hunk → `0xFF1565C0`（blue-800）
- header → `0xFF616161`（grey-700）
- context → `0xFF424242`（grey-800）

**背景色** `backgroundFor(kind)`：
- add → `0x1A2E7D32`（10% 透明绿）
- remove → `0x1AC62828`（10% 透明红）
- 其它 → transparent

### 15.2 双栏并排 diff（parseDiffSideBySide）

```dart
enum DiffRowKind { context, removeOnly, addOnly, mixed, header }

class DiffRow {
  final String? leftText;
  final String? rightText;
  final int? leftLineNo;
  final int? rightLineNo;
  final DiffRowKind kind;
  const DiffRow({...});
}
```

**hunk 头正则**：`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`，捕获 oldStart/oldLen/newStart/newLen。

**算法**：
1. 按 `\n` 拆行，空行跳过（注释：空行通常是末尾换行产生的伪行，真实空行在 diff 里是 ` ` 单空格）
2. 用 `_hunkRe` 匹配 hunk 头：命中则 `flush()` 上一批缓冲，进入 hunk 模式，记录 `oldNo`/`newNo`，输出一行 `header`
3. hunk 之前的内容：`---`/`+++` 作为 `header` 行，其它忽略
4. hunk 内按首字符分类：
   - `\\` 开头（`\ No newline at end of file`）跳过
   - ` `（空格）：`flush()` 后输出 `context` 行（左右相同，行号 oldNo/newNo），两个行号都 +1
   - `-`：加入 `removes` 缓冲，oldNo+1
   - `+`：加入 `adds` 缓冲，newNo+1
   - 其它罕见无前缀：当 context 处理，两个行号都 +1
5. **`flush()` 配对算法**：取 `removes.length` 和 `adds.length` 的最大值 n，按下标 zip：
   - 都有 → `mixed`（左红右绿）
   - 仅 removes 有 → `removeOnly`（右留空）
   - 仅 adds 有 → `addOnly`（左留空）
6. 末尾再 `flush()` 一次

**双栏背景色**：
- `sideBackgroundLeft(kind)`：removeOnly/mixed → 红（`0x1AC62828`）；context → 极浅灰（`0x0D000000`）；其它 transparent
- `sideBackgroundRight(kind)`：addOnly/mixed → 绿（`0x1A2E7D32`）；context → 极浅灰；其它 transparent

**健壮性**：空 diff 直接返回空列表；无 hunk 头的纯文件头也能展示；多 hunk、多文件头都支持。

---

## §16 Android 平台配置

### 16.1 AndroidManifest.xml

```xml
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <!-- 网络权限 -->
    <uses-permission android:name="android.permission.INTERNET"/>
    <uses-permission android:name="android.permission.ACCESS_NETWORK_STATE"/>

    <!-- 兼容 TV -->
    <uses-feature android:name="android.hardware.touchscreen" android:required="false"/>
    <uses-feature android:name="android.software.leanback" android:required="false"/>

    <application
        android:label="HeyCode"
        android:name="${applicationName}"
        android:icon="@mipmap/ic_launcher"
        android:usesCleartextTraffic="true">
        <activity
            android:name=".MainActivity"
            android:exported="true"
            android:launchMode="singleTop"
            android:taskAffinity=""
            android:theme="@style/LaunchTheme"
            android:configChanges="orientation|keyboardHidden|keyboard|screenSize|smallestScreenSize|locale|layoutDirection|fontScale|screenLayout|density|uiMode"
            android:hardwareAccelerated="true"
            android:windowSoftInputMode="adjustResize">
            <meta-data
                android:name="io.flutter.embedding.android.NormalTheme"
                android:resource="@style/NormalTheme"/>
            <intent-filter>
                <action android:name="android.intent.action.MAIN"/>
                <category android:name="android.intent.category.LAUNCHER"/>
            </intent-filter>
        </activity>
        <meta-data
            android:name="flutterEmbedding"
            android:value="2"/>
    </application>
</manifest>
```

**关键点**：
- `INTERNET` 权限必需（连接自托管后端）
- `usesCleartextTraffic="true"`：允许 HTTP 明文（开发期连 `http://192.168.x.x:8787`；Android 9+ 默认禁明文。上 HTTPS 后可删）
- `configChanges` 自己处理配置变化，避免重建
- `windowSoftInputMode="adjustResize"`：键盘弹出时调整布局
- `flutterEmbedding` = 2

### 16.2 build.gradle.kts（app 级）

```kotlin
plugins {
    id("com.android.application")
    id("kotlin-android")
    id("dev.flutter.flutter-gradle-plugin")  // 必须放最后
}

import java.util.Properties
import java.io.FileInputStream

val keystorePropertiesFile = rootProject.file("key.properties")
val keystoreProperties = Properties()
if (keystorePropertiesFile.exists()) {
    keystoreProperties.load(FileInputStream(keystorePropertiesFile))
}

android {
    namespace = "com.heycode.app"
    compileSdk = 36  // shared_preferences_android 要求 36
    ndkVersion = flutter.ndkVersion

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }

    defaultConfig {
        applicationId = "com.heycode.app"
        minSdk = 24          // Android 7.0
        targetSdk = 36
        versionCode = flutter.versionCode
        versionName = flutter.versionName
    }

    signingConfigs {
        create("release") {
            if (keystoreProperties.isNotEmpty()) {
                keyAlias = keystoreProperties["keyAlias"] as String
                keyPassword = keystoreProperties["keyPassword"] as String
                storeFile = file(keystoreProperties["storeFile"] as String)
                storePassword = keystoreProperties["storePassword"] as String
            }
        }
    }

    buildTypes {
        release {
            signingConfig = signingConfigs.getByName("release")
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
        }
        debug {
            signingConfig = signingConfigs.getByName("debug")
        }
    }

    flutter {
        source = "../.."
    }
}
```

### 16.3 build.gradle.kts（project 级）

```kotlin
plugins {
    id("com.android.application") version "8.7.3" apply false
    id("org.jetbrains.kotlin.android") version "2.0.20" apply false
}
```

- AGP（Android Gradle Plugin）版本：**8.7.3**
- Kotlin 版本：**2.0.20**
- 注释：`dev.flutter.flutter-plugin-loader` 必须在 `settings.gradle.kts` 里 apply，不能放在 project 级

### 16.4 settings.gradle.kts

```kotlin
pluginManagement {
    val flutterSdkPath = run {
        val properties = java.util.Properties()
        file("local.properties").inputStream().use { properties.load(it) }
        val flutterSdkPath = properties.getProperty("flutter.sdk")
        require(flutterSdkPath != null) { "flutter.sdk 未在 local.properties 设置" }
        flutterSdkPath
    }
    includeBuild("$flutterSdkPath/packages/flutter_tools/gradle")
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}

plugins {
    id("dev.flutter.flutter-plugin-loader") version "1.0.0"
    id("com.android.application") version "8.7.3" apply false
    id("org.jetbrains.kotlin.android") version "2.0.20" apply false
}

include(":app")
```

### 16.5 gradle.properties

```properties
org.gradle.jvmargs=-Xmx4G -XX:MaxMetaspaceSize=2G -XX:+HeapDumpOnOutOfMemoryError
org.gradle.parallel=true
org.gradle.configureondemand=true
kotlin.code.style=official
android.useAndroidX=true
```

### 16.6 proguard-rules.pro

```
# 保留 Flutter 引擎相关类
-keep class io.flutter.app.** { *; }
-keep class io.flutter.plugin.**  { *; }
-keep class io.flutter.util.**  { *; }
-keep class io.flutter.view.**  { *; }
-keep class io.flutter.**  { *; }
-keep class io.flutter.plugins.**  { *; }

# 保留 Kotlin 元数据
-keep class kotlin.Metadata { *; }

# 保留 Parcelable 序列化对象
-keepclassmembers class * implements android.os.Parcelable {
    public static final android.os.Parcelable$Creator CREATOR;
}
```

### 16.7 key.properties.example

```properties
storePassword=你的keystore密码
keyPassword=你的key密码
keyAlias=release
storeFile=/绝对路径/to/app-release.jks
```

keystore 生成命令：

```bash
keytool -genkey -v -keystore ~/keystore/app-release.jks \
  -keyalg RSA -keysize 2048 -validity 10000 -alias release
```

`key.properties` 不提交 git（.gitignore 排除）。

### 16.8 MainActivity.kt

文件路径：`android/app/src/main/kotlin/com/heycode/app/MainActivity.kt`

```kotlin
package com.heycode.app

import io.flutter.embedding.android.FlutterActivity

class MainActivity: FlutterActivity()
```

### 16.9 styles.xml 与启动背景

`android/app/src/main/res/values/styles.xml`：

```xml
<resources>
    <style name="LaunchTheme" parent="@android:style/Theme.Light.NoTitleBar">
        <item name="android:windowBackground">@drawable/launch_background</item>
    </style>
    <style name="NormalTheme" parent="@android:style/Theme.Light.NoTitleBar">
        <item name="android:windowBackground">?android:colorBackground</item>
    </style>
</resources>
```

`android/app/src/main/res/drawable/launch_background.xml`：

```xml
<?xml version="1.0" encoding="utf-8"?>
<layer-list xmlns:android="http://schemas.android.com/apk/res/android">
    <item android:drawable="?android:colorBackground" />
</layer-list>
```

### 16.10 应用 ID 与命名空间约定

- **applicationId / namespace**：`com.heycode.app`
- **Kotlin 包路径**：`android/app/src/main/kotlin/com/heycode/app/`
- **MainActivity**：`com.heycode.app.MainActivity`

---

## §17 代码规范与 lint

### 17.1 analysis_options.yaml

```yaml
include: package:flutter_lints/flutter.yaml

analyzer:
  language:
    strict-casts: false
    strict-raw-types: false
  errors:
    invalid_annotation_target: ignore

linter:
  rules:
    - prefer_const_constructors
    - prefer_const_literals_to_create_immutables
    - prefer_final_locals
    - prefer_single_quotes
    - unnecessary_const
    - unnecessary_new
    - avoid_print
```

### 17.2 代码风格约定

- **字符串**：单引号（`'...'`），符合 `prefer_single_quotes`
- **常量构造**：可空默认值用 `const`；widget 用 `const` 构造
- **import 顺序**：dart: → package:flutter → package:第三方 → 相对路径
- **命名**：
  - 文件 snake_case（`session_controller.dart`）
  - 类 PascalCase
  - provider camelCase + `Provider` 后缀（`serversProvider`、`apiClientProvider`）
- **状态管理**：`StateNotifier` + 不可变 state + `copyWith`；列表更新用 `[...state.list, item]` 展开
- **错误处理**：
  - service 层 `try/on DioException catch (e) { _err(e); }`，`_err` 是 `Never`
  - UI 层 `try { await api.xxx() } catch (e) { /* SnackBar 反馈 */ }`
- **JSON 解析**：
  - 所有 `fromJson` 对缺失字段回退默认值，永不抛
  - 枚举用 `wire` + `fromWire(orElse:)` 模式
  - 可空字段 toJson 用 `if (x != null)` 条件
- **sealed class**：Dart 3 sealed 用于 union 类型，switch 模式匹配分发
- **路由**：
  - go_router，路径全小写 + `:id` 路径参数 + query 参数
  - 导航用 `context.push` 入栈、`context.go` 根替换、`context.pop` 出栈
- **注释**：中文注释为主（文件头说明模块、关键方法说明意图）
- **Riverpod**：
  - `Provider`（单例服务）
  - `StateProvider`（简单值/配置）
  - `FutureProvider.family`（参数化查询）
  - `StateNotifierProvider.autoDispose`（控制器）
  - `ref.watch` 用于响应式依赖，`ref.read` 用于一次性读取

---

## §18 构建与发布

### 18.1 项目初始化

```bash
# 1. 创建 Flutter 项目（指定 Android 平台和包名）
flutter create . --platforms=android --project-name heycode_app

# 2. 把预写的 lib/、android/ 配置文件、pubspec.yaml、analysis_options.yaml 放进去
#    （flutter create 不会覆盖已有文件）

# 3. 拉取依赖
flutter pub get
```

### 18.2 debug 构建

```bash
cd heycode_app
flutter pub get
flutter build apk --debug
# 或 flutter run（直接装机调试）
```

输出：`build/app/outputs/flutter-apk/app-debug.apk`

### 18.3 release 构建（签名要求）

```bash
# 1. 生成 keystore（首次）
keytool -genkey -v -keystore ~/.heycode/app-release.jks \
  -keyalg RSA -keysize 2048 -validity 10000 -alias release

# 2. 配置 android/key.properties（复制 key.properties.example 改）
# 3. 构建
cd heycode_app
flutter pub get
flutter build apk --release
```

输出：`build/app/outputs/flutter-apk/app-release.apk`

也可用 `flutter build apk --release --split-per-abi` 按 ABI 拆分（arm64-v8a / armeabi-v7a / x86_64），输出三个小 APK。

### 18.4 本地构建辅助脚本 build-android.sh

提供三种模式：`debug` / `apk`（=release，默认）/ `split`：

```bash
./build-android.sh debug    # 构建调试 APK
./build-android.sh apk      # 构建 release APK（需 key.properties）
./build-android.sh split    # 按 ABI 拆分
```

脚本流程：
1. 环境检查（flutter + java）
2. Android 脚手架检查（缺失则 `flutter create . --platforms=android --project-name heycode_app`）
3. 签名配置（首次交互式生成 keystore 并写 key.properties）
4. 打包

### 18.5 APK 输出路径

- debug：`build/app/outputs/flutter-apk/app-debug.apk`
- release：`build/app/outputs/flutter-apk/app-release.apk`
- split：`build/app/outputs/flutter-apk/app-{arm64-v8a,armeabi-v7a,x86_64}-release.apk`
- 备用路径（flutter create 后）：`android/app/build/outputs/flutter-apk/`

### 18.6 版本号管理

`pubspec.yaml` 中 `version: 1.0.0+1`（`versionName+versionCode`）。`app/build.gradle.kts` 中：

```kotlin
versionCode = flutter.versionCode    // 1
versionName = flutter.versionName    // 1.0.0
```

由 Flutter Gradle 插件从 pubspec.yaml 解析注入。改版本只需改 `pubspec.yaml` 的 `version:` 字段，无需动 gradle。`flutter build apk` 会自动用该版本号。

---

## §19 GitHub Actions CI

### 19.1 workflow 文件

`.github/workflows/build-android.yml`

### 19.2 触发条件

- push 到 `main`/`master` 分支（paths 含 `mobile/**`、`shared/**`、workflow 文件）→ 构建 **debug** APK
- push tag `v*` → 构建 **release** APK 并发 GitHub Release
- `workflow_dispatch` 手动触发，可选 debug/release/split

### 19.3 并发控制

```yaml
concurrency:
  group: build-android-${{ github.ref }}
  cancel-in-progress: true
```

同分支新构建取消旧的。

### 19.4 权限

```yaml
permissions:
  contents: write  # 发 Release 需要
```

### 19.5 Job 步骤

`runs-on: ubuntu-latest`，`timeout-minutes: 30`：

1. `actions/checkout@v4`
2. `actions/setup-java@v4`：JDK 17（temurin）
3. `subosito/flutter-action@v2`：stable channel，启用 cache
4. `yes | flutter doctor --android-licenses`：接受 Android license
5. `flutter doctor -v`
6. `flutter pub get`（working-directory: mobile）
7. `flutter create . --platforms=android --project-name heycode_app`：补齐脚手架资源（不覆盖已有文件）
8. 校验 AndroidManifest 含 INTERNET 权限（缺失则 `exit 1`）
9. `flutter analyze || true`：静态检查（不阻断）
10. 决定构建模式（`steps.mode.outputs.build_mode`）：workflow_dispatch 用 input；tag 用 release；其它（main/master push）用 debug
11. **解码签名 keystore**（仅非 debug）：从 secret `ANDROID_KEYSTORE_BASE64` base64 解码到 `android/app/release.jks`，写 `android/key.properties`（4 个 secret：`ANDROID_KEYSTORE_BASE64`、`ANDROID_KEY_ALIAS`、`ANDROID_KEY_PASSWORD`、`ANDROID_STORE_PASSWORD`）。未配置则构建未签名 APK（不阻断）
12. 构建：`flutter build apk --debug/--release/--release --split-per-abi`（均 `|| true`，避免 Flutter 工具路径检查误报失败）
13. **上传 artifact**：`actions/upload-artifact@v4`，name 为 `app-{mode}` 或 `app-split-per-abi`，path 同时覆盖两个可能路径，`retention-days: 14`，`if-no-files-found: error`
14. **发布 Latest Prerelease**（仅 `github.ref == refs/heads/main` 且非 PR）：`softprops/action-gh-release@v2`，tag_name `latest`，prerelease=true，make_latest=false，上传 `app-debug.apk`，body 含直接下载链接 `https://github.com/<org>/<repo>/releases/download/latest/app-debug.apk`
15. **创建正式 Release**（仅 `startsWith(github.ref, 'refs/tags/v')`）：`softprops/action-gh-release@v2`，上传 `app-release.apk`，`generate_release_notes: true`，draft=false，prerelease=false
16. **构建结果摘要**：写 `$GITHUB_STEP_SUMMARY`（模式、事件、分支、提交、APK 列表）

### 19.6 所需 GitHub Secrets

| Secret 名 | 用途 |
|---|---|
| `ANDROID_KEYSTORE_BASE64` | release keystore 的 base64 编码 |
| `ANDROID_KEY_ALIAS` | keystore alias（通常 `release`） |
| `ANDROID_KEY_PASSWORD` | key 密码 |
| `ANDROID_STORE_PASSWORD` | keystore 密码 |

---

## §20 任务分解（Milestones）

### M1 - 项目初始化与主题

**任务**：
- `flutter create . --platforms=android --project-name heycode_app`
- 配置 pubspec.yaml 依赖
- 配置 analysis_options.yaml
- 实现 main.dart（ProviderScope + MaterialApp.router + 主题）
- 配置 Android 平台文件（AndroidManifest、build.gradle.kts、settings.gradle.kts、gradle.properties、proguard-rules.pro、MainActivity.kt、styles.xml、launch_background.xml）

**验收**：
```bash
flutter pub get
flutter analyze
flutter build apk --debug
# APK 输出在 build/app/outputs/flutter-apk/app-debug.apk
```

### M2 - 数据模型层

**任务**：
- 实现 `lib/models/unified_event.dart`（CliKind、ContentBlock、UnifiedEvent 全部子类、ClientCommand、ServerEnvelope、FileChange、TodoItem、SessionStats）
- 实现 `lib/models/server.dart`（SshAuth sealed、SshAuthKind、ServerStatus、Server、FileListing、FileContent、ServerTestResult）
- 实现 `lib/models/project.dart`
- 实现 `lib/models/task.dart`（TaskStatus）
- 实现 `lib/models/session.dart`（SessionStatus）
- 实现 `lib/models/file_entry.dart`
- 实现 `lib/models/file_snapshot.dart`（FileSnapshot、RollbackResult）

**验收**：
- 所有 model 的 fromJson/toJson 单元测试通过
- 缺失字段回退默认值，不抛异常
- 未知事件 type 回退 ErrorEvent

### M3 - 服务层

**任务**：
- 实现 `lib/services/storage.dart`（Storage 类）
- 实现 `lib/config.dart`（AppConfig）
- 实现 `lib/services/api_client.dart`（Dio 配置、ApiException、所有 REST 方法）
- 实现 `lib/services/ws_client.dart`（状态机、重连、心跳、消息收发）

**验收**：
- ApiClient 所有方法签名与端点正确
- WsClient 状态机转换正确（disconnected/connecting/connected/reconnecting）
- 重连后发 SessionResyncCommand

### M4 - 状态层与路由

**任务**：
- 实现 `lib/state/providers.dart`（全部 provider）
- 实现 `lib/state/router.dart`（完整路由表）
- 实现 `lib/state/session_controller.dart`（State + 四种启动入口 + 事件分发 + 流式增量聚合）

**验收**：
- `ref.watch(serversProvider(null))` 能返回数据
- 路由跳转正常（push/go/pop）
- SessionController 事件分发逻辑正确

### M5 - 通用组件与启动页

**任务**：
- 实现 `lib/widgets/loading_indicator.dart`
- 实现 `lib/widgets/empty_state.dart`
- 实现 `lib/widgets/error_view.dart`
- 实现 `lib/screens/splash_screen.dart`
- 实现 `lib/screens/settings_screen.dart`（后端地址 + API Key 管理）

**验收**：
- 启动页健康检查通过后跳 `/servers`，失败跳 `/settings`
- 设置页保存后能更新 configProvider
- API Key 列表能加载、编辑、删除

### M6 - 服务器与项目管理

**任务**：
- 实现 `lib/screens/servers_screen.dart`
- 实现 `lib/screens/server_form_screen.dart`
- 实现 `lib/screens/files_screen.dart`（含内嵌 `_FileEditorScreen`）
- 实现 `lib/screens/projects_screen.dart`
- 实现 `lib/screens/project_form_screen.dart`
- 实现 `lib/screens/tasks_screen.dart`
- 实现 `lib/screens/task_form_screen.dart`
- 实现 `lib/screens/session_list_screen.dart`

**验收**：
- 服务器 CRUD 完整
- 服务器连通性测试可用
- SFTP 文件浏览与编辑可用
- 项目/任务 CRUD 完整
- 会话列表能加载、新建会话

### M7 - 会话页核心

**任务**：
- 实现 `lib/widgets/message_bubble.dart`
- 实现 `lib/widgets/tool_call_card.dart`
- 实现 `lib/widgets/file_change_card.dart`
- 实现 `lib/widgets/command_log_card.dart`
- 实现 `lib/widgets/todo_progress_bar.dart`
- 实现 `lib/widgets/session_input_bar.dart`
- 实现 `lib/screens/session_screen.dart`（5 个 Tab + 流式 + 状态横幅）

**验收**：
- 新建会话能连接 WS 并收到 session.init
- 流式消息累积显示正确
- 工具调用卡片状态流转正确
- 5 个 Tab 切换正常
- 中断/结束会话可用

### M8 - 文件快照历史与回滚

**任务**：
- 实现 `lib/utils/diff_painter.dart`（parseDiff + parseDiffSideBySide）
- 实现 `lib/widgets/snapshot_card.dart`
- 实现 `lib/widgets/diff_side_by_side.dart`
- 实现 `lib/screens/snapshot_history_screen.dart`

**验收**：
- 快照列表能加载
- 双栏 diff 展示正确（左红右绿配对）
- 单条回滚 / 全部回滚可用

### M9 - 编译发布与 CI

**任务**：
- 实现 `build-android.sh` 本地构建脚本
- 配置 `.github/workflows/build-android.yml`
- 配置 GitHub Secrets（4 个签名相关）
- 测试 push 到 main 触发 debug 构建
- 测试 push tag v* 触发 release 构建

**验收**：
- 本地 `./build-android.sh debug` 能生成 APK
- GitHub Actions debug 构建成功，artifact 可下载
- push tag 后 Release 自动发布，APK 可下载

### M10 - 端到端测试与优化

**任务**：
- 配置真实后端地址，跑通完整流程：启动 → 配置 → 服务器 → 项目 → 任务 → 会话 → AI 交互 → 文件变更 → 回滚
- 性能优化（长会话滚动、diff 渲染）
- 异常处理覆盖（网络断开、WS 重连、API 错误）
- 国际化/无障碍（如有需要）

**验收**：
- 完整业务流程跑通
- WS 断线重连后状态恢复
- 长会话（100+ 消息）滚动流畅

---

## 附录 A：REST 端点速查

| HTTP | 端点 | 用途 |
|---|---|---|
| GET | `/api/health` | 健康检查 |
| GET | `/api/servers?projectId=` | 服务器列表 |
| POST | `/api/servers` | 创建服务器 |
| GET | `/api/servers/$id` | 服务器详情 |
| PATCH | `/api/servers/$id` | 更新服务器 |
| DELETE | `/api/servers/$id` | 删除服务器 |
| POST | `/api/servers/$id/test` | 测试服务器连通性 |
| GET | `/api/servers/$id/files?path=` | 文件列表 |
| GET | `/api/servers/$id/files/content?path=` | 读文件 |
| PUT | `/api/servers/$id/files/content` | 写文件 |
| DELETE | `/api/servers/$id/files` | 删除文件 |
| GET | `/api/projects?serverId=` | 项目列表 |
| POST | `/api/projects` | 创建项目 |
| GET | `/api/projects/$id` | 项目详情 |
| PATCH | `/api/projects/$id` | 更新项目 |
| DELETE | `/api/projects/$id` | 删除项目 |
| GET | `/api/projects/$projectId/tasks` | 任务列表 |
| POST | `/api/tasks` | 创建任务 |
| GET | `/api/tasks/$id` | 任务详情 |
| PATCH | `/api/tasks/$id` | 更新任务 |
| DELETE | `/api/tasks/$id` | 删除任务 |
| GET | `/api/tasks/$taskId/sessions` | 会话列表 |
| POST | `/api/sessions` | 创建会话 |
| GET | `/api/sessions/$id` | 会话详情 |
| GET | `/api/sessions/$id/events?since=` | 会话历史事件 |
| DELETE | `/api/sessions/$id` | 删除会话 |
| GET | `/api/sessions/$sessionId/snapshots` | 文件快照列表 |
| POST | `/api/snapshots/$snapshotId/rollback` | 单条回滚 |
| POST | `/api/sessions/$sessionId/rollback` | 全部回滚 |
| GET | `/api/api-keys` | API Key 列表 |
| POST | `/api/api-keys` | 保存 API Key |
| DELETE | `/api/api-keys/$cliWire` | 删除 API Key |

---

## 附录 B：WebSocket 协议速查

### 连接

- URL：`{wsUrl}/ws?sessionId={sessionId}`（wsUrl 由 baseUrl 推导）
- 协议：JSON 文本帧

### 客户端 → 服务端（ClientCommand）

| kind | 字段 |
|---|---|
| `session.start` | serverId, cwd, cli, prompt, model?, resumeCliSessionId?, allowedTools? |
| `session.send` | prompt |
| `session.interrupt` | — |
| `session.end` | — |
| `session.resync` | sinceEventId: String |
| `ping` | — |

### 服务端 → 客户端（ServerEnvelope）

```json
{
  "eventId": 123,
  "sessionId": "abc",
  "event": { "type": "...", "timestamp": 1690000000000, ... }
}
```

### 心跳

- 客户端应用层 ping：15s 一次，`{"type":"ping"}`
- 协议层 pingInterval：20s（`IOWebSocketChannel.connect(uri, pingInterval: Duration(seconds: 20))`）
- 服务端可能回 `{"type":"pong"}` 或 `{"type":"ping"}`，客户端忽略

### 重连

- 固定 3 秒间隔
- 重连后若 `已初始化 && lastEventId > 0`，发 `session.resync` 请求增量

---

## 附录 C：事件类型速查

| type wire | 事件类 | 关键字段 |
|---|---|---|
| `session.init` | SessionInitEvent | sessionId, cliSessionId?, cli, model?, cwd |
| `message` | MessageEvent | role, blocks: List<ContentBlock> |
| `streaming.delta` | StreamingDeltaEvent | messageId, textDelta? |
| `streaming.done` | StreamingDoneEvent | messageId |
| `tool.use` | ToolUseEvent | toolUseId, toolName, input |
| `tool.result` | ToolResultEvent | toolUseId, output: String, isError? |
| `file.change` | FileChangeEvent | change: FileChange, toolUseId? |
| `command.exec` | CommandExecEvent | command, cwd?, exitCode?, stdout?, stderr?, toolUseId? |
| `todo.update` | TodoUpdateEvent | todos: List<TodoItem> |
| `thinking` | ThinkingEvent | text |
| `progress` | ProgressEvent | step?, total?, message? |
| `error` | ErrorEvent | message, recoverable?, cli? |
| `session.end` | SessionEndEvent | stats: SessionStats? |

### ContentBlock 类型

| type wire | 类 | 字段 |
|---|---|---|
| `text` | TextBlock | text |
| `thinking` | ThinkingBlock | text, signature? |
| `image` | ImageBlock | mimeType, dataB64 |
| `tool_use` | ToolUseBlock | toolUseId, toolName, input |
| `tool_result` | ToolResultBlock | toolUseId, output, isError? |

### CliKind wire 值

| 枚举 | wire |
|---|---|
| claudeCode | `claude-code` |
| codex | `codex` |
| gemini | `gemini` |
| trae | `trae` |
| opencode | `opencode` |
| lingma | `lingma` |
| pty | `pty` |

### 枚举 wire 值汇总

| 枚举 | 值 |
|---|---|
| SshAuthKind | `password` / `privateKey` / `agent` |
| ServerStatus | `ok` / `fail` / `unknown` |
| TaskStatus | `planned` / `in_progress` / `done` / `archived` |
| SessionStatus | `running` / `idle` / `ended` / `error` |
| FileChangeAction | `create` / `edit` / `delete` |
| TodoStatus | `pending` / `in_progress` / `completed` |
| WsConnectionState | disconnected / connecting / connected / reconnecting（无 wire，仅内部使用） |

---

**文档结束**

按 §20 的 M1→M10 顺序实现即可交付完整的 HeyCode App。所有协议细节、UI 规范、平台配置、构建流程均已包含，无需额外参考。
