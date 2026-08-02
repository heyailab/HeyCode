# HeyCode 后端服务开发文档

> 本文档供 AI 开发者使用，目标是基于 Go 语言从零开发一个后端服务，为移动端 App 提供 REST API 与 WebSocket 实时事件流。后端通过 SSH 接管远端服务器上的 AI CLI（claude-code/codex/opencode/trae 等），把 CLI 的输出结构化为统一事件流推送给 App。
>
> **核心约束**：移动端 App 的协议契约（REST 路径、响应体形态、WS 消息格式、事件 schema、枚举 wire 值）是**硬性规范**，必须逐字段对齐，不得自行调整。

---

## 0. 阅读顺序

1. §1 项目概述 —— 理解系统做什么
2. §2 移动端协议契约 —— **最关键**，所有 API/WS/事件格式以此为唯一依据
3. §3 技术方案 —— Go 技术栈选型与目录结构
4. §4 关键实现要点 —— 踩坑预警
5. §5 任务分解 —— 按 Milestone 顺序实现，每步有验收点
6. §6 部署方案 —— 编译、systemd、部署脚本

---

## 1. 项目概述

### 1.1 系统定位
HeyCode 是一个"AI 编程助手远程接管"系统：
- 用户在手机 App 上配置远端 SSH 服务器
- 后端通过 SSH 连接到服务器，启动 AI CLI（claude-code、codex 等）
- CLI 在远端工作目录里执行编程任务（读写文件、跑命令）
- 后端解析 CLI 输出，结构化为统一事件流，通过 WebSocket 实时推送给 App
- App 渲染消息、工具调用、文件变更、命令日志、Todo 进度等

### 1.2 架构
```
┌─────────────┐      REST + WS       ┌─────────────┐        SSH         ┌─────────────┐
│  Flutter App │ ◄──────────────────► │  Go 后端    │ ◄────────────────► │  远端服务器  │
│  (mobile/)   │   HTTP :8787         │ (backend-go/│   22 端口          │  AI CLI 进程 │
└─────────────┘                      └─────────────┘                    └─────────────┘
                                           │
                                           ▼
                                     ┌─────────────┐
                                     │  SQLite DB  │
                                     │  (数据持久化) │
                                     └─────────────┘
```

### 1.3 核心能力
1. **服务器管理**：SSH 服务器 CRUD + 连接测试
2. **项目/任务管理**：项目绑定服务器+工作目录，任务归属项目
3. **SFTP 文件操作**：浏览、读、写、删除远端文件
4. **API Key 管理**：6 个 CLI 的 API Key 加密存储
5. **CLI 会话**：通过 WebSocket 启动/交互/中断/结束会话
6. **实时事件流**：13 种统一事件类型，支持断线重连与历史回放
7. **文件快照与回滚**：记录文件变更，基于 git 回滚

### 1.4 非目标
- 不开发移动端 App（已存在，仅对接）
- 不引入用户鉴权（当前契约无）
- 不实现 gemini/lingma 专用适配器（用 pty 降级模式兜底）

---

## 2. 移动端协议契约（硬性规范）

### 2.1 数据模型

所有实体的 ID 是字符串（cuid 格式，24 字符）。时间字段在 DB 实体中序列化为 **ISO8601 字符串**，在事件中序列化为 **毫秒整数**。

| 实体 | 主键 | 关键字段 | 关系 |
|---|---|---|---|
| Server | id | name, host, port(默认22), username, authKind, encryptedAuth, lastStatus(默认"unknown"), lastCheckedAt, createdAt | → projects[] |
| Project | id | serverId, name, cwd, defaultCli, defaultModel, rules, createdAt | → server(cascade), tasks[] |
| Task | id | projectId, title, description, status(默认"planned"), createdAt, updatedAt | → project(cascade), sessions[] |
| Session | id | taskId(可空), cliSessionId, cli, model, status(默认"running"), createdAt, endedAt | → task?(cascade), events[], fileSnapshots[] |
| Event | id(autoincrement) | sessionId, eventId, payload(JSON string), createdAt | → session(cascade) |
| FileSnapshot | id | sessionId, path, action, diff, addedLines, removedLines, createdAt | → session(cascade) |
| ApiKey | cli(主键) | cipherText, iv, tag, last4, updatedAt | — |

**字段语义**：
- `Server.authKind`：`password` / `privateKey` / `agent`
- `Server.encryptedAuth`：AES-256-GCM 密文的 JSON 字符串 `{iv, tag, cipherText}`（均 hex 编码），绝不返回明文
- `Server.lastStatus`：`ok` / `fail` / `unknown`
- `Project.defaultCli` / `Session.cli`：见 §2.5 枚举
- `Task.status`：`planned` / `in_progress` / `done` / `archived`
- `Session.status`：`running` / `idle` / `ended` / `error`。**REST 创建 Session 时 status=idle，WS 启动会话时 status=running**
- `Session.cliSessionId`：远端 CLI 返回的会话 ID，用于多轮续接
- `Session.taskId`：可空，WS 直接启动会话时为 null
- `Event.eventId`：每会话独立单调递增的序号（从 1 开始），**唯一约束 (sessionId, eventId)**
- `Event.payload`：UnifiedEvent 的 JSON 字符串
- `FileSnapshot.action`：`create` / `edit` / `delete`
- `ApiKey.cli`：见 §2.5 枚举（不含 pty）

### 2.2 加密方案

- **算法**：AES-256-GCM，IV 12 字节随机
- **密钥来源**：`MASTER_KEY` 环境变量，必须是 32 字节的 hex 字符串（64 字符）
- `encrypt(plaintext)` → `{iv, tag, cipherText}`（均 hex）
- `decrypt({iv, tag, cipherText})` → plaintext
- Server.encryptedAuth 存 `JSON.stringify({iv, tag, cipherText})`（单列）
- ApiKey 拆成 cipherText/iv/tag 三列 + last4
- **dev 兜底**：MASTER_KEY 为占位符 `replace_me_with_32_bytes_hex_string` 时，生成临时内存密钥（重启失效，打印警告）
- Go 实现：`crypto/aes` + `crypto/cipher.NewGCM`，`gcm.Seal(nil, iv, plaintext, nil)` / `gcm.Open(nil, iv, ciphertext, tag)`

### 2.3 REST API 端点

通用约定：
- 所有成功响应 HTTP 200
- 错误响应 `{"error":"..."}` 或 `{"error":{"formErrors":[],"fieldErrors":{...}}}`（校验失败）
- 列表端点返回裸 JSON 数组（除特别注明的包装端点）
- 时间字段为 ISO8601 字符串

#### 2.3.1 Health
| 方法 | 路径 | 响应 |
|---|---|---|
| GET | `/api/health` | `{"ok":true,"version":"0.2.0"}` |

#### 2.3.2 Servers
SshAuth 是判别联合：`{kind:"password",password}` | `{kind:"privateKey",privateKey,passphrase?}` | `{kind:"agent"}`。

| 方法 | 路径 | 请求 | 响应 |
|---|---|---|---|
| GET | `/api/servers` | query: projectId?(忽略，保留参数) | Server[]（createdAt desc） |
| POST | `/api/servers` | `{name,host,port?,username,auth:SshAuth}` | Server |
| GET | `/api/servers/:id` | — | Server / 404 |
| PATCH | `/api/servers/:id` | Partial<上述> | Server / 404 |
| DELETE | `/api/servers/:id` | — | `{ok:boolean}` |
| POST | `/api/servers/:id/test` | — | `{ok:true,latencyMs}` 或 `{ok:false,error}` |

**Server DTO**（绝不返回明文凭据）：`{id,name,host,port,username,authKind,createdAt,lastStatus?,lastCheckedAt?}`。

#### 2.3.3 Files (SFTP)
| 方法 | 路径 | 请求 | 响应 |
|---|---|---|---|
| GET | `/api/servers/:id/files` | query: path(必填) | `{path, entries:FileEntry[]}` / 400 缺 path |
| GET | `/api/servers/:id/files/content` | query: path | `{path, content, size}` |
| PUT | `/api/servers/:id/files/content` | body: `{path, content}` | `{path, size}` |
| DELETE | `/api/servers/:id/files` | body: `{path}` | `{ok:true}` |

FileEntry：`{name, path(绝对), isDir, size, modifiedAt(ISO)}`。

#### 2.3.4 Projects
| 方法 | 路径 | 请求 | 响应 |
|---|---|---|---|
| GET | `/api/projects` | query: serverId? | Project[] |
| POST | `/api/projects` | `{serverId,name,cwd,defaultCli,defaultModel?,rules?}` | Project |
| GET | `/api/projects/:id` | — | Project / 404 |
| PATCH | `/api/projects/:id` | Partial<上述> | Project / 404 |
| DELETE | `/api/projects/:id` | — | `{ok:boolean}` |

#### 2.3.5 Tasks
| 方法 | 路径 | 请求 | 响应 |
|---|---|---|---|
| GET | `/api/projects/:id/tasks` | — | Task[] |
| POST | `/api/tasks` | `{projectId,title,description?}` | Task |
| GET | `/api/tasks/:id` | — | Task / 404 |
| PATCH | `/api/tasks/:id` | `{title?,description?,status?}` | Task / 404 |
| DELETE | `/api/tasks/:id` | — | `{ok:boolean}` |

#### 2.3.6 Sessions
| 方法 | 路径 | 请求 | 响应 |
|---|---|---|---|
| GET | `/api/tasks/:id/sessions` | — | Session[] |
| POST | `/api/sessions` | `{taskId,cli,model?}` | Session（status=idle） |
| GET | `/api/sessions/:id` | — | Session / 404 |
| GET | `/api/sessions/:id/events` | query: since?(int) | **`{events: ServerEnvelope[]}`** |
| DELETE | `/api/sessions/:id` | — | `{ok:boolean}` |

Session DTO：`{id,taskId,cliSessionId?,cli,model?,status,createdAt,endedAt?}`。

#### 2.3.7 FileSnapshots / Rollback
| 方法 | 路径 | 请求 | 响应 |
|---|---|---|---|
| GET | `/api/sessions/:sessionId/snapshots` | — | **`{snapshots: FileSnapshot[]}`** |
| GET | `/api/sessions/:sessionId/snapshots/by-path` | query: path(必填) | `{snapshots: FileSnapshot[]}` |
| POST | `/api/snapshots/:snapshotId/rollback` | body: `{serverId, cwd}` | **`{result: RollbackResult}`** |
| POST | `/api/sessions/:sessionId/rollback` | body: `{serverId, cwd}` | **`{results: RollbackResult[]}`** |

FileSnapshot DTO：`{id,sessionId,path,action,diff?,addedLines?,removedLines?,createdAt}`。
RollbackResult：`{snapshotId,path,action,rolled,method,message}`，method ∈ `git-checkout`/`git-clean`/`skip`。

#### 2.3.8 API Keys
cli 枚举：claude-code/codex/gemini/trae/opencode/lingma（**不含 pty**）。

| 方法 | 路径 | 请求 | 响应 |
|---|---|---|---|
| GET | `/api/api-keys` | — | ApiKeyMeta[]（对 6 个支持的 cli 逐个映射，缺失则 hasKey=false） |
| POST | `/api/api-keys` | `{cli, key}` | ApiKeyMeta |
| DELETE | `/api/api-keys/:cli` | — | `{ok:boolean}` / 400 不支持的 cli |

ApiKeyMeta：`{cli, hasKey, last4?, updatedAt?}`。

### 2.4 WebSocket 协议

#### 2.4.1 连接
- URL：`ws://<host>:8787/ws/sessions/:sessionId`
- 文本帧 JSON，maxPayload 16MB
- 协议级 ping/pong 由客户端 20s 发起，后端用 `gorilla/websocket` 默认支持
- 客户端额外每 15s 发应用层心跳 `{type:"ping"}`，后端可回 `{type:"pong"}`（不强制）

#### 2.4.2 客户端 → 服务端（ClientCommand，用 `kind` 字段判别）
| kind | 字段 | 处理 |
|---|---|---|
| `session.start` | serverId, cwd, cli, prompt, model?, resumeCliSessionId?, allowedTools? | 建 Session(status=running) → 订阅事件总线 → 异步启动 CLI 进程 |
| `session.send` | prompt | claude-code/trae: 写 stdin；codex/opencode/pty: 重启进程续接 |
| `session.interrupt` | — | 写 `\x03` |
| `session.end` | — | 杀进程 + status=ended + cleanup |
| `session.resync` | sinceEventId(int) | 回放 eventId > sinceEventId 的事件 |

校验失败发裸错误帧 `{error:"无效指令..."}`。

#### 2.4.3 服务端 → 客户端（ServerEnvelope）
```json
{"eventId": 1, "sessionId": "xxx", "event": {"type": "...", "timestamp": 1700000000000}}
```
错误帧（非信封）：`{error:"..."}`。

#### 2.4.4 eventId 机制
- 每会话独立单调递增，从 1 开始
- counter 懒加载：首次 publish 时从 DB 取 `max(eventId)`
- **per-session 串行锁**保证单调且不重
- DB 唯一约束 `(sessionId, eventId)`
- 持久化：`Event` 表 payload 存 `JSON.stringify(event)`
- `file.change` 事件额外同步写 `FileSnapshot` 表

#### 2.4.5 断线重连
- WS：重连后发 `session.resync` + `sinceEventId`，后端回放历史事件（不重启 CLI 进程）
- REST：`GET /api/sessions/:id/events?since=N` 返回 `{events:[]}`
- **关键语义**：WS 连接关闭时后端会 `endSession`（杀进程）。重连后只能看历史，无法继续实时流。这是设计行为。

### 2.5 统一事件类型（UnifiedEvent）

所有事件共享 `timestamp: int`（毫秒 epoch）和 `type: string`。未知 type 在 App 端会被包装成 ErrorEvent，所以**不要发明新 type**。

#### 2.5.1 枚举 wire 值（必须逐字对齐）
- **CliKind**：`claude-code`、`codex`、`gemini`、`trae`、`opencode`、`lingma`、`pty`
- **SshAuthKind**：`password`、`privateKey`、`agent`
- **ServerStatus**：`ok`、`fail`、`unknown`
- **TaskStatus**：`planned`、`in_progress`、`done`、`archived`
- **SessionStatus**：`running`、`idle`、`ended`、`error`
- **FileChangeAction**：`create`、`edit`、`delete`
- **TodoStatus**：`pending`、`in_progress`、`completed`

#### 2.5.2 ContentBlock（message.blocks 元素）
| type | 字段 |
|---|---|
| `text` | text |
| `thinking` | text, signature? |
| `image` | mimeType, dataB64 |
| `tool_use` | toolUseId, toolName, input(任意 JSON) |
| `tool_result` | toolUseId, output(string 或 `{type:"json",json}` 或 `{type:"image",dataB64}`), isError? |

#### 2.5.3 事件清单
| type | 字段 | 触发时机 |
|---|---|---|
| `session.init` | sessionId, cliSessionId?, cli, model?, cwd | CLI 进程就绪 |
| `message` | role("user"/"assistant"), blocks:ContentBlock[] | 一条完整消息 |
| `streaming.delta` | messageId, textDelta? | 流式文本增量 |
| `streaming.done` | messageId | 流式消息结束 |
| `tool.use` | toolUseId, toolName, input | 工具调用开始 |
| `tool.result` | toolUseId, output, isError? | 工具返回 |
| `file.change` | change:FileChange, toolUseId? | 文件增删改 |
| `command.exec` | command, cwd?, exitCode?, stdout?, stderr?, toolUseId? | shell 命令执行 |
| `todo.update` | todos:TodoItem[] | TodoList 变更（整体替换） |
| `thinking` | text | 模型思考 |
| `progress` | step?, total?, message? | 步骤进度 |
| `error` | message, recoverable?, cli? | 出错；recoverable=false 致命 |
| `session.end` | stats?:{costUsd?, durationMs?, numTurns?, inputTokens?, outputTokens?} | 会话结束 |

- FileChange：`{path(绝对), action, diff?, addedLines?, removedLines?}`
- TodoItem：`{id(会话内稳定), content, status, progress?}`

### 2.6 CLI 适配器规范

后端需要实现 6 个 CLI 适配器 + 1 个 mock 适配器，每个适配器负责构造启动命令、解析 CLI 输出行、构造多轮输入。

#### 2.6.1 Adapter 接口
```go
type Adapter interface {
    Kind() CliKind
    BuildStartCommand(opts BuildCommandOpts) StartCommand
    ParseLine(line string, ctx *ParseContext) []UnifiedEvent
    BuildUserInput(prompt string) string  // 不支持多轮返回 ""
}

// 可选方法，存在则走 Mock 路径
type TimelineGenerator interface {
    GenerateTimeline(prompt string) []string
}

type BuildCommandOpts struct {
    Cwd               string
    Prompt            string
    Model             string
    ResumeCliSessionId string
    AllowedTools      []string
}

type StartCommand struct {
    Command string
    Args    []string
    Env     map[string]string
}
```

#### 2.6.2 ParseContext（per-session 解析状态）
```go
type ParseContext struct {
    SessionId         string
    Cwd               string
    Cli               CliKind
    Model             string
    PendingToolUseIds []string                          // FIFO，关联 tool_result
    ToolUseIndex      map[string]struct{ Name, Input }  // tool_result 到达时识别特殊工具
    CurrentMessageId  string
    CurrentRole       string
    CurrentBlocks     []ContentBlock                    // 流式适配器累积片段
}
```

#### 2.6.3 适配器实现矩阵

| 适配器 | 命令构造 | 多轮机制 | 解析要点 |
|---|---|---|---|
| **claude-code** | `claude -p --output-format stream-json --input-format stream-json --verbose --cd <cwd>` + 可选 `--model`/`--resume <sid>`/`--allowedTools <逗号分隔>`。prompt 走 stdin | stdin NDJSON：`{"type":"user","message":{"role":"user","content":[{"type":"text","text":<prompt>}]}}\n` | `system/init`→session.init；`user/assistant`→message + mapContent 衍生；`tool_use` 入队 + 文件工具立即发 file.change + TodoWrite→todo.update；`tool_result`→tool.result + Bash 衍生 command.exec；`result`→非 success 发 error(recoverable=false) + session.end(stats) |
| **trae** | 同 claude-code，仅 command 改为 `trae` | 同 claude-code | **完全继承 claude-code 的 parseLine/buildUserInput**，仅 Kind() 返回 trae |
| **codex** | `codex exec --json --full-auto --skip-git-repo-check --cd <cwd> [--model] [resume <sid>] "<prompt>"`。prompt 作参数 | 不支持 stdin 多轮，靠 `codex exec resume <sid> "<prompt>"` 重启 | `thread.started`→session.init；`turn.started`→progress + 初始化累积；`item.completed` 分发：agent_message→streaming.delta，reasoning→thinking，command_execution→tool.use+command.exec(started) / tool.result+command.exec(completed)，file_change→遍历 changes 发多个 file.change，todo_list/plan_update→todo.update；`turn.completed`→flush message + streaming.done + session.end |
| **opencode** | `opencode run --format json --dangerously-skip-permissions --cwd <cwd> [--model] [--continue <sid>] "<prompt>"` | 不支持 stdin，靠 `--continue <sid>` 重启 | `step_start`→首次发 session.init + progress；`text`→streaming.delta 累积；`reasoning`→thinking；`tool_start`→tool.use + write/edit→file.change + bash→command.exec；`tool_finish`→FIFO shift 匹配 toolUseId + tool.result + bash 补 command.exec；`step_finish`→flush + streaming.done + session.end |
| **pty** | `lingma --cwd <cwd> "<prompt>"`，spawn 时 pty:true | 无续接（重启丢上下文） | 降级模式：stripAnsi 剥离颜色码；首行发 session.init+progress+command.exec；后续每行 command.exec；约定 `__PTY_END__`→streaming.done+session.end |
| **mock** | 仅 MOCK_CLI=1 且 cli=claude-code 时生效，复用 claude-code adapter，但 buildStartCommand 返回占位 | — | process-runner 检测 GenerateTimeline 方法存在即走 Mock 路径，用 25ms delay 逐行喂 parseLine |
| **gemini/lingma** | 抛错 | — | 工厂对这两种 kind 返回 error，引导用 pty 兜底 |

#### 2.6.4 适配器三种模式对比
| 模式 | CLI | prompt 传递 | 多轮 | flush 时机 |
|---|---|---|---|---|
| stream-json 进程型 | claude-code/trae | stdin | stdin NDJSON + --resume | 单行即发 message |
| NDJSON 进程型 | codex/opencode | 命令行参数 | 重启进程 | 回合结束 flush |
| PTY 终端型 | pty | 命令行参数, pty:true | 无续接 | 每行原样 command.exec |

### 2.7 SSH 层规范

#### 2.7.1 连接池
- 基于 `golang.org/x/crypto/ssh`
- `pool: map[serverId]*entry`，entry 含 client、ready、config
- **保活**：keepaliveInterval 30s，readyTimeout 15s
- **复用**：acquire 时若 ready 直接返回，release 是 no-op
- **配置变更检测**：比较 host/port/username/auth JSON，变更时关闭旧连接
- **重连**：连接断开置 ready=false，下次 acquire 自动重连
- **auth**：password→`ssh.Password`；privateKey→`ssh.PublicKeys(signer)` + passphrase；agent→`ssh.Dial` with `ssh.AgentClient`

#### 2.7.2 exec 与 spawnStream
- `exec(serverId, command, opts)`：拼接 `cd <shellQuote(cwd)> && <command>`，收集 stdout/stderr，exit 事件取 exitCode（null→0），支持 timeoutMs
- `spawnStream(serverId, command, args, opts)`：返回 stream（ssh.Session），用于 CLI 进程拉起。pty 适配器 opts.pty=true 调 `session.RequestPty`
- **shellQuote**：空串→`''`；纯 `[A-Za-z0-9@%+=:,./_-]` 原样；否则单引号包裹并转义内部单引号 `'\''`

#### 2.7.3 SFTP
- `sftp.NewClient` 包装，方法：listdir、read、write、delete、mkdir、stat
- listdir 用 `ReadDir`，按 mode 判 isDir（`os.FileMode&os.ModeDir != 0`），mtime 转 ISO
- read/write 用流

### 2.8 关键业务流程

#### 2.8.1 创建会话（WS 主流程）
1. App 连 `ws://host:8787/ws/sessions/:sessionId`
2. 发 `{kind:"session.start", serverId, cwd, cli, prompt, model?, resumeCliSessionId?, allowedTools?}`
3. 后端：`startSession({taskId:null, cli, model})` 建 Session(status=running) → 订阅事件总线 → **异步**启动 `runPrompt`
4. `runPrompt` 内部：
   - update status=running
   - `getAdapter(cli)`
   - 订阅事件总线监听 session.init → 写回 cliSessionId 到 DB
   - `runCli` → `runReal`：
     - `adapter.buildStartCommand` → `sshPool.spawnStream`
     - data 按行切 → `adapter.parseLine` → 逐事件 `eventBus.publish`（分配 eventId + 写 Event 表 + file.change 写 FileSnapshot + 通知订阅者）
     - 写入首条 prompt `stream.write(adapter.buildUserInput(prompt))`
   - 进程结束：未收到 session.end 则补发 error + session.end
   - 成功 → status=idle；失败 → status=error + publish error
5. 事件通过订阅实时推送给 App

#### 2.8.2 多轮续接
- **stdin 多轮型**（claude-code/trae）：进程长驻，`adapter.buildUserInput(prompt)` → 写 stdin
- **重启进程型**（codex/opencode/pty）：
  - `continuationInFlight` 守卫防并发
  - 查 session，校验有 cliSessionId（pty 例外）
  - 复用 runPrompt，把 DB 的 cliSessionId 作为 resumeCliSessionId 传入
  - 适配器 buildStartCommand 构造续接参数（--resume / resume / --continue）
  - 重启进程

#### 2.8.3 文件变更检测与回滚
- **触发**：适配器 parseLine 识别文件类工具即发 file.change
- **持久化**：事件总线检测 file.change → 写 FileSnapshot 表
- **回滚**：依赖远端 git 仓库
  - action=create → `git clean -f -- <relPath>`（method=git-clean）
  - action=edit/delete → `git checkout HEAD -- <relPath>`（method=git-checkout）
  - 非 git 仓库 → method=skip, rolled=false
  - relPath = 去掉 cwd 前缀，特殊字符单引号包裹

---

## 3. 技术方案

### 3.1 技术栈选型
| 用途 | 库 | 说明 |
|---|---|---|
| HTTP 路由 | `github.com/go-chi/chi/v5` | 轻量、中间件友好、path param 清晰 |
| WebSocket | `github.com/gorilla/websocket` | 成熟稳定，支持 ping/pong |
| SSH | `golang.org/x/crypto/ssh` | 官方库 |
| SFTP | `github.com/pkg/sftp` | 配合 x/crypto/ssh |
| SQLite | `modernc.org/sqlite` | 纯 Go 实现，无需 CGO，交叉编译友好 |
| DB 迁移 | `github.com/pressly/goose/v3` | 嵌入 SQL 迁移文件 |
| 日志 | `log/slog`（标准库） | Go 1.21+，结构化日志 |
| .env 加载 | `github.com/joho/godotenv` | 可选，开发期方便 |
| ID 生成 | 自实现 cuid | ~30 行，保证 24 字符格式 |

### 3.2 目录结构
```
backend-go/
├── cmd/
│   └── heycode-backend/
│       └── main.go              # 入口：加载配置、启动 HTTP server、信号处理
├── internal/
│   ├── config/                  # 配置加载（env + 可选 .env）
│   │   └── config.go
│   ├── crypto/                  # AES-256-GCM 加解密
│   │   └── aes_gcm.go
│   ├── db/                      # DB 连接 + 迁移
│   │   ├── db.go
│   │   └── migrations/
│   │       └── 001_init.sql     # 建表
│   ├── store/                   # 数据访问层（每实体一个文件）
│   │   ├── server.go
│   │   ├── project.go
│   │   ├── task.go
│   │   ├── session.go
│   │   ├── event.go
│   │   ├── snapshot.go
│   │   └── apikey.go
│   ├── ssh/                     # SSH 连接池 + exec + spawnStream
│   │   ├── pool.go
│   │   ├── exec.go
│   │   └── sftp.go
│   ├── adapter/                 # CLI 适配器
│   │   ├── adapter.go           # interface + 工厂
│   │   ├── claudecode.go
│   │   ├── trae.go
│   │   ├── codex.go
│   │   ├── opencode.go
│   │   ├── pty.go
│   │   └── mock.go
│   ├── runner/                  # process-runner（runCli/runMock）
│   │   └── runner.go
│   ├── eventbus/                # 事件总线（订阅/发布/串行锁/持久化）
│   │   └── bus.go
│   ├── service/                 # 业务服务层
│   │   ├── server.go
│   │   ├── project.go
│   │   ├── task.go
│   │   ├── session.go
│   │   └── snapshot.go
│   ├── transport/               # HTTP + WS 传输层
│   │   ├── http/
│   │   │   ├── server.go        # chi router 装配
│   │   │   ├── handler_health.go
│   │   │   ├── handler_servers.go
│   │   │   ├── handler_files.go
│   │   │   ├── handler_projects.go
│   │   │   ├── handler_tasks.go
│   │   │   ├── handler_sessions.go
│   │   │   ├── handler_snapshots.go
│   │   │   └── handler_apikeys.go
│   │   └── ws/
│   │       └── handler.go       # WS 连接处理
│   └── types/                   # 共享类型（事件、DTO、枚举）
│       ├── event.go
│       ├── dto.go
│       └── enum.go
├── go.mod
├── go.sum
└── Makefile                     # build / cross-build / test
```

### 3.3 配置加载
```go
type Config struct {
    Port        int    // 默认 8787
    DatabaseUrl string // "file:./dev.db" 或 sqlite 路径
    MasterKey   string // 64 hex chars
    JwtSecret   string // 保留字段，min 8
    LogLevel    string // 默认 info
    MockCli     bool   // 默认 false
}
```
- 从环境变量读，可选支持 `.env` 文件
- MasterKey 占位符检测 → 生成临时内存密钥 + 警告日志

### 3.4 ID 生成
自实现 cuid（24 字符），保证 ID 格式一致。简易实现参考算法：时间戳 + 计数器 + 随机数 + base36 编码。

### 3.5 数据库迁移
SQL 迁移文件嵌入二进制（`go:embed`），启动时自动执行 `goose.Up`。

`001_init.sql` 建表：
- 表名用复数：servers/projects/tasks/sessions/events/file_snapshots/api_keys
- sessions.taskId 可空
- events 唯一约束 (sessionId, eventId) + 索引
- file_snapshots 索引 sessionId

---

## 4. 关键实现要点

### 4.1 事件顺序保证
- process-runner 按行切 → **串行处理**（用 channel 或 mutex），不要并发 parseLine
- eventbus.publish **per-session 串行锁**，counter 懒加载自 DB max
- Go 用 `sync.Mutex` per sessionId，或用 per-session channel 做串行化

### 4.2 流式累积与 flush
- opencode/codex 在 CurrentBlocks 累积片段，每片段发 streaming.delta，回合结束 flush message + streaming.done
- claude-code/trae 单行即发 message（无累积）
- 实现时 ParseContext 的 CurrentBlocks 要小心，回合结束必须清空

### 4.3 工具关联
- claude-code：精确 tool_use_id
- opencode：FIFO 队列 shift 匹配（pendingToolUseIds）
- codex：item.completed 直接带完整信息，无需队列

### 4.4 衍生事件
- claude-code 的 Bash tool_result → 补发 command.exec（isError→exitCode=1, stderr；否则 exitCode=0, stdout）
- claude-code 的 Write/Edit/MultiEdit → 立即发 file.change（统计 addedLines/removedLines）
- opencode 的 bash tool_start → command.exec（无 exitCode）；tool_finish → command.exec（带 exit_code）
- codex 的 file_change item.completed → 遍历 changes 数组发多个 file.change

### 4.5 WS 连接关闭行为
- close 事件 → 取消事件总线订阅 → 若有 sessionId 调 endSession（杀进程 + status=ended + cleanup）
- App 重连后只能看历史，无法继续实时流。这是设计行为。

### 4.6 shellQuote
```go
func shellQuote(s string) string {
    if s == "" {
        return "''"
    }
    if regexp.MustCompile(`^[A-Za-z0-9@%+=:,./_-]+$`).MatchString(s) {
        return s
    }
    return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
```

### 4.7 ANSI 剥离（pty 适配器）
```go
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
func stripAnsi(s string) string {
    return ansiRegex.ReplaceAllString(s, "")
}
```

### 4.8 时间字段序列化
- DB 实体（Server/Project/Task/Session/FileSnapshot/ApiKey）：`createdAt`/`updatedAt`/`endedAt`/`lastCheckedAt`/`modifiedAt` 序列化为 **ISO8601 字符串**
- 事件 `timestamp`：序列化为 **毫秒整数**
- Go 用 `time.Time`，JSON tag 控制格式；事件 timestamp 用 `time.Now().UnixMilli()`

### 4.9 错误响应格式
- 简单错误：`{"error":"服务器不存在"}` + 对应 HTTP 状态码（400/404/500）
- 校验错误：`{"error":{"formErrors":[],"fieldErrors":{"name":["必填"]}}}` + 400
- App 解析逻辑：response.data 是 Map 且含 error 字段 → 用之；否则用字符串；否则用"网络错误"

### 4.10 健康检查
- `/api/health` 返回 `{"ok":true,"version":"0.2.0"}`
- App 设置页"测试连接"读 `version` 字段显示

### 4.11 pty 适配器特例
- `runContinuation` 对 pty 放行：无 cliSessionId 也允许重启
- spawn 时 pty:true，调 `session.RequestPty`
- 降级模式无结构化解析，约定 `__PTY_END__` 标记结束

### 4.12 Mock 模式
- 仅 MOCK_CLI=1 且 cli=claude-code 时生效
- 工厂返回的 adapter 实现了 GenerateTimeline 接口
- runner 检测该接口存在 → 走 runMock 路径（不连 SSH），用 25ms delay 逐行喂 parseLine
- Mock timeline 产出 claude-code stream-json 模拟序列：system init → assistant text → assistant tool_use(Bash) → user tool_result → assistant text → result success

### 4.13 codex schema 漂移兼容
codex CLI 输出的 JSON 可能用 `item.type` 或旧版 `item.item_type`，且旧版 `assistant_message` 需归一为 `agent_message`。`getItemType` 兼容两种字段名。

### 4.14 REST 与 WS 创建 Session 的 status 差异
- REST `POST /api/sessions` 创建的 Session status=`idle`
- WS `session.start` 创建的 Session status=`running`

---

## 5. 任务分解（Milestone）

### M1：项目骨架与配置
- [ ] 初始化 go module，添加依赖
- [ ] 实现 config 加载（env + .env）
- [ ] 实现 crypto（AES-256-GCM）
- [ ] 实现 db 连接（modernc.org/sqlite）+ 001_init.sql 迁移
- [ ] 实现 chi router + /api/health 端点
- [ ] 实现 graceful shutdown（SIGINT/SIGTERM）
- **验收**：`go build && ./heycode-backend`，`curl http://localhost:8787/api/health` 返回 `{"ok":true,"version":"0.2.0"}`

### M2：数据访问层与基础 CRUD
- [ ] 实现 store 层（server/project/task/apikey）
- [ ] 实现 ID 生成（cuid）
- [ ] 实现 service 层基础 CRUD
- [ ] 实现 HTTP handler（servers/projects/tasks/apikeys）+ 路由注册
- [ ] 实现错误响应格式
- **验收**：用 curl 测试所有 CRUD 端点，响应体形态与 §2.3 一致；Server 的 encryptedAuth 加密存储

### M3：SSH 层与 SFTP
- [ ] 实现 SSH 连接池（保活、复用、重连、配置变更检测）
- [ ] 实现 exec（cd 前缀 + shellQuote + timeout）
- [ ] 实现 spawnStream（pty 选项）
- [ ] 实现 SFTP（listdir/read/write/delete/stat/mkdir）
- [ ] 实现 server.test 端点（exec `echo __ok__`，计 latencyMs）
- [ ] 实现 files 端点（list/read/write/delete）
- **验收**：App 能添加服务器、测试连接、浏览文件、编辑文件、删除文件

### M4：CLI 适配器
- [ ] 实现 Adapter interface + ParseContext
- [ ] 实现 claude-code 适配器（含 mapContent 衍生逻辑）
- [ ] 实现 trae 适配器（继承 claude-code）
- [ ] 实现 codex 适配器（含 schema 漂移兼容）
- [ ] 实现 opencode 适配器（FIFO 队列）
- [ ] 实现 pty 适配器（stripAnsi + __PTY_END__）
- [ ] 实现 mock 适配器（GenerateTimeline）
- [ ] 实现 adapter 工厂（gemini/lingma 抛错）
- [ ] 单元测试：每个适配器用样本行验证 parseLine 输出事件序列
- **验收**：每个适配器有测试用例覆盖关键事件类型；mock 适配器能产出完整 timeline

### M5：事件总线与会话运行器
- [ ] 实现事件总线（subscribe/publish/replay/cleanup + per-session 串行锁 + counter 懒加载 + 持久化）
- [ ] 实现 process-runner（runReal + runMock + sendInput + interrupt + kill）
- [ ] 实现 session.service（startSession/runPrompt/runContinuation/endSession/getEvents）
- [ ] file.change 事件自动写 FileSnapshot
- **验收**：mock 模式下，本地跑通创建会话 → 收到完整事件序列 → 会话结束；eventId 单调递增；DB 有事件记录

### M6：WebSocket 与实时事件流
- [ ] 实现 WS handler（/ws/sessions/:sessionId）
- [ ] 实现 ClientCommand 解析（kind 判别）
- [ ] 实现 ServerEnvelope 推送
- [ ] 实现 session.start/send/interrupt/end/resync
- [ ] 实现 WS close 行为（取消订阅 + endSession）
- [ ] 实现 REST sessions 端点（list/create/get/events/delete）
- **验收**：App 配置 MOCK_CLI=1 的后端，能创建会话、看到实时事件流、发送多轮、中断、断线重连看历史

### M7：文件快照与回滚
- [ ] 实现 snapshot store + service
- [ ] 实现 listBySession / listByPath
- [ ] 实现 rollbackSnapshot（git rev-parse 检测 + git clean / git checkout）
- [ ] 实现 rollbackSession（倒序逐个回滚）
- [ ] 实现 REST 端点（snapshots/by-path/rollback）
- **验收**：App 文件 Tab 能查看变更历史、单条回滚、全部回滚

### M8：真实 CLI 集成测试
- [ ] 准备测试 SSH 服务器（已装 claude/codex/opencode CLI）
- [ ] 端到端测试 claude-code 会话
- [ ] 端到端测试 codex 会话
- [ ] 端到端测试 opencode 会话
- [ ] 验证多轮续接（stdin 型 + 重启型）
- [ ] 验证断线重连
- **验收**：三个主流 CLI 都能在真实环境跑通完整会话流程

### M9：部署与运维
- [ ] 编写 Makefile（build/cross-build/test）
- [ ] 编写 systemd unit 文件
- [ ] 编写部署脚本（scp + systemd restart）
- [ ] 交叉编译 linux/amd64 二进制
- [ ] 部署到服务器，放行 8787
- [ ] App 连接真实后端验证
- **验收**：服务器上 `systemctl start heycode-backend`，App 连接 `http://<server>:8787`，所有功能正常

### M10：回归测试
- [ ] 全量回归测试所有 REST 端点
- [ ] 全量回归测试 WS 协议
- [ ] 内存占用与启动速度测试
- **验收**：内存 < 50MB；启动 < 1s；所有功能无回归

---

## 6. 部署方案

### 6.1 编译
```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o heycode-backend-linux-amd64 ./cmd/heycode-backend
```
- `CGO_ENABLED=0` 确保 modernc.org/sqlite 纯 Go，无动态链接
- `-ldflags="-s -w"` 去掉调试信息，减小体积

### 6.2 systemd unit（`/etc/systemd/system/heycode-backend.service`）
```ini
[Unit]
Description=HeyCode Backend
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/heycode
ExecStart=/opt/heycode/heycode-backend
Environment=PORT=8787
Environment=DATABASE_URL=file:/opt/heycode/data/dev.db
Environment=MASTER_KEY=<64 hex chars>
Environment=LOG_LEVEL=info
Environment=MOCK_CLI=0
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
```

### 6.3 部署脚本
```bash
#!/usr/bin/env bash
# deploy-go.sh —— HeyCode 后端部署
set -euo pipefail

REMOTE="${REMOTE:-root@your-server}"
BIN_NAME="heycode-backend-linux-amd64"
REMOTE_DIR="/opt/heycode"

# 1. 本地编译
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BIN_NAME" ./cmd/heycode-backend

# 2. 上传
ssh "$REMOTE" "mkdir -p $REMOTE_DIR/data"
scp "$BIN_NAME" "$REMOTE:$REMOTE_DIR/heycode-backend"

# 3. 生成 MASTER_KEY（如首次）
ssh "$REMOTE" "test -f $REMOTE_DIR/.env || (echo MASTER_KEY=\$(openssl rand -hex 32) > $REMOTE_DIR/.env && echo '首次生成 .env')"

# 4. 安装 systemd unit（首次）
ssh "$REMOTE" "test -f /etc/systemd/system/heycode-backend.service || cat > /etc/systemd/system/heycode-backend.service <<'UNIT'
[Unit]
Description=HeyCode Backend
After=network.target
[Service]
Type=simple
WorkingDirectory=$REMOTE_DIR
EnvironmentFile=$REMOTE_DIR/.env
Environment=PORT=8787
Environment=DATABASE_URL=file:$REMOTE_DIR/data/dev.db
ExecStart=$REMOTE_DIR/heycode-backend
Restart=always
RestartSec=3
[Install]
WantedBy=multi-user.target
UNIT"

# 5. 重启
ssh "$REMOTE" "systemctl daemon-reload && systemctl restart heycode-backend && systemctl enable heycode-backend"

# 6. 验证
sleep 2
ssh "$REMOTE" "curl -sf http://localhost:8787/api/health && echo '✓ 部署成功'"
```

### 6.4 端口与防火墙
- 默认端口 8787
- 服务器安全组/防火墙放行 8787
- App 后端地址：`http://<服务器IP>:8787`

---

## 7. 开发环境与测试

### 7.1 本地开发
```bash
cd backend-go
go mod tidy
go run ./cmd/heycode-backend  # 默认 :8787，DATABASE_URL=file:./dev.db
```

### 7.2 Mock 模式测试（无需真实 SSH/CLI）
```bash
MOCK_CLI=1 go run ./cmd/heycode-backend
```
此模式下 cli=claude-code 走 MockAdapter，可端到端测试 WS 事件流。

### 7.3 单元测试
```bash
go test ./...
```
重点测试：
- crypto：加解密往返
- adapter：每个适配器 parseLine 输出事件序列
- eventbus：eventId 单调、串行化、持久化
- store：CRUD + 加密字段

### 7.4 集成测试
用移动端 App（mock 模式后端）端到端验证：
1. 启动 Go 后端 `MOCK_CLI=1 ./heycode-backend`
2. App 配置后端地址 `http://<电脑局域网IP>:8787`
3. 创建服务器 → 项目 → 任务 → 会话
4. 验证事件流、多轮、中断、断线重连

---

## 附录 A：完整 REST 端点速查表

```
GET    /api/health                                    → {ok, version}

# Servers
GET    /api/servers                                    → Server[]
POST   /api/servers                                    → Server
GET    /api/servers/:id                                → Server
PATCH  /api/servers/:id                                → Server
DELETE /api/servers/:id                                → {ok}
POST   /api/servers/:id/test                           → {ok, latencyMs} | {ok:false, error}

# Files (SFTP)
GET    /api/servers/:id/files?path=                    → {path, entries:FileEntry[]}
GET    /api/servers/:id/files/content?path=            → {path, content, size}
PUT    /api/servers/:id/files/content                  → {path, size}          body: {path, content}
DELETE /api/servers/:id/files                          → {ok:true}              body: {path}

# Projects
GET    /api/projects?serverId=                         → Project[]
POST   /api/projects                                   → Project
GET    /api/projects/:id                               → Project
PATCH  /api/projects/:id                               → Project
DELETE /api/projects/:id                               → {ok}

# Tasks
GET    /api/projects/:id/tasks                         → Task[]
POST   /api/tasks                                      → Task
GET    /api/tasks/:id                                  → Task
PATCH  /api/tasks/:id                                  → Task
DELETE /api/tasks/:id                                  → {ok}

# Sessions
GET    /api/tasks/:id/sessions                         → Session[]
POST   /api/sessions                                   → Session               body: {taskId, cli, model?}
GET    /api/sessions/:id                               → Session
GET    /api/sessions/:id/events?since=                 → {events: ServerEnvelope[]}
DELETE /api/sessions/:id                               → {ok}

# Snapshots
GET    /api/sessions/:sessionId/snapshots              → {snapshots: FileSnapshot[]}
GET    /api/sessions/:sessionId/snapshots/by-path?path=→ {snapshots: FileSnapshot[]}
POST   /api/snapshots/:snapshotId/rollback             → {result: RollbackResult}    body: {serverId, cwd}
POST   /api/sessions/:sessionId/rollback               → {results: RollbackResult[]} body: {serverId, cwd}

# API Keys
GET    /api/api-keys                                   → ApiKeyMeta[]
POST   /api/api-keys                                   → ApiKeyMeta               body: {cli, key}
DELETE /api/api-keys/:cli                              → {ok}

# WebSocket
WS     /ws/sessions/:sessionId
```

## 附录 B：ClientCommand 与 ServerEnvelope 速查

```
# Client → Server（kind 判别）
{kind:"session.start",   serverId, cwd, cli, prompt, model?, resumeCliSessionId?, allowedTools?}
{kind:"session.send",    prompt}
{kind:"session.interrupt"}
{kind:"session.end"}
{kind:"session.resync",  sinceEventId}

# Server → Client（信封）
{eventId: int, sessionId: string, event: UnifiedEvent}

# 心跳（非信封）
{type:"ping"}
{type:"pong"}

# 错误（非信封）
{error:"..."}
```

## 附录 C：UnifiedEvent 速查

```
session.init     {sessionId, cliSessionId?, cli, model?, cwd, timestamp}
message          {role:"user"|"assistant", blocks:ContentBlock[], timestamp}
streaming.delta  {messageId, textDelta?, timestamp}
streaming.done   {messageId, timestamp}
tool.use         {toolUseId, toolName, input, timestamp}
tool.result      {toolUseId, output, isError?, timestamp}
file.change      {change:FileChange, toolUseId?, timestamp}
command.exec     {command, cwd?, exitCode?, stdout?, stderr?, toolUseId?, timestamp}
todo.update      {todos:TodoItem[], timestamp}
thinking         {text, timestamp}
progress         {step?, total?, message?, timestamp}
error            {message, recoverable?, cli?, timestamp}
session.end      {stats?:{costUsd?, durationMs?, numTurns?, inputTokens?, outputTokens?}, timestamp}

ContentBlock:
  {type:"text", text}
  {type:"thinking", text, signature?}
  {type:"image", mimeType, dataB64}
  {type:"tool_use", toolUseId, toolName, input}
  {type:"tool_result", toolUseId, output, isError?}

FileChange: {path, action:"create"|"edit"|"delete", diff?, addedLines?, removedLines?}
TodoItem:   {id, content, status:"pending"|"in_progress"|"completed", progress?}
```
