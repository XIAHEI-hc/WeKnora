# WeKnora 原生工作台嵌入文档

本目录描述的是 WeKnora 面向多个业务平台的“受限原生项目工作台”集成方式。它不是
PixLab 专属功能，也不是把一个公开聊天 Widget 复制到不同系统中。每个宿主平台使用
自己的身份、权限和安全桥，WeKnora 再按服务端项目绑定选择该平台获准使用的知识库与
Agent。

## 阅读顺序

| 文档 | 适用场景 |
|---|---|
| [多平台嵌入架构](./03-multi-platform-embedding.md) | 先理解宿主、项目、知识库、用户命名空间和安全边界 |
| [原生项目工作台能力矩阵](./02-native-project-capabilities.md) | 确认复用的 WeKnora 原生页面、受限 API 和已知限制 |
| [PixLab 部署](./01-pixlab-deployment.md) | 部署或验收 PixLab/CIS 项目知识服务 |
| [MemoryLab 部署](./02-memorylab-deployment.md) | 部署或验收 MemoryLab/MRA 项目知识服务 |
| [前期技术侦察](./00-phase0-recon.md) | 查看方案形成前的代码、协议和安全分析，仅作历史依据 |

## 当前支持范围

| 宿主平台 | 服务端项目 | 用户命名空间 | 独立授权桥 | 平台部署文档 |
|---|---|---|---|---|
| PixLab | 由 PixLab 项目目录决定，例如 `WK-E2E-20261005` | `pixlab:` | `X-PixLab-*` | [01-pixlab-deployment.md](./01-pixlab-deployment.md) |
| MemoryLab | `MEMORYLAB_MRA` | `memorylab:` | `X-MemoryLab-*` | [02-memorylab-deployment.md](./02-memorylab-deployment.md) |

新增第三个平台时，不能复用另一个平台的项目代码、HMAC 密钥、用户前缀或 Docker
网络。必须按照[多平台嵌入架构](./03-multi-platform-embedding.md)增加受信任宿主适配器，
建立独立项目绑定，并完成正向检索和反向越界两类验收。

> 当前 `/api/v1/pixlab-workbench/`、`pixlab_project_bindings` 以及部分 `PixLab*` 代码名
> 是首个实现留下的兼容 wire contract。名称不代表能力只属于 PixLab，也不能作为跨
> 平台共享身份或知识范围的理由。变更这些兼容名称需要单独的版本化迁移。
