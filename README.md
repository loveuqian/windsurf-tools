# Windsurf Tools 🏄‍♂️

[![Version](https://img.shields.io/badge/Version-v1.10.0-success)](https://github.com/seven7763/windsurf-tools/releases)
[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20macOS%20%7C%20Linux-blue)](#运行环境--prerequisites)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Built with Wails](https://img.shields.io/badge/Built%20with-Wails%20v2-red)](https://wails.io/)

> **Windsurf IDE 无限续杯工具 — 多账号自动切换、额度用完自动换号、无感代理、本地 OpenAI 接口**
> Seamless MITM proxy for Windsurf IDE — auto account rotation when quota exhausted, batch import, local OpenAI-compatible relay. One-click setup, cross-platform.

基于 [Wails v2](https://wails.io/) (Go + Vue 3) 的桌面工具，为 Windsurf / Codeium IDE 提供：

- 🔄 **额度用完自动换号** — 检测到当前账号额度耗尽，秒级自动切换到下一个可用账号，Cascade 对话不中断、不报错，真正无限续杯
- 🎯 **多账号号池管理** — Free / Trial / Pro / Max 多套餐统一管理，支持手动锁定、轮换池、会话粘性，想用哪个用哪个
- 📥 **批量导入** — 支持 API Key / JWT / 邮箱密码 / `邮箱----devin-session-token` 等多种格式混合粘贴，智能识别一键导入
- 🕵️ **无感 MITM 代理** — 透明劫持 Windsurf 请求，protobuf 层替换身份和计费字段，IDE 内无需任何操作
- 🌐 **本地 OpenAI Relay** — SSE 流式输出，兼容 `OpenAI SDK` / `LobeChat` / `ChatGPT-Next-Web` / `Cursor`，自带健康检测和故障倒换
- 📊 **实时用量统计** — Token 流水、美金成本聚合、完整请求审计，钱花在哪一目了然
- 🧹 **清道夫** — 一键清理 Cascade 对话残留和渲染缓存，释放硬盘空间
- 🔐 **一键启动** — macOS / Windows / Linux 全平台，CA 安装 + Hosts 配置一次弹窗搞定

---

## 🎨 界面缩略与核心功能 | Features & Previews

#### 1. 代理核心与全局总览 (Dashboard)
直观的全局大盘！一眼确认纯 MITM 代理状态、健康度、号池总量与活跃的无感切割链路，以及中转大盘信息。

| 首页总览面板 |
| :---: |
| ![Dashboard](docs/images/preview-dashboard.png) |

#### 2. 号池统管全景 (Accounts)
动态跟踪 `Free / Trial / Pro / Max` 全序列套餐状况。无需登录浏览器，随时监控最新订阅边界、当前运行时见底（Runtime Exhausted）、历史用量以及池绑定状况。

| 账号与号池管理视图 |
| :---: |
| ![Accounts](docs/images/preview-accounts.png) |

#### 2. 本地 OpenAI API 兼容中转 (OpenAI Relay)
集成 SSE 流式输出能力。您可以将自己购买或获取到的账号无缝接入类似 `ChatGPT-Next-Web`, `LobeChat`, `Cursor`, 甚至 `OpenAI SDK` 客户端。后端自带健康检测与故障倒换，前端全UI掌控模型映射。

| OpenAI Relay 控制台 |
| :---: |
| ![Relay](docs/images/preview-relay.png) |

#### 3. 流量用量统计面板 (Usage & Diagnostics)
全新设计的 **Usage Dashboard** 精确计算并聚合从您机器发往 Windsurf / OpenAI 的全部流通 Token 的数量以及大略转换的美金价值，全方位杜绝隐藏费用，更有完整历史流水审计明细。

| 数据用量与流水洞察 |
| :---: |
| ![Usage](docs/images/preview-usage.png) |

#### 4. 高级抓包与环境调试引擎 (Advanced MITM Config)
强大的 MITM 号池设置机制！从会话固化（Session Binding）、静默截获到高能协议体 Protobuf 的深度解析与截流。更支持直接抓取原始流水（Dump），方便二次排查分析。

| 核心层代理与策略配置 |
| :---: |
| ![Settings](docs/images/preview-settings.png) |

#### 5. 垃圾与残留清道夫 (Clean-Up Optimizer)
不再让海量 Cascade AI 对话数据和渲染缓存吃掉你珍贵的硬盘空间！轻轻一点即可完成各环节的精简化部署清理，重获新生。

| 磁盘瘦身优化 |
| :---: |
| ![Cleanup](docs/images/preview-cleanup.png) |

> ⚠️ *声明：当前仓库内上述预览图均为最新桌面端界面的脱敏展示图。我们永远不会窃取并上传任何账号池数据，全部本地化存储于 `settings.json`与 `accounts.json`。*

---

## 📦 下载发布包 | Download Releases

每次推送 `v*` 标签后，GitHub Actions 会自动构建并发布以下产物到 [Releases](https://github.com/seven7763/windsurf-tools/releases)：

| 文件 | 平台 | 说明 |
|------|------|------|
| `windsurf-tools-wails.exe` | Windows amd64 | 单文件，启动时默认请求管理员权限 |
| `windsurf-tools-wails-windows-amd64.zip` | Windows amd64 | Windows 单文件压缩包 |
| `windsurf-tools-wails-macos-intel-amd64.zip` | macOS Intel | 打包后的 `.app` 压缩包 |
| `windsurf-tools-wails-macos-apple-silicon-arm64.zip` | macOS Apple Silicon | 打包后的 `.app` 压缩包 |
| `windsurf-tools-wails-linux-amd64.zip` | Linux amd64 (24.04+) | 基于 WebKit2GTK 4.1 |
| `windsurf-tools-wails-linux-amd64-legacy.zip` | Linux amd64 (22.04) | 基于 WebKit2GTK 4.0（旧发行版） |
| `SHA256SUMS.txt` | 全平台 | 所有发布文件的 SHA256 校验 |

> 本程序在 Windows 下默认请求管理员运行以实现完整的代理劫持（Hosts、CA安装配置）。请放心授予或采用受控模式运行。macOS 环境需要处理好初次的 Gatekeeper。

---

## 💻 运行环境 + 安装指南 | Prerequisites & Install Guide

### Windows
- Windows 10 / 11 `amd64`
- [Microsoft Edge WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/) 依赖（Win 11 自带；Win 10 可能需安装）

```powershell
# 下载 windsurf-tools-wails.exe
# 双击运行 → 接受 UAC 提权（默认 admin manifest）
# 如果提示「未发布的应用」: 右键 → 属性 → 解除锁定
```

### macOS（Intel / Apple Silicon）

```bash
# 1) 下载对应 zip 解压，把 .app 拖到 /Applications
# 2) 首次启动绕过 Gatekeeper:
xattr -d com.apple.quarantine /Applications/windsurf-tools-wails.app
open /Applications/windsurf-tools-wails.app
# 或: 右键 → 打开（Gatekeeper 会让你再次确认）
```

### Linux

#### 真硬性依赖（缺了 app 起不来）

| 包 | 作用 |
|---|---|
| `libwebkit2gtk-4.1-0` (Debian/Ubuntu) / `webkit2gtk4.1` (Fedora) / `webkit2gtk-4.1` (Arch) | Wails 渲染 UI 用，**必须**装 |

#### 推荐装但都有 fallback（缺了仅影响个别功能）

| 包 | 缺了影响 | App 内 fallback |
|---|---|---|
| `ca-certificates` (提供 `update-ca-certificates`) | MITM 模式 CA 装不上 | OpenAI Relay 模式（推荐）完全不需要 |
| `policykit-1` / `polkit` (提供 `pkexec`) | 提权弹窗换成终端密码 | 自动降级 `sudo` |
| `libnotify-bin` / `libnotify` (提供 `notify-send`) | 系统通知中心不弹 | 自动降级 `gdbus`（glib2 自带）→ app 内 toast 始终 OK |
| `xdg-utils` (提供 `xdg-open`) | 「在文件管理器中显示」失效 | 自动降级 `gio → kde-open5 → exo-open → gnome-open` |

99% 桌面 Linux 上后 4 个早已默认装好。极简服务器 / WSL / Docker 才需手动装。

#### 一键全装命令（懒人推荐）

```bash
# Debian / Ubuntu
sudo apt install libwebkit2gtk-4.1-0 ca-certificates policykit-1 libnotify-bin xdg-utils

# Fedora / RHEL / CentOS
sudo dnf install webkit2gtk4.1 ca-certificates polkit libnotify xdg-utils

# Arch / Manjaro
sudo pacman -S webkit2gtk-4.1 ca-certificates polkit libnotify xdg-utils

# 然后:
unzip windsurf-tools-wails-linux-amd64.zip
chmod +x windsurf-tools-wails
./windsurf-tools-wails
```

> Dashboard 顶部「平台兼容性检查」按钮会准确告诉你**当前**缺哪个依赖、影响哪个功能、修复命令。

如果遇到 `error while loading shared libraries: libwebkit2gtk-4.1.so.0` 类报错，说明缺 WebKit2GTK 4.1。Ubuntu 22.04 用户可下载 `linux-amd64-legacy.zip`（基于 WebKit2GTK 4.0）。

---

## 🌐 平台特性矩阵 | Platform Feature Matrix

| 功能 | macOS | Windows | Linux |
|---|:---:|:---:|:---:|
| MITM 代理 (核心) | ✅ | ✅ | ✅ |
| 号池轮换 / Pin / 池 | ✅ | ✅ | ✅ |
| OpenAI Relay | ✅ | ✅ | ✅ |
| Cascade 破限注入 | ✅ | ✅ | ✅ |
| Clash IP 智能切换 | ✅ | ✅ | ✅ |
| CA 自动安装 | ✅ | ✅ | ✅ (推荐装 `ca-certificates`) |
| Hosts 自动写入 | ✅ | ✅ (UAC) | ✅ (pkexec → sudo 自动 fallback) |
| 桌面通知 | ✅ | ✅ | ✅ (notify-send → gdbus → app toast 三级 fallback) |
| 文件管理器集成 | ✅ Finder | ✅ Explorer | ✅ (xdg-open → gio → kde-open5 等链式 fallback) |
| 系统托盘 | ✅ | ✅ | ⚠️ 依赖桌面环境 (无 tray 时仍可用窗口) |
| 配置导出/导入 | ✅ | ✅ | ✅ |

> Linux 上后 4 项即使缺包也有 fallback；唯一真硬性依赖是 `libwebkit2gtk-4.1-0`（app 启动需要）。
> Dashboard 顶部「平台兼容性检查」按钮可一键诊断当前平台所有依赖是否就绪。

---

## 🧰 从源码构建 | Build from Source

#### 前置条件
- [Go](https://go.dev/dl/) 1.24.x
- [Node.js](https://nodejs.org/) 20+
- [Wails CLI v2](https://wails.io/docs/gettingstarted/installation)

```bash
git clone https://github.com/seven7763/windsurf-tools.git
cd windsurf-tools

# 安装前端依赖
cd frontend
npm install
cd ..

# 编译应用 (默认输出在 build/bin/ 下)
wails build
```

---

## ⚙️ 系统集成：服务化运转模式

支持基于 [kardianos/service](https://github.com/kardianos/service) 的无 UI 后台服务模式（纯 Daemon），使得你的工作环境能持久享受 OpenAI 中继及 MITM 打通福利！

`windsurf-tools-wails.exe install/start/stop`

---

## 📁 隐私与数据目录 | Privacy

应用核心配置目录：
- **Windows**：`%APPDATA%\WindsurfTools\`
- **macOS**：`~/.windsurf-tools/`（含 CA 证书 `ca/ca.pem`）

内部保存 `accounts.json`、`settings.json` 及全套 MITM 证书。**切勿向公共仓库提交这些文件。** 详见 [SECURITY.md](SECURITY.md)。

---

## 🔧 最近修复 | Recent Fixes

### v1.10.0 (2026-05-27)

**上游代理热切换重构 + macOS 托盘 + 全面打磨 | Upstream Proxy Hot-Switch + macOS Tray + Polish**

**代理热切换 | Proxy Hot-Switch (P1、3)**

- **OpenAIRelay 对齐 MITM 闭包策略** — `SetUpstreamProxy` 不再重建 transport，改为改字段 + `CloseIdleConnections`，与 `MitmProxy.SetUpstreamProxy` 行为完全一致。切换瞬间 in-flight 请求不被 abort，无双出口、无 GC 抖动
- **applyUpstreamProxy epoch 串行化** — 用户连点保存设置 / 多入口同时触发时，不再出现多个 goroutine 并发探活后旧 stale 结果覆盖新结果。`atomic.Uint64` epoch + `sync.Mutex` 双重校验
- **clash probe TTL 缓存** — `probeClashProxyEntry` 2s 内存缓存，避免 UI 连点重复跑 3s HTTP 探活；AutoSetupClash 等明确状态变更入口调 `InvalidateClashProbeCache` 绕过
- **readSystemProxy 多域 probe** — 从单一 windsurf 域改成 [windsurf, anthropic, openai] 顺序探，任一命中即返回，应对 NO_PROXY 边角场景
- **clash secret 双发** — 同时设 `Authorization: Bearer` 和 `?secret=` query，兼容 mihomo / clash-meta + 老 clash-premium fork
- **clash 三端口全 0 诊断日志** — 探活通了但 mixed-port/port/socks-port 全为 0 时写 `log.Printf`，不再静默降级

**启动 / 关闭 | Lifecycle (A5、6、7, S3)**

- **`log.Fatalf` → `runtime.Quit`** — 启动失败不再 `os.Exit` 跳过 OnShutdown。交互模式弹原生 MessageDialog，silent 模式写日志，两者都走 `runtime.Quit` 让 hosts / CA / 443 listener / 托盘被清理
- **shutdown 加 `sync.Once`** — wails 在 panic / second-instance / 快速退出路径上重入 OnShutdown 不再走到 hosts / CA 重复处理
- **显式 `mitmProxy.Stop()`** — 与 `openaiRelay.Stop()` 对称。未启动 / 无 hosts / 无 CA 三条都不满足时 listener 不再漏关
- **shutdown cleanup 同步等待到底** — 写 hosts / 卸 CA / macOS sudo 弹窗都走完成不走超时。残留 hosts / CA 会把用户全机流量劈到 127.0.0.1:443，比 wails 多卡几秒严重得多；清理前后加日志告知用户为什么在等
- **第二实例抢焦** — `activateExistingWindow` 加短暂置顶，绕过 Win11 防偷焦机制

**端口 / 默认值 | Ports & Defaults (N2, N3)**

- **OpenAIRelay 端口被占顺序降级** — 从 desiredPort 起最多探 14 个连续端口（8787-8800），实际端口写回 Status API。避免常用端口冲突 Relay 启不来
- **Clash 默认控制器端口 9097 → 9090** — 对齐 mihomo / clash-meta / 原版 Clash / ClashX（Verge 仍 9097，Help 页双值说明）。初装用户照默认填不再探活失败

**UI & macOS 托盘 | UI & Tray (P6, T1)**

- **Dashboard 上游代理状态角标** — 新增 `GetUpstreamProxyStatus()` Wails API，Dashboard 顶部加第 6 张卡片显示当前走 `Clash + 轮换 / Clash / 系统代理 / 直连`，不用翻日志猜为什么还在被风控
- **macOS 启用系统托盘** — `tray.go` build constraint 从 `windows` 改为 `windows || darwin`。Mac 用户现在能用 MinimizeToTray，关窗隐藏到顶栏而非直接退出

**仓库健度 | Repo Health**

- **添加 `.gitattributes` 强制 LF** — 修复 Windows `core.autocrlf=true` 导致的 EOL 翻转问题。之前编辑器回写时把 LF 转 CRLF，git diff 出现大量虚假行变动
- **修 1 个预存在测试 bug** — `TestResolveJailbreakOverrideFilePath_EmptyUsesDefault` 在 Windows 上 fail（期望 `.claude/override.md` 但实际 `.claude\override.md`），改用 `filepath.Join` 构造跨平台后缀

### v1.9.0 (2026-05-14)

**新增 邮箱----Token 导入格式 | Email-Token Import**

- **新导入格式** — 支持 `email----devin-session-token$JWT` 格式批量导入。粘贴即自动识别为「邮箱/Token」类型（粉色标签），直接创建账号并写入号池，无需 GetJWTByAPIKey 验证
- **后端 `ImportByEmailAPIKey`** — 新增导入方法，接收 email + API key 对，直接存入 `WindsurfAPIKey` 字段 + `enrichAccountInfoLite` 获取套餐信息
- **前端自动检测增强** — `importAutoDetect.ts` 新增 `email_apikey` 类型，正则匹配 `email----devin-session-token$...` / `email----sk-ws-...` / `email----eyJ...` 三种变体
- **ImportModal UI** — 新增「邮箱/Token」粉色标签 + placeholder 示例 + 格式提示文案

### v1.8.0 (2026-05-14)

**新增「关于」+「帮助」视图 + 法律免责声明 | About/Help/Legal**

- **新视图 `About.vue`** — 三栏 QR 码区：作者微信 (Seven77078) / 赞赏码 / 微信交流群（3 张图轮播）+ 点击放大 Lightbox + 复制微信号按钮 + 仓库链接。所有图嵌入 vite hash 资源。
- **新视图 `Help.vue` (7 章 FAQ)** — iOS Settings disclosure 风折叠，覆盖：
  - **1. 我该选哪种模式？** — IDE 直接切号 vs MITM 代理 对比 (Switch-Account-Local vs SwitchMitmToAccount)
  - **2. API Key 是什么？怎么获取？** — sk-ws- 前缀 + 来源
  - **3. 怎么导入账号？** — 4 种凭证自动识别 + 去重
  - **4. MITM 代理原理** — CA 证书 + Hosts + protobuf F20/F32 字段替换
  - **5. 号池轮换 + Pin + Pool** — 3 个自动触发点 + 手动锁定 + 轮换池
  - **6. Clash IP 智能启用** — 防限速 + type-aware 过滤
  - **7. Cascade 破限注入** — 原理 + 4 个预设风险对比表
- **法律免责声明组件 `LegalDisclaimer.vue`** — 6 条核心：用途限定 / 自负风险 / 账号合法性 / 本地处理 / 破限风险 / 开源免费
- **Sidebar 底部双按钮入口** — 「帮助」(蓝) + 「关于」(红)，不占主菜单
- **章节搜索** — Help 页顶部搜索框，按关键词过滤章节

### v1.7.0 (2026-05-14)

**完整 Linux/Win 支持 + 用户安装文档**

- **README 三平台完整安装指南** — Debian/Ubuntu + Fedora/RHEL + Arch/Manjaro 的 apt/dnf/pacman 依赖一行命令；Windows UAC + WebView2 提示；macOS Gatekeeper `xattr -d com.apple.quarantine` 绕过。WebKit2GTK 4.0 vs 4.1 区分（Ubuntu 22.04 用 legacy zip）
- **平台特性矩阵** — 11 项功能 × 3 平台 ✅/⚠️ 一目了然
- **Releases 表加 Linux artifacts** — `linux-amd64.zip` (24.04+) + `linux-amd64-legacy.zip` (22.04) 两个版本
- **Linux pkexec → sudo fallback** — `privileged_exec.go` 实际已实现 (`pkexec → sudo → 友好错误`)
- **README Platform badge** — 加 Linux

### v1.6.0 (2026-05-14)

**跨平台兼容性 全面加固 | Cross-platform Audit**

- **平台兼容性诊断 API** — 新增 `RunDiagnostics()` 后端方法，一键检测 7 大类平台依赖：桌面通知 / 文件打开 / 提权 / CA 证书工具 / App 数据目录可写 / Windsurf 安装 / Clash 控制器 / (Windows) WebView2。每项返回 `ok / warn / error / n/a` + 具体修复建议
- **Dashboard「平台兼容性检查」紫色按钮** — 点击弹底部 sheet modal 展示完整诊断报告：每项带 ✓/⚠/✗ 图标 + 详细说明 + 蓝色 💡 修复 hint 命令（如 `sudo apt install libnotify-bin`）
- **Linux 桌面通知 fallback** — `notify-send` 缺时降级 `gdbus call` (GNOME/glib 自带，更可能存在)，都没有时静默 + 日志友好提示安装方法
- **Linux 文件打开器 fallback 链** — `xdg-open → gio open → kde-open5 → exo-open → gnome-open`，按发行版常见优先级挑可用的，全失败时返回友好错误带 `sudo apt install xdg-utils`
- **Windows PowerShell 通知 escape 修复** — 改用单引号 literal 字符串（内部 `'` 双写）+ 按 kind 选 SystemIcons (Information/Warning/Error)；powershell.exe 缺时 fallback pwsh.exe (PowerShell 7+)
- **Linux 提权 pkexec → sudo fallback** — 自动检测命令存在性 + 降级提示
- **README 加 Linux 平台标记**

**新增组件 | New Components**

- `app_diagnose.go`: RunDiagnostics 跨平台诊断 + 9 个 check 实现 + intToStr helper
- `app_diagnose_test.go`: 5 个测试覆盖 report 结构 / ID 唯一性 / status 枚举 / intToStr / windsurf candidates

### v1.5.0 (2026-05-14)

**iOS 化二期 全方位打磨 | iOS-Polish v2**

- **全局滚动条 iOS 化** — Webview 默认 14px 太粗，改为 8px 细半透明 (rgba 18% thumb, hover 32%) + 圆角 6 + content-box clip + corner 透明。Light/Dark 镜像 + Firefox `scrollbar-width: thin` 兼容。**所有滚动区域立即 iOS 化**
- **IToast 重做** — 从底部小条改成**顶部下滑 22px 大圆角 glass blur** 卡片：弹簧 cubic-bezier 入场（`0.34, 1.56, 0.64, 1`）+ 多 toast 堆叠 stagger + hover 显示 ✕ 关闭 + 点 toast 任意位置可关 + 4 种 tone（success/绿、error/红、warning/黄、info/蓝）大色块 icon + `dismissToast(id)` 手动关闭 API
- **IConfirm 改成底部 Action Sheet 风** — iOS HIG 标准：底部弹（移动端）/ 中心弹（桌面 sm+），主按钮上 + 取消下 vertical stack，destructive 时主按钮红 + Trash 图标 + 大 AlertTriangle warning icon。ESC / Enter / 遮罩点击都能关
- **ISettingRow 新组件** — iOS Settings.app 风格行（title + description / slot 控件 / 底部细分隔线 + destructive 模式）。等高居中布局 + 自适应窄屏堆叠 + extra slot 给二级附加内容（折叠/警告条）
- **Cleanup / Usage 空状态升级** — 复用 Accounts 大渐变 icon 模板：Usage 用 BarChart3+Activity 角标区分「无记录 vs 无筛选结果」；Cleanup 加载中改为大型旋转 icon + 友好文案
- **新增 `dismissToast(id)` API** — `utils/toast.ts` 暴露给 IToast 手动关闭使用

### v1.4.0 (2026-05-14)

**iOS 化全方面打磨 | iOS-Polish**

- **INumberStepper 新组件** — `[− 数字 +]` 风格替换所有 number input：长按 500ms 后 80ms 间隔连续 ±，点击数字可键入。Settings 里 8 处替换（quota poll / 间隔 / 并发 / 池间隔 / 池额度刷新 / clash 间隔 / 延迟阈值 等）
- **ISelectSheet 新组件** — iOS 弹底部 sheet 替换 native select：圆角 + backdrop blur + Check 图标标识当前选中 + 拖手柄 + 取消按钮。支持 description 二级说明、ESC / 遮罩点击关闭、Teleport 到 body 不被父 overflow 截断。替换 4 处（Quota Policy / Accounts 排序 / 套餐分组 / 分页大小）
- **IToggle 弹性过渡** — `cubic-bezier(0.34,1.56,0.64,1)` 弹簧曲线 + active:scale-95 + 开启状态 inset 高光，更 iOS 原生触感
- **Settings 左侧 sticky 锚点 nav** — `IntersectionObserver` 跟踪当前 section + 高亮当前位置。5 段 nav（使用模式 / OpenAI Relay / Clash IP 轮换 / 保活·切换·池 / 高级·破限），点 nav 平滑 `scrollIntoView`。大屏 lg 显示左侧 sticky；小屏 lg- 退化为顶部 horizontal pill nav
- **Accounts 空状态升级** — 大型渐变 icon 组合 (32×32 + Plus 角标) + 24px 大字标题 + 3 步引导卡片（① 批量导入 ② 启用 MITM ③ 开始使用）+ 主 CTA 加渐变 + scale 触感

### v1.3.0 (2026-05-14)

**新功能 | Features**

- **手动锁定 (Manual Pin)** — 手动 `SwitchMitmToAccount` 成功后**自动锁定**当前账号，3 个自动切换通道（`onKeyExhausted` 回调 / 热轮询 / 定期同步）全部 skip + 日志。用户 100% 控制激活账号，避免「明明切到 user-A 转眼又被自动换走」。Header / Account 卡片 / Settings 三处都有 🔒 徽章 + 一键解锁按钮。热轮询时 pin 不阻断额度刷新（仍刷数据但不切号），保证 UI 数据不停
- **轮换池 (Rotation Pool)** — 选 N 个账号进池，**定时切 + 额度耗尽双触发**只在池内来回切，池外账号完全不参与自动轮换。池内账号 1 分钟（可调 [1,10]）刷一次额度让 UI 实时显示。Settings 加完整 UI：开关 / 间隔输入 / 多选账号 / 状态面板（成员数 / 切换次数 / 上次切到 / Pin 暂停指示）。`intersectByID` 把 candidates 收窄到池成员
- **桌面通知** — Pin 解除 / 额度耗尽切号失败 / Clash 错误 等关键事件弹系统通知中心，60s 内同类去重。跨平台原生命令：macOS `osascript` / Windows PowerShell BalloonTip / Linux `notify-send`。Settings 里可一键关闭
- **配置导出 / 导入** — 跨设备迁移 / 备份。**自动剥离敏感字段**（Clash secret / Relay secret / Pin / 池成员 / Jailbreak 自定义文本）；导入时也保留当前敏感字段不被覆盖。前端走浏览器原生 download / input[type=file]，比 SaveDialog 更稳
- **Account 卡片人性化** — 4 个新徽章 / 按钮：🔒 已锁定 + 解锁按钮、🔁 池内 + 加入/移出按钮、复制 sk-ws-key 按钮（一键给其它工具用）
- **Header 全局可视性** — 顶部右上角 🔒 锁定徽章 + 解锁按钮（任何 view 都能看到 + 操作）、📋 复制当前活跃 API Key 按钮

**Bug 修复 | Bugfixes**

- `app_quota.go` 热轮询 + 定期同步 + `onKeyExhausted` 三处自动切都加 ManualPin guard，防 pin 被无视
- `rotateMitmToNextAvailable` 在 candidates 阶段和 freshCandidates 阶段都用 `intersectByID(pool)` 收窄，避免预热后池外账号又混进来

**测试 | Tests**

- `app_pin_test.go`: setManualPin 持久化 / idempotent / 覆盖 / 拒空 ID / UnpinManualAccount 幂等 / GetManualPinStatus 富化 email+nickname (8 个测试)
- `app_rotation_pool_test.go`: dedupNonEmpty / stringSliceEqual / rotationPoolMemberUsable / pickNextRotationPoolMember 环绕/skip 不可用/全不可用/池外 currentID 等 / intersectByID (8 个测试)
- 全套 16 + 19 (v1.2) = 35 新测试全过，无回归

### v1.2.0 (2026-05-14)

**新功能 | Features**

- **破限注入 全面增强 v2** — 「设置 → Cascade 破限注入」整组面板重构：
  - **4 个预设模板**（带 risk 徽章 + 简介）：
    - `极简（推荐日常）` — 只压拒绝口径 + 不水化代码，最不容易踩 Anthropic 网关，低风险
    - `软版（脱敏）` — 去掉 cyber 关键词，保留 OVERRIDE 优先级 + 场景包装，中风险
    - `原版（硬版）` — patch-claude-v2.py 原始文本，含 malware/exploit/RAT 完整白名单，⚠️ 必触 Anthropic cyber-policy 拒绝
    - `自定义` — 用 textarea 自己编辑
  - **外部文件源** — 可指定 `~/.claude/override.md` 等路径，与 Claude Code 共享 override 文件。textarea 与文件源 segmented switch 二选一
  - **「打开编辑」/「在文件管理器显示」/「保存当前文本到文件」** — 跨平台 (macOS open / Windows explorer /select / Linux xdg-open)
  - **运行时状态面板** — 显示当前生效文本来源、字符数、今日注入数、累计注入数、上次注入相对时间
  - **cyber 雷词警告** — 启发式检测 override 文本含 malware/exploit/0day/AV-EDR bypass/DNS poisoning 等关键词时显示红框警告，提示必触发 Anthropic 网关拒绝
- **注入计数器** — 后端 atomic + RWMutex 跟踪 total/today/last-inject-at，按本机时区自然日重置，UI 可一键清零

**Bug 修复 | Bugfixes**

- **`isMostlyText` UTF-8 续字节漏算** — 中文/日文/韩文 3 字节字符只 1/3 计入 → 文本被误判为 binary。现把 0x80-0xBF 续字节也计入文本范围

**测试 | Tests**

- `jailbreak_presets_test.go` — 4 个 preset ID 唯一性、文本语义、cyber 雷词检测（19 个子测试）
- `jailbreak_file_test.go` — 路径解析（~展开/绝对路径/默认）、save/load 往返、超大截断、binary 检测
- `jailbreak_stats_test.go` — record/snapshot 并发安全（50 goroutines × 100 次）、跨天自动重置

### v1.1.0 (2026-05-14)

**新功能 | Features**
- **Cascade 破限注入 (Jailbreak Override)** — MITM 在每次 `GetChatMessage` / `GetCompletions` 请求的 F2 system prompt 末尾追加 override 文本，覆盖模型 alignment / 拒绝模板。等效于 Claude Code `--append-system-prompt-file`，但走 MITM 协议层，**不动 IDE 任何文件**。可在「设置 → Cascade 破限注入」里 toggle + 编辑文本，`恢复默认` 按钮拉后端内置文本。⚠️ 默认文本含 cyber 关键词会被 Anthropic 网关拒，自行删减再用
- **Clash IP 轮换 智能启用** — 一键按钮：探活控制器 → 自动挑节点最多的 selector group → 写设置 → 启 rotator → 立即切一次。type-aware 过滤排除子组(selector/fallback/urltest)和伪节点("剩余流量"/"套餐到期"/"防失联" type=vmess 假装真节点)。强制开启「限速自动切」开关，避免用户旧设置覆盖
- **导入未识别提示** — `importAutoDetect` 加 `unknown` 类型，短输入/中文备注/乱码不再被强塞为 refresh_token 提交浪费请求；UI 显示 `X 行未识别` 警示

**Bug 修复 | Bugfixes**
- **Settings.vue `SkeletonBlock` 漏 import** — 11 处 template 使用但没 import，Vue 控制台报 `Failed to resolve component` (critical runtime bug)
- **MitmPanel `v-for :key` 撞车** — pool_status 列表用 `key_short` 作 key，`devin-session-token$<JWT>` 类账号共享 16 字符前缀，Vue 错误复用节点。改用 `key_hash`(sha256 前 12 hex)
- **SessionBindingInfo 跨账号会话误算** — 同上原因，`pool_key_short` 全等过滤会把不同账号但前缀同的会话算到当前 key。后端 SessionBindingInfo 加 `pool_key_hash`，前端 Sidebar 用 hash 精确过滤
- **Firebase 错误中文化** — `ImportByEmailPassword` 失败时把 `INVALID_LOGIN_CREDENTIALS` / `EMAIL_NOT_FOUND` / `TOO_MANY_ATTEMPTS_TRY_LATER` / `USER_DISABLED` 等英文错误映射成中文
- **ImportModal 关闭不清 inputText** — 切到 Accounts 重开导入仍能看到上次残留
- **ImportModal 按钮 disabled 漏检** — 全行未识别 (lineCount=0) 仍可点导致点了无反应
- **Settings 测试连接「版本: unknown」** — 后端不返回 version 字段，改显示 selector 组数
- **Settings 顶层 `fetchClashStatus()` 与 onMounted 竞态** — 移入 onMounted 同其他 fetch 一起

**清理 | Cleanup**
- 删 `ImportModal` 4 个未用 import (`toAPIKeyItems` 等)
- 删 `settingsModel.ts` 17 处多余 `(s as any)` cast — models.Settings 已含所有字段
- 删 `Cleanup.vue` 5 处 `(APIInfo as any)` cast — 方法都已正确暴露
- **Accounts.vue 性能优化** — `quickFilterOptions` 从 O(8N) 降到 O(N)，单次遍历计算 7 个 filter 命中数

### v1.0.0 (2026-05-09)

- **登录路径透传** — 重新设计 MITM 身份注入逻辑：只对承载 `conversation_id` 的路径（`Chat / Cortex / Trajectory`）替换号池身份，其余（`auth_pb` / `seat_management_pb` / `cascade_plugins_pb` / `Ping` / 工作流模板）一律透传 IDE 真实凭据，修复 IDE 报 `failed to validate Devin token: Invalid token` 卡死登录的问题
- **全部账号显示"当前活跃" Bug** — `PoolKeyInfo` 加 `KeyHash`（sha256 前 12 hex）严格匹配 Email/Nickname；之前 `KeyShort` 截 16 字符对 `devin-session-token$<JWT>` 类账号全部撞车导致全 pool 都贴同一个 Email
- **Clash IP 轮换徽章状态同步** — toggle 切换立刻进入"启动中…/停止中…"过渡态，自动保存完成后 `fetchClashStatus` 刷新徽章，杜绝"toggle 已开但徽章一直显示已停止"
- **MITM 前置条件 一键就绪** — `SetupMitmAll` 顺序安装 CA + Hosts，已就绪步骤自动 Skipped 避免重复弹密码框；macOS 弹 Terminal 索取登录密码；失败时返回带平台提示的 hint
- **每张卡片单独卸载 CA / Hosts** — 卡片右上角垃圾桶图标，二次确认后只卸载这一步，不影响另一步

### v0.x

- **F20/F32 计费字段替换** — 修复原先只替换 api_key+JWT 不替换 UserID/TeamID 导致上游 auth 用号池账号但 billing 仍记登录用户的严重 Bug（`proxy_identity.go`）
- **macOS 26+ CA 信任** — 改用 Terminal.app 交互式 sudo 走 `security add-trusted-cert`，解决 osascript 无法完整授权的问题
- **单密码批量特权** — `hosts` / DNS flush / 端口 443 绑定合并进一次弹窗，不再多次输入密码
- **Clash TUN 模式兼容** — 自动维护 `Merge.yaml` hosts + DIRECT 规则，避免 TUN 接管后绕过 `/etc/hosts`
- **会话粘性 pool key** — 同一 Cascade conversation 稳定复用同一 pool key，避免 `Invalid Cascade session` 错误

---

## 🔢 版本管理 | Versioning

本仓库遵循 [SemVer](https://semver.org/lang/zh-CN/)：`MAJOR.MINOR.PATCH`。

### 单一事实来源（Single Source of Truth）

版本号在两个地方必须**严格一致**，发版前请同时更新：

| 文件 | 字段 | 用途 |
|------|------|------|
| `wails.json` | `info.productVersion` | Wails 打包元数据、macOS `.app` 的 `CFBundleShortVersionString`、Windows 安装包文件版本 |
| `frontend/package.json` | `version` | Vite 注入 `VITE_APP_VERSION`，前端页脚 / Header 显示 `v<x.y.z>` |
| `README.md` | 顶部 Version 徽章 | 显示用 |

### 一键校验

任何时候执行：
```bash
node -p "JSON.parse(require('fs').readFileSync('wails.json','utf8')).info.productVersion"
node -p "JSON.parse(require('fs').readFileSync('frontend/package.json','utf8')).version"
```
两条输出必须相同；不一致就是 bug。

### 一键升版

仓库自带 `scripts/bump-version.sh`，用 node 严格修改 JSON，避免 sed 误伤：
```bash
scripts/bump-version.sh 1.0.1
```
执行后两处版本号就同步到了 `1.0.1`。

### 发版流程

1. `scripts/bump-version.sh <x.y.z>` —— 同步 `wails.json` + `frontend/package.json`
2. 在 `README.md` 顶部 `Version` 徽章 + "最近修复" 段落新增 `### v<x.y.z>` 小节
3. `git add -A && git commit -m "chore: bump version to v<x.y.z>"`
4. `git tag v<x.y.z> && git push --tags` —— GitHub Actions `release-windows.yml` 自动构建多平台产物上传 [Releases](https://github.com/seven7763/windsurf-tools/releases)
5. macOS DMG 用本地命令构建：`wails build -platform darwin/arm64 -clean`，再用 `create-dmg` 打包

---

## 💬 社区交流 | Community

欢迎加入 **AI 的小圈子** 微信交流群，遇到 Bug 优先在群里反馈，作者会更快响应：

| 主群 (互相学习) | 主群备用 | 3 群 |
| :---: | :---: | :---: |
| <img src="docs/images/wechat-group-2.jpg" width="220" alt="WeChat Group 主群" /> | <img src="docs/images/wechat-group-1.jpg" width="220" alt="WeChat Group 主群备用" /> | <img src="docs/images/wechat-group-3.jpg" width="220" alt="WeChat Group 3 群" /> |

> 📌 微信群二维码 7 天内有效，过期请进 [Releases](https://github.com/seven7763/windsurf-tools/releases) 拉取最新 README 看新二维码。
> 🙋 **二维码失效或群已满进不去？** 直接加作者微信 **`Seven77078`**（备注 *Windsurf Tools*），拉你进群。

---

## ⚠️ 免责声明 | Disclaimer

本项目仅供学习研究 Windsurf / Codeium 协议使用。使用本工具进行商业规避、批量滥用或违反 Windsurf/Codeium 服务条款的行为，相关责任由使用者自负。作者不鼓励、不支持任何违反目标服务 ToS 的用法。

---

## 📄 开源许可 | License
[MIT License](LICENSE)
