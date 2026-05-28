#!/bin/bash
# Linux 版本远程打包（SSH 到 Kali，比 Docker 快 10×）
# 用法：./build/linux/build-ssh.sh
#
# 前置：
#   - SSH alias `kali` 已配（key-auth）
#   - Kali 已装：go ≥1.22、node、npm、libgtk-3-dev、libwebkit2gtk-4.1-dev、wails CLI
#
# 产物：build/bin/windsurf-tools-wails-linux-amd64 (~17MB ELF)

set -e
cd "$(dirname "$0")/../.."

REMOTE_DIR="/root/build/windsurf-tools-wails"
BIN_NAME="windsurf-tools-wails-linux-amd64"

echo "=== Linux amd64 远程打包（Kali via SSH） ==="

# SSH 连通性
if ! ssh -o ConnectTimeout=5 kali "true" < /dev/null 2>/dev/null; then
    echo "❌ ssh kali 不通。检查 ~/.ssh/config 或网络。"
    exit 1
fi

# 创建远程目录
ssh kali "mkdir -p $REMOTE_DIR" < /dev/null

# 同步项目（排除大目录）
echo "📤 同步源码到 Kali..."
rsync -az --delete \
    --exclude 'frontend/node_modules' \
    --exclude 'frontend/dist' \
    --exclude 'build/bin' \
    --exclude '.git' \
    --exclude '*.log' \
    --exclude 'capture' \
    --exclude 'proto_dumps' \
    --exclude 'traffic_dumps' \
    -e ssh \
    ./ kali:$REMOTE_DIR/

# 远程构建（用 webkit2_41 build tag 适配 Kali 默认的 libwebkit2gtk-4.1）
echo "🔨 远程构建（约 1 分钟）..."
ssh kali "cd $REMOTE_DIR && /root/go/bin/wails build -platform linux/amd64 -tags webkit2_41 -o $BIN_NAME" < /dev/null

# 拉回产物
mkdir -p build/bin
echo "📥 拉回产物..."
rsync -az -e ssh kali:$REMOTE_DIR/build/bin/$BIN_NAME build/bin/

# 验证
if [ -f "build/bin/$BIN_NAME" ]; then
    SIZE=$(ls -lh "build/bin/$BIN_NAME" | awk '{print $5}')
    echo ""
    echo "✅ Linux 版本构建成功"
    echo "   路径: build/bin/$BIN_NAME"
    echo "   大小: $SIZE"
    echo ""
    echo "📋 用户在 Linux 运行前需安装："
    echo "   # Debian/Ubuntu/Kali"
    echo "   sudo apt install libgtk-3-0 libwebkit2gtk-4.1-0"
    echo "   # 或老 Ubuntu (20.04/22.04 默认是 4.0)"
    echo "   sudo apt install libgtk-3-0 libwebkit2gtk-4.0-37"
else
    echo "❌ 构建失败 — 未找到产物"
    exit 1
fi
