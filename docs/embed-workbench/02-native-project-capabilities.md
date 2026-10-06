# PixLab 原生项目工作台能力与接口矩阵

本文是 `PixLab_WeKnora_Native_Embed_DevPack_20261006` 的最终实现对照。项目模式不是一套新的 PixLab 知识库或聊天前端：PixLab 只负责登录、项目选择、授权、一次性 ticket、iframe 宿主和同源代理；iframe 内继续运行 WeKnora 原生组件。

## 原生组件复用清单

| 能力 | 复用的 WeKnora 实现 | 项目模式适配 |
|---|---|---|
| 平台框架 | `views/platform/index.vue` | 关闭设置、命令面板、邀请和新手引导；保留上传队列 |
| 左侧菜单 | `components/menu.vue` | 仅显示当前项目 KB、新对话和当前用户的项目会话 |
| 知识库详情 | `views/knowledge/KnowledgeBase.vue` | KB ID 来自只读项目上下文；数据走受限 transport |
| 目录与上传 | `KbFolderTree.vue`、`KbUploadSourceDropdown.vue`、`UploadTasksPanel.vue` | 保留目录相对路径；项目成员按 `upload` capability 上传 |
| 文档列表 | `DocumentCardView.vue`、`DocumentListView.vue`、`DocumentActionMenu.vue` | 只显示受限 API 真正支持的处理轨迹和条件重试 |
| 文档详情 | `doc-content.vue`、`knowledge-processing-timeline.vue` | 详情、chunk、轨迹和原文均使用项目 API |
| 新对话 | `views/creatChat/creatChat.vue` | 服务端固定绑定 Agent 和 KB |
| 对话页面 | `views/chat/index.vue`、`Input-field.vue`、`botmsg.vue` | Cookie/CSRF SSE transport；上滚分页恢复完整历史 |
| 引用与原文 | `ChatReferencesDrawer.vue`、`ChatReferenceSourceView.vue` | 文档和 chunk 再按绑定 KB 校验 |

`frontend/src/pixlab-workbench/` 只包含启动、路由、上下文、bridge、受限 API transport 和 DTO 适配，不复制上述业务页面。

## 21 项能力矩阵

| # | 原生界面操作 | 状态 | 受限接口或部署路径 | 约束与证据 |
|---:|---|---|---|---|
| 1 | 项目身份与绑定 | 已实现 | `POST /session`、`POST /session/resume`、`GET /projects/:p/context` | ticket 一次性兑换；HttpOnly Cookie；恢复时重新校验成员/绑定并旋转 CSRF，不延长 Redis TTL |
| 2 | 原生菜单/知识库入口 | 已实现 | `GET /projects/:p/context` | `menu.vue` 只导航绑定 KB 与项目会话；项目 router 使用 allowlist |
| 3 | 原生会话列表 | 已实现 | `GET /projects/:p/sessions?page=&size=` | 当前 PixLab user ID 与 project code 双重过滤；菜单分页加载，不固定首 100 条 |
| 4 | 创建会话 | 已实现 | `POST /projects/:p/sessions` | 服务端写入 `pixlab:<user UUID>`、project code；客户端不能覆盖 KB/Agent |
| 5 | 会话详情和消息 | 已实现 | `GET /sessions/:id`、`GET /sessions/:id/messages?limit=&before_time=` | 深链接先执行 owned-session 校验；消息上滚游标分页、去重并保持滚动位置 |
| 6 | 删除/停止会话 | 已实现 | `DELETE /sessions/:id`、`POST /sessions/:id/stop` | 当前用户 + 当前项目；写请求要求 CSRF；跨用户/项目返回 404 |
| 7 | 流式问答/续流 | 已实现 | `POST /sessions/:id/answers`、`GET /sessions/:id/continue-stream` | Agent/KB 由绑定注入；SSE 使用 Cookie/CSRF，不产生普通 WeKnora JWT；流期间每 2 秒复核 PixLab 授权，撤权时同时关闭连接与后台生成 |
| 8 | 目录树 | 已实现 | `GET /projects/:p/folders` | 只读取绑定 KB；原生 `KbFolderTree` 展示 |
| 9 | 文档列表 | 已实现 | `GET /projects/:p/documents` | 支持分页、精确目录、关键字、解析状态和白名单排序；最大页大小 100 |
| 10 | 单文件上传 | 已实现 | `POST /projects/:p/documents` | 需要 `upload` capability 和 CSRF；文件名、大小、类型及相对路径校验 |
| 11 | 包含文件的目录上传 | 已实现 | 同上 | 保留 `webkitRelativePath`；拒绝绝对路径、空段和 `..` 穿越 |
| 12 | 空目录持久化 | 有限 | 无 | 浏览器目录选择不会发送空目录；当前只保存目录中的文件，未显示虚假“新建空目录”操作 |
| 13 | 解析状态/轨迹 | 已实现 | `POST /documents/status`、`GET /documents/:id/stages` | 原生轮询恢复；终态停止；文档必须属于绑定 KB |
| 14 | 失败文档重试 | 已实现 | `POST /documents/:id/reparse` | DTO 的 `can_reparse` 仅在有上传权限、本人上传、failed/cancelled 时为真；其他人不显示重试 |
| 15 | 文档原文预览 | 已实现 | `GET /documents/:id/preview` | Cookie 请求；跨 KB 文档返回 404；不使用普通 Bearer 下载 URL |
| 16 | 回答引用片段 | 已实现 | `GET /documents/:id/chunks`、`GET /documents/:id/chunks/:chunkId`、`GET /chunks/:chunkId` | 会话先按用户/项目校验，文档和 chunk 再按绑定 KB 校验；复用原生抽屉和来源预览 |
| 17 | 搜索/筛选/排序 | 已实现（受限） | `GET /projects/:p/documents` | 当前支持关键字、解析状态、目录和 `updated_at/created_at/file_name` 排序；未接入的原生筛选不显示 |
| 18 | 标签/批量/移动/重命名/删除 | 隐藏 | 无对应项目写接口 | 项目模式不显示标签编辑、移动、删除、批量、下载和取消解析；不能通过菜单进入普通 API |
| 19 | Wiki/FAQ/图库/Agent 设置 | 隐藏/按类型受限 | 无通用项目管理接口 | 普通文档 KB 只显示文档；设置、跨 KB、租户、Agent/MCP/工具入口不注册或不渲染 |
| 20 | PixLab 生产页面路由 | 已实现 | `/weknora-workbench/` | PixLab Nginx 精确代理到 `weknora-frontend:80`；子路由回退由 WeKnora workbench bundle 处理 |
| 21 | PixLab 生产 API 路由 | 已实现 | `/api/v1/pixlab-workbench/` | 精确代理到 `weknora-app:8080`；关闭 buffering；上传上限 256 MiB；不代理普通 `/api/v1/*` |

## 项目模式隐藏控件

项目模式不会渲染或不会注册以下能力：租户切换、全局知识库列表、设置、邀请、命令面板、全局搜索、Agent 创建/切换、模型选择、Web 搜索、跨 KB `@`、聊天附件、MCP、工具、沙箱、知识库删除/设置、文档下载/删除/移动/重命名/标签/批量操作、手工文档、URL 导入和空目录创建。

隐藏这些控件不是视觉裁剪，而是因为项目受限 API 没有相应授权语义。普通 WeKnora SPA 继续使用原有 JWT API 和完整功能，不受项目模式限制。

## 授权与隔离证据

- `internal/application/service/pixlab_workbench_test.go`：项目、CSRF、绑定版本、read capability、CSRF 旋转和 Redis TTL。
- `internal/handler/pixlab_workbench_test.go`：上传路径、可信元数据、跨用户/跨项目会话、删除/停止、跨 KB 文档/preview/chunk、重试所有权/状态和缺失/错误 CSRF。
- `internal/handler/session/authorization_lifetime_test.go`：项目宿主撤权信号会取消后台问答；普通 WeKnora 请求不受影响。
- `frontend/src/pixlab-workbench/*.test.ts`：受限 transport、Cookie/CSRF、SSE、nonce 和路由 allowlist。
- `frontend/src/composables/pixlabChatHistory.test.ts`：消息游标、去重和终页判定。
- `frontend/src/views/platform/projectDropPolicy.test.ts`：项目聊天禁附件拖放、项目 KB 仅在有上传能力时接收拖放。

运行态 A/B/Q 验收必须使用两个 PixLab 用户和两个绑定到不同 KB 的项目：P 成员 A/B 共享文档但看不到彼此会话；Q 用户不能通过 P URL 读取 session、document、chunk 或 preview。单元测试是合并门槛，不替代发布环境浏览器证据。

## 已知限制

- 空目录不持久化；只保留目录中已上传文件的相对路径。
- 项目模式不开放 URL/手工文档上传、文档删除/移动/重命名/标签/批量/下载。
- 失败或取消文档只允许原上传者重试；其他项目成员仍可查看共享状态和处理轨迹。
- Wiki、FAQ、图库只在后续补齐项目受限接口和产品权限规则后开放。
- 项目聊天不开放模型、Web 搜索、Agent 切换、跨 KB 引用、附件、MCP、工具或沙箱。
- 项目 router 不提供普通 WeKnora 管理和设置路由。

部署网络、环境变量、健康检查和无删库回滚步骤见 `docs/embed-workbench/01-pixlab-deployment.md`。
