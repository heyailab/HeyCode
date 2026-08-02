#!/usr/bin/env bash
# HeyCode 后端一键安装脚本
#
# 用法（服务器上执行）：
#   curl -fsSL https://github.com/heyailab/HeyCode/releases/latest/download/install.sh | sudo bash
#
# 或下载后执行：
#   curl -fsSL -o install.sh https://github.com/heyailab/HeyCode/releases/latest/download/install.sh
#   sudo bash install.sh
#
# 环境变量（可选，覆盖默认值）：
#   HEYCODE_VERSION=backend-v1.0.0   # 指定版本（默认 latest）
#   HEYCODE_PORT=8787                # 监听端口（默认 8787）
#   HEYCODE_MASTER_KEY=...           # 主密钥（不提供则自动生成）
#   HEYCODE_INSTALL_DIR=/opt/heycode # 安装目录（默认 /opt/heycode）
#
# 流程：
#   1. 下载二进制 + systemd unit + .env.example
#   2. 生成 MASTER_KEY（若未提供）
#   3. 创建 heycode 用户 + 设置权限
#   4. 安装 systemd 服务并启动
#   5. 健康检查
#
# 前置：
#   - Linux x86_64
#   - root 或 sudo 权限
#   - curl / wget
#   - systemctl

set -euo pipefail

# ---- 配置 ----
REPO="heyailab/HeyCode"
VERSION="${HEYCODE_VERSION:-latest}"
INSTALL_DIR="${HEYCODE_INSTALL_DIR:-/opt/heycode}"
PORT="${HEYCODE_PORT:-8787}"
SERVICE_NAME="heycode-backend"
APP_USER="heycode"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()  { echo -e "${GREEN}==>${NC} $*"; }
warn()  { echo -e "${YELLOW}警告:${NC} $*"; }
error() { echo -e "${RED}错误:${NC} $*" >&2; }
step()  { echo -e "${BLUE}  →${NC} $*"; }

# ---- 前置检查 ----
if [ "$(id -u)" -ne 0 ]; then
    error "请用 root 或 sudo 执行：sudo bash install.sh"
    exit 1
fi

ARCH=$(uname -m)
if [ "$ARCH" != "x86_64" ]; then
    error "当前架构 $ARCH 不支持，仅支持 x86_64"
    exit 1
fi

if ! command -v curl >/dev/null 2>&1 && ! command -v wget >/dev/null 2>&1; then
    error "需要 curl 或 wget，请先安装：apt install -y curl"
    exit 1
fi

if ! command -v systemctl >/dev/null 2>&1; then
    error "需要 systemd，请使用支持 systemd 的 Linux 发行版（Ubuntu 18+/Debian 10+/CentOS 7+）"
    exit 1
fi

# ---- 下载函数 ----
download() {
    local url="$1"
    local dest="$2"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL -o "$dest" "$url"
    else
        wget -qO "$dest" "$url"
    fi
}

# ---- 1. 确定 GitHub 下载地址 ----
if [ "$VERSION" = "latest" ]; then
    BASE_URL="https://github.com/$REPO/releases/latest/download"
else
    BASE_URL="https://github.com/$REPO/releases/download/$VERSION"
fi

info "HeyCode 后端安装"
echo "  版本: $VERSION"
echo "  架构: linux/amd64"
echo "  目录: $INSTALL_DIR"
echo "  端口: $PORT"
echo ""

# ---- 2. 创建临时目录 ----
TMP_DIR=$(mktemp -d)
trap "rm -rf $TMP_DIR" EXIT
step "临时目录: $TMP_DIR"

# ---- 3. 下载文件 ----
info "下载文件..."
step "打包文件: heycode-backend-linux-amd64.tar.gz"
download "$BASE_URL/heycode-backend-linux-amd64.tar.gz" "$TMP_DIR/heycode-backend-linux-amd64.tar.gz" || {
    error "下载失败，请检查版本 $VERSION 是否存在"
    exit 1
}
# 解包（tar.gz 内含：二进制 + systemd unit + .env.example + install.sh）
tar -xzf "$TMP_DIR/heycode-backend-linux-amd64.tar.gz" -C "$TMP_DIR/"
chmod +x "$TMP_DIR/heycode-backend-linux-amd64"

# 兜底：若 tar 包里缺 .env.example（旧版打包遗漏），从 GitHub 单独下载
# GitHub 会把点开头的 asset 重命名为 default.env.example
if [ ! -f "$TMP_DIR/.env.example" ]; then
    step "fallback 下载 .env.example（GitHub 重命名为 default.env.example）"
    download "$BASE_URL/default.env.example" "$TMP_DIR/.env.example" || {
        warn "无法下载 .env.example，将使用内置最小配置"
        cat > "$TMP_DIR/.env.example" <<EOF
PORT=$PORT
DATABASE_URL=file:$INSTALL_DIR/data.db
MASTER_KEY=replace_me_with_32_bytes_hex_string
JWT_SECRET=change_me_min_8_chars
LOG_LEVEL=info
MOCK_CLI=false
EOF
    }
fi

# ---- 4. 准备 .env ----
info "生成配置..."
if [ -f "$INSTALL_DIR/.env" ]; then
    warn "检测到已有 $INSTALL_DIR/.env，保留现有配置"
    ENV_SOURCE="$INSTALL_DIR/.env"
else
    ENV_SOURCE="$TMP_DIR/.env.example"
    # 替换端口
    sed -i "s/^PORT=.*/PORT=$PORT/" "$ENV_SOURCE"
    # 生成或使用提供的 MASTER_KEY
    if [ -n "${HEYCODE_MASTER_KEY:-}" ]; then
        sed -i "s|^MASTER_KEY=.*|MASTER_KEY=$HEYCODE_MASTER_KEY|" "$ENV_SOURCE"
        step "MASTER_KEY: 使用环境变量提供的值"
    else
        MASTER_KEY=$(openssl rand -hex 32 2>/dev/null || head -c 32 /dev/urandom | xxd -p | tr -d '\n')
        sed -i "s|^MASTER_KEY=.*|MASTER_KEY=$MASTER_KEY|" "$ENV_SOURCE"
        step "MASTER_KEY: 已自动生成"
    fi
fi

# ---- 5. 创建用户 + 目录 ----
info "创建用户和目录..."
if ! id "$APP_USER" &>/dev/null; then
    step "创建用户 $APP_USER"
    useradd -r -s /usr/sbin/nologin -d "$INSTALL_DIR" "$APP_USER"
fi

mkdir -p "$INSTALL_DIR"
chown -R "$APP_USER:$APP_USER" "$INSTALL_DIR"

# ---- 6. 安装文件 ----
info "安装文件..."
step "二进制 → $INSTALL_DIR/heycode-backend"
cp "$TMP_DIR/heycode-backend-linux-amd64" "$INSTALL_DIR/heycode-backend"
chmod 755 "$INSTALL_DIR/heycode-backend"

step "配置 → $INSTALL_DIR/.env"
cp "$ENV_SOURCE" "$INSTALL_DIR/.env"
chown "$APP_USER:$APP_USER" "$INSTALL_DIR/.env"
chmod 600 "$INSTALL_DIR/.env"

step "systemd unit → /etc/systemd/system/heycode-backend.service"
# 调整 unit 中的路径（若 INSTALL_DIR 非默认）
sed "s|/opt/heycode|$INSTALL_DIR|g" "$TMP_DIR/heycode-backend.service" \
    > /etc/systemd/system/heycode-backend.service
chmod 644 /etc/systemd/system/heycode-backend.service

# ---- 7. 启动服务 ----
info "启动服务..."
systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl restart "$SERVICE_NAME"
step "服务已启动"

# ---- 8. 健康检查 ----
info "健康检查..."
sleep 2

# 从 .env 读 PORT
CHECK_PORT=$(grep -E "^PORT=" "$INSTALL_DIR/.env" | cut -d= -f2 | tr -d '[:space:]')
CHECK_PORT="${CHECK_PORT:-8787}"

# 本机检查
for i in 1 2 3 4 5; do
    if curl -sf "http://127.0.0.1:$CHECK_PORT/api/health" | grep -q '"ok"'; then
        echo ""
        info "${GREEN}安装成功！${NC}"
        echo ""
        echo "服务状态:    systemctl status heycode-backend"
        echo "实时日志:    journalctl -u heycode-backend -f"
        echo "配置文件:    $INSTALL_DIR/.env"
        echo "健康检查:    http://<服务器IP>:$CHECK_PORT/api/health"
        echo ""
        echo "App 连接:    在 HeyCode App 设置页填入 http://<服务器IP>:$CHECK_PORT"
        echo ""
        exit 0
    fi
    step "等待服务就绪... ($i/5)"
    sleep 2
done

# 检查失败
error "健康检查失败，请查看日志："
echo "  sudo journalctl -u heycode-backend -n 50 --no-pager"
echo ""
echo "常见问题："
echo "  - 端口被占用: sudo lsof -i:$CHECK_PORT"
echo "  - 配置错误:   sudo cat $INSTALL_DIR/.env"
echo "  - 防火墙:     sudo ufw allow $CHECK_PORT/tcp"
exit 1
