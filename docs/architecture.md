# ChatDock 代码分层说明

ChatDock 是 Go 后端 + React 前端的单体项目，生产形态是 Go embed + Docker Compose 单服务部署。代码分层目标是让“产品界面、API 调用、业务状态、持久化、模型/MCP 集成”边界清晰，避免所有逻辑继续堆进单个文件。

## 顶层目录

```text
cmd/chatdock/              进程入口，只负责读取环境变量并启动 App
internal/chatdock/         Go 后端核心包
web/                       前端源码与 Go embed 入口
web/src/components/        React 组件层
web/src/lib/               前端纯函数、传输层和协议解析
```

## 后端分层

当前后端仍保持 `internal/chatdock` 单 package，避免一次性跨 package 搬迁破坏接口；但职责按文件分层：

```text
app.go                    HTTP 路由、静态资源、App 组装
handler.go                聊天 HTTP handler
chat_jobs.go              流式/后台聊天任务
store.go                  SQLite 持久化入口和事务协调
attachments.go            附件上传、落盘、文本提取、模型上下文注入
llm.go / llm_tools.go     OpenAI 兼容模型请求与工具调用适配
mcp_client.go             MCP 客户端
runs.go                   MCP 执行记录
scheduled_tasks.go        自动化任务
skills.go                 工作空间技能
product.go                产品化状态、健康检查、数据状态
config.go / types.go      配置和 DTO
```

后续继续拆分时，优先把 `store.go` 拆成同 package 文件，而不是马上拆 package：

```text
store_db.go               SQLite 初始化、迁移、meta
store_prompt.go           工作空间 / prompt 配置
store_sessions.go         会话 CRUD
store_jobs.go             chat_jobs 持久化
store_mcp_runs.go         MCP runs 持久化
```

## 前端分层

`web/src/App.jsx` 只保留应用状态编排、路由状态、事件处理和组件组合。新增分层如下：

```text
web/src/lib/appUtils.js       时间、容量、状态标签、路由路径、诊断文本等纯函数
web/src/lib/upload.js         文件上传传输层，封装 XHR 和进度回调
web/src/lib/sse.js            SSE 流解析
web/src/lib/markdown.js       Markdown 渲染
web/src/components/base.jsx   通用 UI：Markdown、卡片、弹窗、登录页、快捷面板、工作空间选择
web/src/components/chat.jsx   对话工作台、消息、附件卡片、空状态
web/src/components/settings.jsx 配置中心：工作空间、模型、技能、MCP、运行记录、自动化、数据、安全
```

## 规则

1. 新增 API 请求封装优先放 `web/src/lib/`，不要直接塞进组件深处。
2. 新增可复用 UI 优先放 `web/src/components/`，`App.jsx` 只做组合。
3. 新增后端能力先按业务文件拆分；只有边界稳定后再考虑 Go 子 package。
4. 生产部署必须使用 `/Volumes/KIOXIA/Docker/chatdock/compose.yaml`，不能用源码仓库示例 compose 直接重建生产容器。
5. 涉及生产数据前必须先确认挂载和备份 SQLite 三件套。
