-- +goose Up
-- HeyCode 后端初始 schema
-- 时间字段统一为 ISO8601 字符串（Go time.Time 序列化）
-- ID 为 TEXT（cuid 24 字符）
-- events.event_id 每会话独立单调递增，UNIQUE(session_id, event_id) 保证不重

CREATE TABLE servers (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    host            TEXT NOT NULL,
    port            INTEGER NOT NULL DEFAULT 22,
    username        TEXT NOT NULL,
    auth_kind       TEXT NOT NULL,           -- password | privateKey | agent
    encrypted_auth  TEXT NOT NULL,           -- JSON: {iv, tag, cipherText}
    last_status     TEXT NOT NULL DEFAULT 'unknown', -- ok | fail | unknown
    last_checked_at TEXT,
    created_at      TEXT NOT NULL
);

CREATE TABLE projects (
    id            TEXT PRIMARY KEY,
    server_id     TEXT NOT NULL,
    name          TEXT NOT NULL,
    cwd           TEXT NOT NULL,
    default_cli   TEXT NOT NULL,             -- claude-code | codex | gemini | trae | opencode | lingma | pty
    default_model TEXT,
    rules         TEXT,
    created_at    TEXT NOT NULL,
    FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE
);
CREATE INDEX idx_projects_server_id ON projects(server_id);

CREATE TABLE tasks (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL,
    title       TEXT NOT NULL,
    description TEXT,
    status      TEXT NOT NULL DEFAULT 'planned', -- planned | in_progress | done | archived
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);
CREATE INDEX idx_tasks_project_id ON tasks(project_id);

CREATE TABLE sessions (
    id              TEXT PRIMARY KEY,
    task_id         TEXT,                    -- 可空：WS 直接启动会话时为 NULL
    cli_session_id  TEXT,                    -- 远端 CLI 返回的会话 ID，用于多轮续接
    cli             TEXT NOT NULL,
    model           TEXT,
    status          TEXT NOT NULL DEFAULT 'running', -- running | idle | ended | error
    created_at      TEXT NOT NULL,
    ended_at        TEXT,
    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
);
CREATE INDEX idx_sessions_task_id ON sessions(task_id);
CREATE INDEX idx_sessions_status ON sessions(status);

CREATE TABLE events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id  TEXT NOT NULL,
    event_id    INTEGER NOT NULL,            -- 每会话独立单调递增，从 1 开始
    payload     TEXT NOT NULL,               -- JSON.stringify(UnifiedEvent)
    created_at  TEXT NOT NULL,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
    UNIQUE(session_id, event_id)
);
CREATE INDEX idx_events_session_id ON events(session_id);
CREATE INDEX idx_events_session_event ON events(session_id, event_id);

CREATE TABLE file_snapshots (
    id            TEXT PRIMARY KEY,
    session_id    TEXT NOT NULL,
    path          TEXT NOT NULL,             -- 绝对路径
    action        TEXT NOT NULL,             -- create | edit | delete
    diff          TEXT,
    added_lines   INTEGER,
    removed_lines INTEGER,
    created_at    TEXT NOT NULL,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);
CREATE INDEX idx_snapshots_session_id ON file_snapshots(session_id);
CREATE INDEX idx_snapshots_session_path ON file_snapshots(session_id, path);

CREATE TABLE api_keys (
    cli         TEXT PRIMARY KEY,            -- claude-code | codex | gemini | trae | opencode | lingma (不含 pty)
    cipher_text TEXT NOT NULL,
    iv          TEXT NOT NULL,
    tag         TEXT NOT NULL,
    last4       TEXT,
    updated_at  TEXT NOT NULL
);

-- +goose Down
DROP TABLE api_keys;
DROP TABLE file_snapshots;
DROP TABLE events;
DROP TABLE sessions;
DROP TABLE tasks;
DROP TABLE projects;
DROP TABLE servers;
