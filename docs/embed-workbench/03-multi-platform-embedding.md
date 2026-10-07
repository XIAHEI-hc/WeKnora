# WeKnora 多平台原生知识工作台嵌入架构

## 1. 定位

WeKnora 的原生项目工作台是一项可被多个业务平台复用的知识服务能力。它提供知识库
文档、目录、解析状态、引用、历史会话和 AI 问答等原生页面；宿主平台继续拥有自己的
登录、用户、项目成员关系和业务权限。

这里的“多平台”表示多个受信任宿主可以分别接入同一套 WeKnora 能力，不表示不同
平台共享同一个默认知识库，也不表示浏览器可以自行选择 tenant、知识库或 Agent。

```text
PixLab 用户 ── PixLab 身份与权限桥 ── PixLab 项目绑定 ── CIS 知识库
                                      │
                                      └─ WeKnora 原生工作台

MemoryLab 用户 ─ MemoryLab 身份与权限桥 ─ MemoryLab 项目绑定 ─ MRA/MemoryLab 知识库
                                           │
                                           └─ 同一套 WeKnora 原生工作台
```

每次请求必须同时落在一个明确的宿主、一个明确的项目和该项目的服务端知识范围内。

## 2. 三层边界

### 2.1 宿主平台层

宿主平台负责：

- 验证本平台登录会话和用户状态；
- 判断用户是否能进入某个业务项目；
- 签发短期、一次性 bootstrap ticket；
- 通过独立 HMAC backchannel 响应 ticket 兑换和持续权限复核；
- 以同源代理承载 iframe 页面和受限工作台 API；
- 在自己的导航中提供直接入口，不让终端用户先选择 WeKnora 项目。

每个宿主必须拥有独立的 public origin、backchannel URL、Key ID、HMAC secret、请求头
前缀、用户 ID 前缀、会话 channel 和内部网络。一个平台的凭据不能验证另一个平台的
principal。

### 2.2 项目绑定层

WeKnora 使用服务端项目绑定把宿主业务项目映射到知识范围：

| 字段 | 含义 | 安全要求 |
|---|---|---|
| `project_code` | 宿主和 WeKnora 共同认可的稳定项目代码 | 来自受信任 ticket，不接受浏览器随意切换 |
| `tenant_id` | WeKnora 工作空间 | 必须存在且与绑定一致 |
| `knowledge_base_id` | 当前项目可访问的知识库 | 查询、上传、预览、chunk 和引用都再次校验 |
| `agent_id` | 当前项目使用的问答 Agent | 由服务端注入，浏览器不能覆盖 |
| `status` | 绑定状态 | 非 `active` 不得建立或恢复会话 |
| `revision` | 绑定版本 | 变化后使旧 principal/session 失效 |

当前 wire contract 为一个项目绑定一个主要知识库和一个 Agent。若未来一个项目需要组合
多个知识库，应在服务端绑定模型和所有读写接口中统一扩展知识库集合，并补齐跨库授权
测试；不能只在前端拼接多个知识库 ID。

### 2.3 WeKnora 原生能力层

WeKnora 复用自己的知识库、文档、问答、引用和会话组件。宿主页面只负责 iframe 生命周期
与 bootstrap 消息，不复制聊天 UI、知识库管理 UI 或项目选择 UI。

受限工作台 API 必须执行以下收敛：

- 会话按宿主用户命名空间和 `project_code` 双重过滤；
- 知识库、文档、preview、chunk、引用和上传固定在项目绑定范围内；
- 问答请求忽略客户端提交的 KB/Agent 范围，使用绑定值；
- 写请求要求 CSRF 和 capability；
- 长时间流式回答期间持续复核宿主授权；
- 不通过宿主网关暴露普通 WeKnora `/api/v1/*` 管理接口。

## 3. 建立会话的时序

```text
1. 用户登录宿主平台并点击知识服务入口
2. 宿主加载同源 /weknora-workbench/projects/<project_code>
3. iframe 发送带 nonce 的 ready 消息
4. 宿主后端验证登录、项目权限并签发一次性 ticket
5. 宿主将 ticket、project_code 和 nonce 发送给 iframe
6. WeKnora 按 project_code 选择已注册的宿主适配器
7. WeKnora 通过该适配器的独立 HMAC backchannel 兑换 ticket
8. WeKnora 查询服务端项目绑定，创建受限 HttpOnly/CSRF 会话
9. iframe 只通过受限工作台 API 访问绑定知识库和 Agent
10. 刷新时优先 resume，并重新验证宿主 principal 与绑定 revision
```

ticket 只能使用一次，不能放入 URL；HMAC 请求必须包含 timestamp 和 nonce，并使用 replay
cache；日志必须脱敏 ticket、session token 和 CSRF token。

## 4. 当前宿主适配器

| 属性 | PixLab | MemoryLab |
|---|---|---|
| bridge 开关 | `PIXLAB_BRIDGE_ENABLED` | `MEMORYLAB_BRIDGE_ENABLED` |
| 项目选择 | PixLab 返回的项目目录/默认项目 | 固定 `MEMORYLAB_MRA` |
| HMAC 请求头 | `X-PixLab-*` | `X-MemoryLab-*` |
| 用户命名空间 | `pixlab:` | `memorylab:` |
| 问答/上传 channel | `pixlab-workbench` | `memorylab-workbench` |
| public origin | `PIXLAB_PUBLIC_ORIGIN` | `MEMORYLAB_PUBLIC_ORIGIN` |
| 内部网络 | `pixlab-internal` | `memorylab-internal` |

两者共享原生前端 bundle 和兼容 API 路径，但不共享认证配置、principal、项目绑定或知识库。

## 5. 新平台接入清单

接入第三个平台时至少完成以下工作：

1. 定义稳定的平台 ID、项目代码规则、用户前缀、channel 和请求头前缀。
2. 在宿主实现登录/项目权限校验、一次性 ticket、principal validate 和 nonce 防重放。
3. 在 WeKnora 注册独立宿主配置和 principal client；未知平台或项目必须默认拒绝。
4. 创建独立 HMAC 凭据和内部网络连接，不与已有平台复用 secret。
5. 建立服务端项目绑定，记录 tenant、知识库、Agent、状态和 revision。
6. 在宿主同源代理中只开放 workbench 页面和受限 API 前缀。
7. 复用 WeKnora 原生 UI，不创建第二套聊天页面或面向终端用户的项目选择页。
8. 增加配置、签名、命名空间、项目绑定、跨用户、跨项目和跨知识库自动化测试。
9. 新增该平台自己的部署文档、配置矩阵、回滚步骤和验收证据。

当前实现已经注册 PixLab 和 MemoryLab。新增宿主仍需显式开发、测试和部署适配器；仅增加
一组环境变量不会自动获得可信接入能力。

## 6. 隔离验收

每个平台至少验证以下两组问题：

| 类型 | 操作 | 期望结果 |
|---|---|---|
| 正向 | 询问当前平台知识库中有明确依据的问题 | 正常回答，只引用当前项目绑定知识库 |
| 反向 | 询问另一个平台独有的项目、流程或文档内容 | 明确缺少依据或无法回答，不返回对方引用 |

例如，在 MemoryLab/MRA 工作台询问 CIS 专属流程时找不到资料，是正确的隔离结果；如果
能够引用 CIS 文档，则必须立即检查 `project_code`、项目绑定、session principal 和所有
文档/chunk 查询的知识库约束。

还必须验证：

- 用户 A 看不到同项目用户 B 的私人会话；
- 项目 P 不能通过改 URL 访问项目 Q 的会话、文档、preview 或 chunk；
- 撤销宿主登录、项目成员关系或 capability 后，下一次请求或流式复核失败；
- 修改 binding revision 后旧会话不能继续使用旧知识范围；
- PixLab、MemoryLab 等其他已注册宿主仍能正常工作。

## 7. 兼容命名说明

当前 `/api/v1/pixlab-workbench/`、`pixlab_project_bindings`、工作台 Cookie 名以及部分
`PixLab*` Go/TypeScript 标识符来自第一个 PixLab 实现。它们目前是兼容 wire contract，
不是产品能力边界。

新平台可以复用这条兼容路径，但必须由服务端 `project_code` 选择正确的宿主适配器。若要
改成中性的 `/api/v1/host-workbench/` 和通用类型名，应另行设计版本化迁移、双路径兼容、
Cookie 迁移和回滚方案，不能在新增宿主时直接破坏已有 PixLab/MemoryLab 部署。

## 8. 平台文档

- [PixLab/CIS 部署与验收](./01-pixlab-deployment.md)
- [MemoryLab/MRA 部署与验收](./02-memorylab-deployment.md)
- [原生项目工作台能力与接口矩阵](./02-native-project-capabilities.md)
