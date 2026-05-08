#!/usr/bin/env bash
# bump-version.sh — 一次性把 wails.json + frontend/package.json 的版本号改到目标值。
#
# 用法:
#   scripts/bump-version.sh 1.0.1
#
# 脚本只改文件，不会自动 git commit / tag —— 你确认 README 也写好后再自己 tag。
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "用法: $0 <new_version>"
  echo "例:  $0 1.0.1"
  exit 1
fi

NEW="$1"

# 简单校验 SemVer
if ! [[ "$NEW" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.-]+)?$ ]]; then
  echo "✗ 版本号不符合 SemVer (x.y.z 或 x.y.z-prerelease): $NEW"
  exit 1
fi

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WAILS="$ROOT/wails.json"
PKG="$ROOT/frontend/package.json"

[[ -f "$WAILS" ]] || { echo "✗ 找不到 $WAILS"; exit 1; }
[[ -f "$PKG"   ]] || { echo "✗ 找不到 $PKG"; exit 1; }

OLD_WAILS=$(node -p "JSON.parse(require('fs').readFileSync('$WAILS','utf8')).info.productVersion")
OLD_PKG=$(node -p   "JSON.parse(require('fs').readFileSync('$PKG','utf8')).version")

echo "wails.json:           $OLD_WAILS  →  $NEW"
echo "frontend/package.json: $OLD_PKG  →  $NEW"

# 用 node 改，避免 sed 处理 JSON 出错
node -e "
const fs = require('fs');
const f = '$WAILS';
const j = JSON.parse(fs.readFileSync(f, 'utf8'));
j.info = j.info || {};
j.info.productVersion = '$NEW';
fs.writeFileSync(f, JSON.stringify(j, null, 2) + '\n');
"

node -e "
const fs = require('fs');
const f = '$PKG';
const j = JSON.parse(fs.readFileSync(f, 'utf8'));
j.version = '$NEW';
fs.writeFileSync(f, JSON.stringify(j, null, 2) + '\n');
"

echo "✓ 版本号已统一为 $NEW"
echo ""
echo "下一步:"
echo "  1. 在 README.md 顶部 Version 徽章 改为 v$NEW"
echo "  2. README.md '最近修复' 段落新增 ### v$NEW 小节"
echo "  3. git add -A && git commit -m \"chore: bump version to v$NEW\""
echo "  4. git tag v$NEW && git push --tags"
