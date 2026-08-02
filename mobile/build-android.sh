#!/usr/bin/env bash
# HeyCode Android 本地构建辅助脚本。
#
# 用法：
#   ./build-android.sh debug    # 构建调试 APK
#   ./build-android.sh apk      # 构建 release APK（需 key.properties，默认）
#   ./build-android.sh split    # 按 ABI 拆分构建 release APK
#
# 详见 SPEC-FLUTTER-APP.md §18.4。

set -euo pipefail

MODE="${1:-apk}"
case "$MODE" in
  debug|apk|split) ;;
  *)
    echo "用法: ./build-android.sh [debug|apk|split]"
    exit 1
    ;;
esac

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# ---- 1. 环境检查 ----
echo "==> 环境检查"
command -v flutter >/dev/null 2>&1 || { echo "错误：未找到 flutter，请先安装 Flutter SDK"; exit 1; }
command -v java >/dev/null 2>&1 || { echo "错误：未找到 java，请先安装 JDK 17"; exit 1; }
echo "flutter: $(flutter --version --machine 2>/dev/null | head -1 || flutter --version | head -1)"
echo "java:    $(java -version 2>&1 | head -1)"

# ---- 2. Android 脚手架检查 ----
echo "==> Android 脚手架检查"
if [ ! -d "android/app/src/main/kotlin" ]; then
  echo "未检测到 Android 脚手架，执行 flutter create 补齐…"
  flutter create . --platforms=android --project-name heycode_app
else
  echo "Android 脚手架已存在。"
fi

# ---- 3. 依赖 ----
echo "==> flutter pub get"
flutter pub get

# ---- 4. 签名配置（仅非 debug）----
if [ "$MODE" != "debug" ]; then
  echo "==> 签名配置检查"
  if [ ! -f "android/key.properties" ]; then
    echo "警告：未找到 android/key.properties"
    echo "  - 复制 android/key.properties.example 为 android/key.properties 并填入签名信息"
    echo "  - 或继续构建未签名 APK（CI 场景）"
  else
    echo "已加载 android/key.properties"
  fi
fi

# ---- 5. 打包 ----
echo "==> 构建（mode=$MODE）"
case "$MODE" in
  debug)
    flutter build apk --debug
    OUT="build/app/outputs/flutter-apk/app-debug.apk"
    ;;
  apk)
    flutter build apk --release
    OUT="build/app/outputs/flutter-apk/app-release.apk"
    ;;
  split)
    flutter build apk --release --split-per-abi
    OUT="build/app/outputs/flutter-apk/app-arm64-v8a-release.apk"
    ;;
esac

# ---- 6. 结果 ----
echo "==> 构建完成"
if [ -f "$OUT" ]; then
  echo "产物：$OUT"
  ls -lh build/app/outputs/flutter-apk/*.apk 2>/dev/null || true
else
  # 备用路径
  ALT="android/app/build/outputs/flutter-apk/"
  echo "主路径未找到 APK，检查备用路径 $ALT"
  ls -lh "$ALT"*.apk 2>/dev/null || { echo "错误：未找到 APK 产物"; exit 1; }
fi
