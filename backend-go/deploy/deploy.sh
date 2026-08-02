#!/usr/bin/env bash
# HeyCode 后端部署脚本
#
# 用法：
#   ./deploy/deploy.sh <user@host> [ssh-port]
#
# 流程：
#   1. 交叉编译 linux/amd64 二进制（CGO_ENABLED=0）
#   2. scp 二进制 + systemd unit + .env 到远端 /opt/heycode/
#   3. 远端创建 heycode 用户（若不存在）、设权限
#   4. systemctl daemon-reload + restart
#   5. 健康检查 /api/health
#
# 前置：
#   - 本地能 ssh 到目标服务器（密钥认证）
#   - 远端有 sudo 权限
#   - deploy/.env 已配置好 MASTER_KEY（生产密钥，勿用占位符）

set -euo pipefail

# ---- 参数 ----
if [ $# -lt 1 ]; then
    echo "用法: $0 <user@host> [ssh-port]"
    echo "示例: $0 deploy@10.0.0.1 22"
    exit 1
fi
TARGET="$1"
SSH_PORT="${2:-22}"

# 路径（脚本位于 backend-go/deploy/，项目根为 backend-go/）
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
APP="heycode-backend"
REMOTE_DIR="/opt/heycode"
SERVICE_FILE="$SCRIPT_DIR/heycode-backend.service"
ENV_FILE="$SCRIPT_DIR/.env"

cd "$PROJECT_DIR"

# ---- 1. 交叉编译 ----
echo "==> 交叉编译 linux/amd64 二进制..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "$APP-linux-amd64" ./cmd/heycode-backend
echo "    产物: $APP-linux-amd64"

# ---- 2. 前置检查 ----
if [ ! -f "$ENV_FILE" ]; then
    echo "错误: $ENV_FILE 不存在，请先配置（参考 .env.example）"
    exit 1
fi
if grep -q "replace_me_with_32_bytes_hex_string" "$ENV_FILE"; then
    echo "错误: $ENV_FILE 中 MASTER_KEY 仍是占位符，请生成生产密钥："
    echo "    openssl rand -hex 32"
    exit 1
fi

SSH_OPTS="-p $SSH_PORT -o StrictHostKeyChecking=accept-new"

# ---- 3. 上传文件 ----
echo "==> 上传文件到 $TARGET:$REMOTE_DIR ..."
ssh $SSH_OPTS "$TARGET" "sudo mkdir -p $REMOTE_DIR && sudo chown -R \$USER:\$USER $REMOTE_DIR"
scp $SSH_OPTS "$APP-linux-amd64" "$TARGET:$REMOTE_DIR/$APP"
scp $SSH_OPTS "$SERVICE_FILE" "$TARGET:/tmp/heycode-backend.service"
scp $SSH_OPTS "$ENV_FILE" "$TARGET:$REMOTE_DIR/.env"

# ---- 4. 安装 systemd unit + 启动 ----
echo "==> 安装 systemd 服务并重启..."
ssh $SSH_OPTS "$TARGET" bash <<'REMOTE'
set -e
# 创建专用用户（若不存在）
if ! id heycode &>/dev/null; then
    sudo useradd -r -s /usr/sbin/nologin -d /opt/heycode heycode
fi
# 设置权限
sudo chown -R heycode:heycode /opt/heycode
sudo chmod 750 /opt/heycode
sudo chmod 600 /opt/heycode/.env
sudo chmod 755 /opt/heycode/heycode-backend
# 安装 unit
sudo mv /tmp/heycode-backend.service /etc/systemd/system/heycode-backend.service
sudo systemctl daemon-reload
sudo systemctl enable heycode-backend
sudo systemctl restart heycode-backend
echo "    服务已重启"
REMOTE

# ---- 5. 健康检查 ----
echo "==> 健康检查..."
sleep 2
# 从 .env 读 PORT（默认 8787）
PORT=$(grep -E "^PORT=" "$ENV_FILE" | cut -d= -f2 | tr -d '[:space:]')
PORT="${PORT:-8787}"
# 解析 host
HOST="${TARGET#*@}"

if curl -sf "http://$HOST:$PORT/api/health" | grep -q '"ok":true'; then
    echo "    健康检查通过: http://$HOST:$PORT/api/health"
    echo "==> 部署成功"
else
    echo "警告: 健康检查失败，请检查日志："
    echo "    ssh $SSH_OPTS $TARGET 'sudo journalctl -u heycode-backend -n 50 --no-pager'"
    exit 1
fi
