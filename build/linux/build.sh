#!/bin/bash
# Linux 版本打包脚本（需要 Docker Desktop）
# 用法：./build/linux/build.sh
#
# 产物：build/bin/windsurf-tools-wails (Linux ELF amd64, ~25-35MB)
# 用户在 Linux 上运行前需安装运行时依赖：
#   sudo apt install libgtk-3-0 libwebkit2gtk-4.0-37

set -e
cd "$(dirname "$0")/../.."

echo "=== 构建 Linux amd64 版本（Docker） ==="
echo "项目根目录: $(pwd)"

# 检查 Docker
if ! command -v docker &>/dev/null; then
    echo "❌ 未检测到 Docker。请安装 Docker Desktop："
    echo "   brew install --cask docker"
    echo "   # 然后打开 Docker.app 并等待启动完成"
    exit 1
fi

# 构建镜像
echo "📦 构建 Docker 镜像（首次约 3-5 分钟，后续利用缓存）..."
docker build -t wt-linux-builder -f build/linux/Dockerfile .

# 提取产物
mkdir -p build/bin
echo "📤 提取 Linux 二进制..."
docker run --rm -v "$(pwd)/build/bin:/out" wt-linux-builder

# 验证
if [ -f build/bin/windsurf-tools-wails ]; then
    SIZE=$(ls -lh build/bin/windsurf-tools-wails | awk '{print $5}')
    FILE_TYPE=$(file build/bin/windsurf-tools-wails | grep -o 'ELF.*' | head -1)
    echo ""
    echo "✅ Linux 版本构建成功！"
    echo "   路径: build/bin/windsurf-tools-wails"
    echo "   大小: $SIZE"
    echo "   类型: $FILE_TYPE"
    echo ""
    echo "📋 用户在 Ubuntu/Debian 上运行前需安装："
    echo "   sudo apt install libgtk-3-0 libwebkit2gtk-4.0-37"
else
    echo "❌ 构建失败 — 未找到产物"
    exit 1
fi
