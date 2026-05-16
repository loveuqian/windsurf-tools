# F7 / SmartFriend 模式 · 发布前删除清单

> **F7（SmartFriend 模式）是作者自用工具开关**，把 `GetChatMessage` 请求的
> 顶层 F7 varint 从 `CASCADE(5)` 改成 `SMART_FRIEND(13)` 来绕过日/周限额。
> 发布前必须从代码库里彻底拆除，本文档是步骤化操作手册。

## 前置：grep 一行列出所有需删除点

```bash
grep -rn "F7-REMOVAL" --include='*.go' --include='*.vue' --include='*.ts' --include='*.md' .
```

每条结果都已用注释明确标注「整文件 / 整段 / 整行 / 此分支 / 此参数」要做什么。
按下面顺序执行能保证每步都 build 通过。

## 第 1 步：前端

### 1.1 删除文件

```bash
rm frontend/src/composables/useSmartFriend.ts
rm frontend/src/components/F7Banner.vue
```

### 1.2 删除引用

按 `F7-REMOVAL` 注释逐处删掉：

- **`frontend/src/views/Dashboard.vue`**
  - 删 `import F7Banner` / `import { useSmartFriend }` 两行
  - 删 `const sf = useSmartFriend()`
  - `criticalAccounts` / `blockedAccounts` / `healthyAccounts` 中 `if (sf.active.value) ...` 分支删除
  - 删 `<F7Banner variant="full" />` 标签
- **`frontend/src/views/Accounts.vue`**
  - 删 `import F7Banner` / `import { useSmartFriend }`
  - 删 `const sfState = useSmartFriend()`
  - `getQuotaColor` / `isWeeklyBlockedDisplay` / `getCardStateMeta` / `matchesQuickFilter` /
    `quickFilterOptions` 中所有 `sfState.active.value` 分支删除
  - 删 `<F7Banner variant="compact" />` 标签
- **`frontend/src/views/Settings.vue`**
  - 删 `<!-- F7-REMOVAL-BEGIN -->` ↔ `<!-- F7-REMOVAL-END -->` 之间整段卡片
- **`frontend/src/utils/settingsModel.ts`**
  - 删 `smart_friend_enabled` 字段定义、4 处赋值（`createDefaultSettings` / `normalizeSettings` /
    `settingsToForm` / `formToSettings`）

## 第 2 步：后端 settings 字段

- **`backend/models/settings.go`**
  - 删 `SmartFriendEnabled bool` 字段（含 5 行注释）

## 第 3 步：后端 App 层调用面

- **`app.go`** — 删 `a.syncSmartFriendConfig()`
- **`app_settings.go`** — 删 `a.syncSmartFriendConfig()`
- **`app_mitm.go`** — 删 `func (a *App) syncSmartFriendConfig()` 整函数；
  `syncMitmPoolKeys` 注释里 SmartFriend 三行说明 + `collectEligibleMitmPoolKeyInfos`
  传参从 `settings.SmartFriendEnabled` 改为字面量 `false`
- **`app_switch.go`** — 删 `func (a *App) shouldBypassQuotaCheck()` 整函数；
  调用点全部改为传字面量 `false`（`prepareAccountForUsage` / `rotateMitmToNextAvailable`）
- **`app_quota.go`** — 删 `restartQuotaHotPollIfNeeded` / `pollCurrentSessionQuotaAndMaybeSwitch` /
  `refreshDueQuotas` 中三处 `if settings.SmartFriendEnabled` 早返回分支
- **`app_wiring.go`** — 删 `onMitmKeyExhausted` 中 `if s.SmartFriendEnabled` 早返回分支
- **`app_rotation_pool.go` / `app_mitm.go` / `app_switch.go`** — 把 `bypassQuota bool` 参数从
  `accountEligibleForUsage` / `orderedSwitchCandidates` / `orderedMitmCandidates` /
  `pickNextSwitchableAccount` / `pickNextMitmSwitchableAccount` / `prepareAccount` /
  `collectEligibleMitmPoolKeyInfos` / `collectEligibleMitmAPIKeys` /
  `pickNextRotationPoolMember` / `rotationPoolMemberUsable` 中全部去掉，函数体内
  `if !bypassQuota && ...` / `if bypassQuota { return true }` 等分支恢复成纯额度判定
- **测试同步**：`app_switch_test.go` / `app_rotation_pool_test.go` 删除带 `bypassQuota:true` /
  `BypassQuotaAllowsExhausted` 的用例（grep `bypassQuota`）

## 第 4 步：后端 MITM 代理 + Relay

- **`backend/services/proxy_smartfriend.go`** — 整文件 `rm`
- **`backend/services/proxy.go`**
  - 删 `MitmProxy.smartFriendEnabled` 字段
  - 删 `markRuntimeExhaustedAndRotate` 中 `if p.smartFriendEnabled` 分支
  - 删 `handleRequest` 中 SmartFriend F7 patch 块（`if sfEnabled && isChatPath(path)`）
- **`backend/services/proxy_test.go`** — 删 `TestHandleResponseStreamQuotaExhaustedSmartFriendBypass`
- **`backend/services/chat_proto.go`**
  - 把 `buildChatRequestWithModelMode` 改名回 `buildChatRequestWithModel`、去 `smartFriend bool` 参数
  - F7 赋值块改回 `f7 := uint64(5)` 然后 `encodeVarintField(7, 5)`
  - `BuildChatRequestWithModel` 中调用改回新签名
- **`backend/services/chat_proto_test.go`** — 删 `TestBuildChatRequestSmartFriendUsesF7Mode13` /
  `firstTopLevelVarint` helper（如果没有别处用到）
- **`backend/services/openai_relay.go`** — 删 `smartFriend := r.proxy.smartFriendEnabled` 行；
  调用改回 `BuildChatRequestWithModel(...)`
- **`backend/services/relay_anthropic.go`** — 同上

## 第 5 步：移除本文档与残留 marker

```bash
rm docs/F7-REMOVAL.md
# 应该已经没有任何 F7-REMOVAL marker 留下：
grep -rn "F7-REMOVAL" --include='*.go' --include='*.vue' --include='*.ts' --include='*.md' .
# ↑ 期望 0 命中
grep -rn "SmartFriend\|smart_friend\|smartFriend" \
  --include='*.go' --include='*.vue' --include='*.ts' --include='*.md' .
# ↑ 期望 0 命中（除了本次删除留下的 commit log）
```

## 第 6 步：验证

```bash
# 后端
go vet ./... && go build ./... && go test -count=1 -timeout=120s ./...
go test -race -count=1 -timeout=180s ./...
GOOS=windows go build ./...

# 前端
cd frontend && npm run build && cd ..

# Wails binding 应保持一致（应该没改 binding 面，diff 为空）
wails generate module
git diff frontend/wailsjs/go/ -- ':!frontend/wailsjs/go/models.ts'
# models.ts 会因 SmartFriendEnabled 字段消失而有 diff，这是预期。
git checkout -- frontend/wailsjs/runtime/runtime.{d.ts,js}  # CLI 副作用
```

## 完成判据

- `grep -rn "F7-REMOVAL"` 0 命中
- `grep -rn "SmartFriend"` 0 命中（不含 git 历史）
- 所有测试绿（含 race）
- 前端 build 绿
- 桌面 app 启动 → Settings 页不再有 SmartFriend 卡片 / Dashboard 与 Accounts 不再有 F7 banner
