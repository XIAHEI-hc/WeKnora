# Phase 0：Embed Workbench 接入点侦察（fork 本地文档）

> 本文件属于 **fork 本地开发文档**，不一定适合回馈上游。
> 目标：把《WeKnora Embed Workbench 增量改造开发包》里的"建议路径"换成**这个仓库的真实路径**，并锁定基线。
> 分支：`feature/embedding`；写这份文档时工作区处于基线状态、**未写任何业务代码**。

---

## 0. 基线指纹（不许忘的数字）

| 项 | 值 |
|---|---|
| 分支 | `feature/embedding` |
| Commit | `b71b31c897efae7b6b8320ab0e5bdf796eb1e006` |
| git describe | `v0.8.2-99-gb71b31c89` |
| 提交时间 | 2026-09-30 13:43:55 +0800 |
| CHANGELOG 最新版本 | `[0.8.2] - 2026-09-24` |
| 与 `upstream/main` 关系 | 完全一致（0 / 0） |
| 与 `origin/main` 关系 | 领先 111（fork 未推送，等 GitHub 登录） |

**开工前请重新确认 commit 没有被 rebase 掉**：`git -c safe.directory=D:/Vibe_coding/WeKnora rev-parse HEAD`

---

## 1. 本机环境事实（影响"怎么验证"）

| 能力 | 状态 | 说明 |
|---|---|---|
| Go 工具链 | ❌ **未安装** | `go version` 找不到 → **Go 单测在本机跑不了**，只能用容器或先装 Go |
| Node | ✅ v24.21.0 | |
| pnpm | ✅ 12.4.2 | 前端测试/构建可用 |
| Python | ✅ 3.14.7 | `scripts/test_embed_nginx.py` 可用 |
| Docker（Windows 侧） | ❌ 不在 PATH | |
| Docker（WSL Ubuntu） | ✅ 可用 | `wsl -e sh -lc "docker ps"` 能跑 |
| 当前运行的 WeKnora | ⚠️ **不是本工作区构建的** | 容器 `WeKnora-app` / `WeKnora-docreader` 跑的是 `wechatopenai/weknora-app:latest` 官方镜像（已运行 46 小时），不是当前代码 |

→ 结论：**改了前端可以直接 `pnpm build` 验证；改了 Go 必须解决工具链（装 Go，或在 WSL 里用容器跑 `go test`）**。

---

## 2. 真实路径对照表（开发包 → 本仓库）

| 开发包的说法 | 真实情况 |
|---|---|
| `docs/embed-subdomain.md` | ❌ 不存在（已迁移） |
| `docs/embed-secure-mode.md` | ❌ 不存在（已迁移） |
| `docs/api/organization.md`、`docs/api/knowledge-base.md` | ❌ 不存在（已迁移到 `website-docs/04-api/`） |
| 权威 Embed 文档 | ✅ **`website-docs/03-features/13-embed-channel.md`（327 行，必读）** |
| `frontend/public/weknora-widget.js` | ✅ 存在（宿主加载器 SDK） |
| `frontend/src/api/embed/index.ts` | ✅ 存在（771 行） |
| `internal/middleware/embed_auth.go` | ✅ 存在（247 行） |
| `frontend/src/views/knowledge/wiki/WikiBrowser.vue` | ✅ 存在 |
| `internal/router/routes_knowledge.go` | ✅ 存在 |
| "新增 embed entry + embed.html" | ✅ 方向正确，现状见 §3 |
| 公开 API 参考 | `website-docs/04-api/02-api-channels.md` |

---

## 3. 现有 Embed 前端接线（已逐行核实）

- 独立入口：`frontend/embed.html` → `frontend/src/embed-main.ts`
  - 只有**一个路由**：`/embed/:channelId` → `frontend/src/views/embed/EmbedPage.vue`（`embed-main.ts:14-23`）
  - 额外全局挂载了 `ProtectedResourcePreview`（`embed-main.ts:26`）
- Vite 多入口：`frontend/vite.config.ts:98-101` 的 `rollupOptions.input`（含 `index` 与 `embed` 两个 html）
  - dev 环境用插件把 `/embed/:channelId` 落到 `/embed.html`（`vite.config.ts:32-43`）
  - 有一段按 `hostId?.includes('embed')` 判断的**分包逻辑**（`vite.config.ts:78-90`），把 EmbedBotMessage / EmbedUserMessage / EmbedChatCore 等拆到独立 chunk
- nginx（生产）：`frontend/nginx.conf`
  - `location ^~ /embed/` → `auth_request /_embed-frame-policy` + `try_files /embed.html`（`nginx.conf:61-73`）
  - `location = /_embed-frame-policy` → 反向代理到后端 `GET /api/v1/embed-frame-policy`，并带 `X-Embed-Page-URI`（`nginx.conf:76-85`）
  - **故意不继承主站 `X-Frame-Options: SAMEORIGIN`**（`nginx.conf:33` vs `:60-73`）
  - 文件头 `nginx.conf:1-14` 有"独立 embed 子域"的注释样板（只暴露 `/embed/*`、`/weknora-widget.js`、`/assets/*`、`/api/`）

---

## 4. 现有 Embed 鉴权 / 会话模型 —— **本项目最大的约束**

### 4.1 路由

`internal/router/routes_agent.go:220-262` `RegisterEmbedPublicRoutes`：

```go
embed := r.Group("/api/v1/embed/:channel_id", middleware.EmbedAuth(embedService, tenantService, redisClient))
```

公开端点（全部经过 `EmbedAuth`）：`POST /exchange`、`GET /config`、`GET /suggested-questions`、
`GET /chunks/:chunk_id`、`POST /sessions`、`POST /knowledge-chat/:session_id`、`POST /agent-chat/:session_id`、
`GET /messages/:session_id/load`、`POST /sessions/:session_id/stop`、消息推荐、MCP OAuth、工具审批、
`POST /sessions/:session_id/events`、`GET /files`。

管理端点：`RegisterEmbedChannelRoutes`（`routes_agent.go:264-284`），Admin/Viewer 分级 + API Key `ManageChannels` 能力。

CSP 策略：`embedFramePolicyHandler`（`routes_agent.go:343-371`）与 `embedFrameAncestorsMiddleware`（`routes_agent.go:373-401`），
`embedChannelIDFromPath`（`:327-341`）对路径做规范化防绕过。

### 4.2 身份注入：**渠道级匿名访客，不是宿主用户**

`internal/middleware/embed_auth.go:156-173`：

```go
user := &types.User{
    ID:       fmt.Sprintf("embed-%s", channelID),
    Username: fmt.Sprintf("embed-%s", channelID),
    Email:    fmt.Sprintf("embed-%s@embed.local", channelID),
    TenantID: ch.TenantID,
    IsActive: true,
}
applyAuthSession(c, authSession{
    Principal: types.Principal{Type: types.PrincipalEmbedChannel, ID: ...},
    Role:      types.TenantRoleViewer,   // ← 固定 Viewer，与宿主角色无关
    ...
})
```

- token **只从 `Authorization: Embed <token>` 头取**（`embed_auth.go:178-187`），不接受 query string；
- Origin 校验：同源放行（`Sec-Fetch-Site: same-origin`）+ 白名单（`embed_auth.go:210-241`）；
- 三层限流（`embed_auth.go:128-147`）。

### 4.3 两种 token

| Token | 前缀 | 位置 | 内容 |
|---|---|---|---|
| Publish | `em_` | `embed_channels.publish_token` | 长效，仅服务端（安全模式） |
| Session | `ems_` | Redis `embed:session:{token}` | **值 = channel_id 字符串，TTL 30 分钟** |

`internal/application/service/embed_session.go:19-53`：

```go
const (
    embedSessionTokenPrefix = "ems_"
    embedSessionRedisPrefix = "embed:session:"
    embedSessionTTL         = 30 * time.Minute
)
// IssueSessionToken: redis.Set(key, channelID, 30min)
```

→ **session token 里没有用户身份、没有 capability、没有项目/组织 scope。** 只有"哪个渠道"。

另外：
- `ExchangeEmbedSession`（`internal/handler/embed_channel.go:292-324`）**只接受 publish token**，拒绝用 session token 续期；
- 会话签名 `X-Embed-Session` = HMAC(系统密钥, `embed-session:v2|publish_token|channel_id|session_id`)，见 `embed_session.go:102-134`；轮换 publish token 即全量吊销；
- 访客维度统计头：`X-Embed-Visitor`。

### 4.4 渠道模型

`internal/types/embed_channel.go:13-37`：`EmbedChannel` 是**"面向外部网站的 agent 聊天面"**（注释原文），
字段包含 `TenantID / AgentID / Enabled / PublishToken / AllowedOrigins / WelcomeMessage / RateLimitPerMinute / RateLimitPerDay / PrimaryColor / PageTitle / HeaderTitleMode / ShowSuggestedQuestions / WidgetPosition / AllowWebSearch / AllowFileUpload / DefaultLocale / WebhookURL / WebhookSecret`。

- **没有 `knowledge_base_ids` 字段**，但 `EmbedChannelPublicConfig` 会下发 `knowledge_base_ids`（`embed_channel.go:109`）——即 KB 范围是通过**绑定的 Agent** 推导的；
- 官方文档明确：访客请求里的知识库/文档/标签/@提及/技能/模型覆盖**会被服务端丢弃**（`website-docs/03-features/13-embed-channel.md:3`）。

→ 想按"宿主项目 = 一组知识库"来收敛范围，**要么复用 Agent 绑定，要么新增渠道级/会话级 KB scope**，这是必须做决策的点。

---

## 5. 现有宿主桥（postMessage）—— **与开发包的命名冲突**

已实现的协议（`frontend/src/api/embed/index.ts:439-771`，文档见 `13-embed-channel.md:84-89`）：

- 双方带 `source` 字段：iframe → 宿主 `weknora-embed`，宿主 → iframe `weknora-host`；
- 宿主 → iframe：`provide_token`、`set_context`、`set_locale`、`open_with_query`；
- iframe → 宿主：`ready`、`bootstrap_request`、`message_sent`、`message_received`；
- origin 策略：**trust-on-first-use 固定父窗口 origin**（`index.ts:445-477`），敏感载荷无已知 origin 时直接丢弃（`:486-495`）；握手类消息才允许退化为 `*`。

开发包提议的 `weknora.host.init` / `weknora.workbench.ready` 这套命名 **与现有约定不一致**。
建议：**沿用 `source` + `type` 的既有约定**扩展（新增 workbench 事件），而不是另起一套 `weknora.*` 命名空间，避免宿主侧要同时支持两套协议。

---

## 6. "宿主后端换 token" 已有官方姿势 = 我们的 BFF 落点

`website-docs/03-features/13-embed-channel.md:35-50`（安全模式）：

> 业务后端实现 endpoint，服务端持有 `em_` token，调用 `POST /api/v1/embed/{channel_id}/exchange`
> 换取 `ems_` 短效 token 并返回 `{ "token": "ems_...", "expiresIn": 1800 }`；exchange 时**必须带上与白名单一致的宿主 Origin**。

代码里还有现成的示例生成器：`frontend/src/api/embed/index.ts:658-712`（Node / Go 两版），
以及 `weknora-widget.js` 的 80% TTL 自动刷新（`scheduleRefresh`）。

→ 这说明 **Workbench 的 BFF 天然长在这个 exchange 之上**：宿主后端（PixLab）校验用户/项目 → 调 exchange → 把 `ems_` 交给 iframe。
**缺口只有一处**：`ems_` 里没有宿主用户与 capability。

---

## 7. 回归基线（"不许破坏官方 Chat Embed"的底线）

### 7.1 后端测试（本机缺 Go，需容器/装 Go）

```
internal/middleware/embed_auth_test.go
internal/handler/embed_allowed_origins_test.go
internal/handler/embed_channel_exchange_test.go
internal/handler/embed_flow_test.go
internal/handler/embed_resource_urls_test.go
internal/handler/embed_session_test.go
internal/application/service/embed_channel_chunk_test.go
internal/application/service/embed_channel_public_config_test.go
internal/application/service/embed_channel_update_test.go
internal/application/service/embed_session_test.go
internal/application/service/embed_webhook_test.go
internal/embedpolicy/origin_test.go
internal/router/embed_frame_policy_test.go
internal/types/embed_channel_test.go
```

建议命令：`go test ./internal/middleware/ ./internal/handler/ ./internal/application/service/ ./internal/embedpolicy/ ./internal/router/ ./internal/types/`

### 7.2 前端测试（本机可跑）

```
frontend/src/utils/embedAllowedOrigins.test.ts
frontend/src/i18n/embedLocale.test.ts
```

命令：`cd frontend && pnpm test`（`package.json` 的 `test` = `tsx --test`）；构建：`pnpm build`。

### 7.3 nginx 嵌入策略测试（本机可跑，需 Python）

`scripts/test_embed_nginx.py`（含 `/embed/` 与 `/_embed-frame-policy` 的失败即拒绝用例）

### 7.4 手工冒烟（跑起来才算）

创建渠道 → 用预览会话打开 `/embed/<channelId>` → 能问答、能出引用、白名单外域名被拒。

---

## 8. 后端侦察结论（已合并；行号来自只读侦察，未运行任何测试）

### 8.1 路由与隔离

- 唯一注册点 `RegisterEmbedPublicRoutes`（`internal/router/routes_agent.go:220-262`），注册在**全局 Auth 之前**
  （`internal/router/router.go:177-187`，而 `middleware.Auth` 到 `:208` 才 `r.Use`）。
- 中间件链：全局（CORS/RequestID/Language/Logger/Recovery/ErrorHandler）→ 可选 `embedFrameAncestorsMiddleware`（`router.go:165-167`，仅 `/embed/` 页面路径）→ `middleware.EmbedAuth`。
- **含义：embed 访客调不到任何 `/api/v1` KB/Wiki/Agent/成员接口** → Workbench 必须有一层持凭据的服务端。

### 8.2 token 细节补充

| 项 | 事实 | 证据 |
|---|---|---|
| `em_` 生成 | 32 字节随机 → base64url | `service/embed_channel.go:49-55` |
| `em_` 存储 | DB 明文列 `varchar(64)`，`json:"-"` | `types/embed_channel.go:19` |
| `em_` 校验 | 常量时间比较 | `service/embed_channel.go:213-234` |
| `em_` 生命周期 | **无 TTL**，只能 rotate | `service/embed_channel.go:195-211` |
| `ems_` 值 | 仅 channelID | `service/embed_session.go:36-53` |
| 单 token 撤销 | ❌ **不存在**（全仓 `embed:session:` 只有 1 处引用）；rotate 只让 `X-Embed-Session` 句柄失效，已签发的 `ems_` 到期前仍可用 | — |
| Redis 不可用 | 503 | `service/embed_session.go:37-39`、`handler/embed_channel.go:308-312` |

### 8.3 权限真相（比预想更微妙）

- `RequireRole`（`middleware/rbac.go:68-112`）读 `Caller.Role`；缺失时 **fail-closed 当 viewer**（`types/context_helpers.go:189-192`）；`EnableRBAC` 默认开（`config/config.go:878-887`）。
- **KB 维度有后门级行为**：`access.ResolveKB`（`application/access/knowledgebase.go:117-118`）对**同租户 KB 直接 grant `OrgRoleAdmin`**，
  与调用者的 tenant role 无关 → embed 访客在 KB 维度其实是 admin。真正的收敛靠
  `patchEmbedChatPayload`（`handler/embed_channel.go:687-742`，`:706-717` 清空 knowledge_ids/tag_ids/mentions/skills）。
- 角色命名：tenant 维度是 `owner/admin/contributor/viewer`（`types/tenant_member.go:18-43`），
  **"editor" 只存在于组织维度**（`types/organization.go:13-18`）。开发包里写的 "editor" 要翻译成 `contributor`。

### 8.4 可复用 API 清单（Workbench 各页面对应）

> 全部在 `/api/v1` 下、**需要凭据**（embed 匿名面读不到）。路径后括注权限。

- **Overview**：`GET /knowledge-bases`(Viewer)、`GET /knowledge/:id/stages|spans`（解析 5 段 trace，`handler/knowledge.go:673`）、`GET /knowledge-bases/:id/activity`(**Admin+**)
- **Knowledge**：`POST /knowledge-bases/:id/knowledge/file|url|manual`(OwnedKBOrAdmin+KBAccessWrite)、`GET …/knowledge`、`GET /knowledge/:id`、`GET /knowledge/:id/preview`、`GET /:id/download`(Contributor)、`DELETE|PUT /knowledge/:id`、`reparse`、文件夹 `GET|PUT /knowledge-bases/:id/knowledge/folders`、批量 `PUT /knowledge/tags`、`POST /knowledge/batch-reparse|batch-delete|folder|move`、标签 `GET /knowledge-bases/:id/tags`
- **chunk 预览**：`POST /chunker/preview`、`GET /chunks/:knowledge_id`、`GET /chunks/by-id/:id`、`revisions`
- **Wiki**（`/knowledgebase/:kb_id/wiki`）：`GET pages|pages/*slug|folders|index`、`GET search`、跨 KB `POST /wiki-search`、`GET revisions/*slug`、`POST revert`、`GET graph|stats|lint|issues`；写 `POST pages` / `PUT|DELETE pages/*slug` / `move-page`(=OwnedWikiKBOrAdmin+KBAccessWrite)
- **Search**：`POST /knowledge-search`、`GET /knowledge/search`、`POST /messages/search`
- **Chat**：`POST|GET /sessions`、`GET|PUT|DELETE /sessions/:id`、`POST /knowledge-chat|agent-chat/:session_id`、`stop`、`continue-stream`、attachments/steer/artifacts
- **Agents**：`GET /agents`、`/agents/:id`、`/agents/:id/suggested-questions`、`GET /shared-agents`、`GET /shared-knowledge-bases`
- **Members**：`GET /organizations/:id/members`(Viewer+)、`PUT|DELETE …/members/:tenant_id`(Admin+)、`GET /tenants/:id/members`(Viewer+manage_members)、`POST|PUT|DELETE /members`(**Owner**)
  → **没有**"按宿主平台用户维度"的成员接口

### 8.5 能力开关

key `integrations.embed`（`handler/deployment_capabilities.go:17,110`），来源只看 handler 是否注入
（`router/deployment_capabilities.go:13-14`），经 `GET /api/v1/system/capabilities` 下发；
前端 `stores/deploymentCapabilities.ts:18-47`（**fail-open**）、`router/index.ts:381-384`、`config/integrations.ts:19-24`。

### 8.6 仍是硬约束的一条

新增 `/api/v1` 路由**必须**用 `g.apiKeyGroup/g.apiKeyRoute` 声明 API-key 策略，
否则 scoped key 静默 403；`assertAPIKeyPoliciesMatchRoutes`（`router/rbac.go:410-435`）会在启动时 panic 校验声明与真实路由是否对得上。

### 8.7 待前端侦察报告补充

- [ ] Wiki 前端组件可复用性（`WikiBrowser.vue` 抠出来要拆哪些依赖）
- [ ] 前端"最小新增文件集"（新入口 vs 复用 embed 入口加子路由）
- [ ] 现有聊天 UI 与主 SPA 聊天页是"共用"还是"复制"

---

## 9. 已锁定的设计结论（等实现阶段验证）

1. **不动 Core**：Workbench 只新增前端 surface + 薄 BFF，复用现有 KB/Wiki/Chat API。
2. **身份必须从服务端来**：宿主后端校验用户/项目 → 决定 capability → 换 token；浏览器不得自报角色。
3. **token 里必须能装下 scope+capability**：现有 `ems_`（Redis 值仅 channel_id）需要扩展或新增一类 token，**不能靠前端约定**。
4. **KB 范围**：优先复用"渠道绑定 Agent"的既有收敛机制；若需"按宿主项目多 KB"，再评估新增渠道级 scope 字段。
5. **宿主桥沿用 `source`+`type` 既有约定**，只新增事件，不另起命名空间。
6. **CSP / Origin / 限流 复用现成机制**，不新造一套。
7. **不破坏官方 Chat Embed**：§7 的测试是可执行底线。

---

## 10. capability 到底在哪一层执行（**决定方案可行性的关键，已亲自逐行核实**）

这一节是本次侦察最重要的产出。**结论：仓库里已经有一套"能用、能收敛、能按路由声明"的授权执行层，而且可以被外部注入。**

### 10.1 执行链

```go
// internal/middleware/api_key_gate.go:111-126
scope, ok := types.TenantAPIKeyScopeFromContext(c.Request.Context())
if !ok { c.Next(); return }            // ← 没有 scope 就放行，交给 JWT 角色守卫
if err := a.authorize(scope, method, c.FullPath()); err != nil { 403 }
```

- `authorize`（`api_key_gate.go:130-153`）= 按 `(method, fullPath)` 查**路由声明表**，逐条比对 capability；**未声明 = default deny**；
- 责任声明在注册处：`g.apiKeyGroup(...)` / `g.apiKeyRoute(...)`（`router/rbac.go:396-404`），启动时 `assertAPIKeyPoliciesMatchRoutes`（`rbac.go:410-435`）panic 校验。

### 10.2 这个 scope 是**普通结构体 + 公开的 context setter**

```go
// internal/types/tenant_api_key.go:268-291
type TenantAPIKeyScope struct {
    KeyID            uint64
    Name             string
    ScopeType        APIKeyScopeType
    FullAccess       bool
    KnowledgeBaseIDs StringArray   // ← KB 白名单
    Capabilities     StringArray   // ← 能力清单
}
func WithTenantAPIKeyScope(ctx context.Context, scope TenantAPIKeyScope) context.Context
```

配合 `HasCapability`（`:309-320`）、`AllowsKnowledgeBase`（`:322-337`）、`IsKnowledgeBaseRestricted`（`:339-341`）。

**含义：任何一层中间件只要 `WithTenantAPIKeyScope(...)` 注入一个合成 scope，就能白嫖整套按路由的 capability + KB 白名单执行，一行执行层代码都不用改。**

### 10.3 已有的"宿主用户身份"机制（不是我们要造的，是现成的）

`resolveAPIPrincipal`（`internal/middleware/auth.go:563-611`）+ `verifyExternalUserJWT`（`:613-659`）：

| 模式 | 头 | 说明 |
|---|---|---|
| `tenant`（默认） | — | 整个 key 一个 principal |
| `direct_header` | `X-External-User-ID` | **低保证**，仅服务端互调 |
| `signed_token` | `X-External-User-Token` | HS256 JWT，**aud 必须 `weknora`**、**必须带 exp**、**寿命 ≤ 24h**、workspace 声明必须匹配、`sub` = 外部用户 id |

产物：`Principal{Type: PrincipalAPIExternalUser, ID: "<tenantID>:<externalUserID>"}`（`types/principal.go:13`），
并且**会话按外部用户隔离**（`types/principal.go:198`：`api_external_user:<tenant>:<uid>`）。

配置入口：`GET|PUT /api/v1/tenants/:id/api-principal-config`（**Owner only**，`router/routes_auth_tenant.go:101-102`），
`HMACSecret` 落库前 AES-GCM 加密（`types/tenant.go:219-229`）。已有测试：`middleware/auth_api_principal_test.go`。

### 10.4 ⚠️ 一个必须点破的安全红线冲突

侦察报告推荐的"方案 A：BFF 持 tenant API key + 签名外部用户 token"**本身没问题**，但要注意：

> **tenant API key 是长期凭据**。如果让 iframe 直接带 `X-API-Key` 调 WeKnora，就等于把长期 key 放进浏览器——
> 这正是开发包明令禁止的（"所有敏感长期凭据留在服务端，浏览器只拿短期 session token"）。

所以"零 Go 改动"只有在 **BFF 做全量代理**（iframe 只跟宿主后端说话）时才成立；
那意味着 BFF 要代理 KB/Wiki/搜索/**SSE 流式聊天**，比"最薄的 BFF"厚不少，多一跳延迟。

---

## 11. 三个方案对比与推荐（Phase 1 待你拍板）

| | A：BFF 全量代理 | **C：Workbench 会话 token（推荐）** | B：改 `EmbedAuth` |
|---|---|---|---|
| 浏览器持什么 | 短期 token（宿主后端代持 API key） | **短期 `ews_` token** | 短期 `ems_` |
| Go 改动 | **零** | 新增 1 个中间件 + 1 个 token 服务 + 1 个 bootstrap 端点 | 改共享中间件 |
| 复用能力执行层 | ✅（key 侧） | ✅ **注入合成 `TenantAPIKeyScope`，执行层零改动** | ✅ |
| 影响官方 Chat Embed | 无（物理隔离） | **无**（新文件，不碰 `EmbedAuth`） | ⚠️ **有**，波及全部 embed 回归测试 |
| 流式聊天 | ❌ 需 BFF 代理 SSE | ✅ 直连 | ✅ 直连 |
| 工作量重心 | 在 PixLab（Python）侧 | 在 WeKnora 侧 | 在 WeKnora 共享代码 |
| 主要风险 | BFF 变厚、双跳延迟、SSE 代理易错 | 需在 `middleware.Auth` 里加一条"Workbench token"解析分支（additive，但有共享代码风险） | 破坏官方能力，不可接受 |

**推荐方案 C 的骨架**（全部是新增文件，`EmbedAuth` 一行不改）：

1. `POST /api/v1/embed-workbench/:channel_id/bootstrap`
   入参：宿主后端签名断言（复用 `APIPrincipalConfig.HMACSecret` 或渠道级 secret）+ `host` + `project_id`。
   服务端映射出 `{tenant_id, external_user_id, capabilities[], knowledge_base_ids[], exp≤30min}`，
   写 Redis `workbench:session:<ews_...>`，返回 `ews_...` 与 capabilities/ui 描述。
2. 中间件 `WorkbenchAuth`（新文件）：
   解析 `Authorization: Workbench <ews_...>` → 读 Redis → `applyAuthSession(... PrincipalAPIExternalUser ...)`
   + `types.WithTenantAPIKeyScope(ctx, 合成 scope)`。
3. `middleware.Auth` 增加一条 additive 分支识别该 token 类型（**唯一需要碰的共享文件**，需重点回归）。
4. 前端：新增 Workbench 入口/路由/页面（详见前端侦察结论），宿主桥沿用现有 `source`+`type` 约定扩展。
5. 路由：Workbench 复用现有 `/api/v1/*` 端点 → **必须**为新增路由补 `g.apiKeyRoute` 声明，否则启动 panic。

**待你确认的关键取舍**：
- 是否接受"方案 C 需要碰一次 `middleware.Auth`（加一条分支）"？
- 若你想**完全零 Go 改动**，那就得接受方案 A（BFF 全量代理，含 SSE），我会把 BFF 接口清单写成独立文档。
- 宿主角色 → capability 的映射表（开发包已给建议：OWNER/MAINTAINER/DEVELOPER/VIEWER），要不要按它的默认值定？
