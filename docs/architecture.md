# ChatDock 代码分层说明

ChatDock 是 Go 后端 + React 前端的单体项目，生产形态是 Go embed + Docker Compose 单服务部署。代码分层目标是让“产品界面、API 调用、业务状态、持久化、模型/MCP 集成”边界清晰，避免所有逻辑继续堆进单个文件。

## 顶层目录

```text
cmd/chatdock/              进程入口，只负责读取环境变量并启动 App
internal/chatdock/         Go 后端核心包，保持单 package，按职责拆文件
web/                       前端源码与 Go embed 入口
web/src/components/        React 组件层
web/src/lib/               前端 API、传输层、协议解析和纯函数
web/src/styles/            前端 CSS 模块
```

## 后端分层

当前后端保持 `internal/chatdock` 单 package。这样可以避免 Go 项目为了“层级感”提前拆 package、抽 interface 或制造循环依赖；治理方式是先把同一业务域的 handler 和 Store 方法拆到清晰文件里。

```text
app.go                    HTTP 路由、静态资源、App 组装
config.go / types.go      配置和 DTO
errors.go / id.go         通用错误和 ID 工具

handler.go                HTTP JSON / SSE 读写 helper
handler_health.go         健康检查
handler_config.go         模型配置与 Prompt 工作空间兼容入口
handler_mcp.go            MCP 配置、工具列表、测试和工具调用
handler_skills.go         技能 CRUD handler
handler_tasks.go          自动化任务 CRUD 与立即运行 handler
handler_sessions.go       会话 CRUD、导出、置顶、复制、重命名 handler
handler_chat.go           普通聊天、流式聊天和聊天工具链入口

product.go                产品化 API 的共享响应结构
product_setup.go          首次配置和初始化状态
product_workspace.go      工作空间列表、切换、配置、Prompt 预览
product_model.go          模型供应商、模型连接测试、模型列表
product_data.go           SQLite、WAL、备份和数据健康状态
product_system.go         系统状态和 MCP 状态聚合

chat_jobs.go              流式/后台聊天任务
store.go                  Store 生命周期、DB 连接、进程内状态
store_db.go               SQLite 初始化、旧 JSON 迁移、meta
store_prompt.go           工作空间 / Prompt / 模型配置 / MCP 配置
store_sessions.go         会话 CRUD、消息追加、会话标题与预览
store_files.go            文件读写、JSON 格式化、DB 时间格式工具
attachments.go            附件上传、落盘、文本提取、模型上下文注入
llm.go / llm_tools.go     OpenAI 兼容模型请求、自动上下文整理与工具调用适配
mcp_client.go             MCP 客户端
runs.go                   MCP 执行记录
scheduled_tasks.go        自动化任务存储与执行
skills.go                 工作空间技能
```

### 后端新增代码规则

1. 新增 HTTP handler 先放到对应 `handler_*.go`；没有明确归属时再扩展新的业务文件，不回填到巨型 `handler.go`。
2. 产品化状态 API 放到 `product_*.go`；`product.go` 只放共享 DTO，不再继续堆实现。
3. Store 仍保持直接方法调用，不提前抽 repository/interface；只有真实需要跨 package 复用时再讨论 package 拆分。
4. 复杂业务流程需要中文注释说明原因、约束和坑点，不写“解释语法”的无效注释。
5. 聊天上下文默认使用 `context_mode=auto`：最近消息保留原文，更早消息在模型请求前提炼成系统摘要；只有 `custom` 模式才把 `max_context_messages` 当作硬性最近消息数。

## 前端分层

`web/src/App.jsx` 只保留应用状态编排、路由状态、事件处理和组件组合。API 请求、SSE、上传、Markdown 和纯函数放到 `web/src/lib/`；可复用界面放到 `web/src/components/`。

```text
web/src/App.jsx             应用状态、页面编排、事件处理
web/src/main.jsx            React 入口和样式入口

web/src/lib/http.js         JSON API client、鉴权 header 和错误归一化
web/src/lib/chatApi.js      聊天流、后台任务事件流
web/src/lib/sessionApi.js   会话列表、CRUD、导出
web/src/lib/settingsApi.js  配置中心、工作空间、模型、MCP、技能、自动化、数据状态 API
web/src/lib/upload.js       文件上传传输层，封装 XHR 和进度回调
web/src/lib/sse.js          SSE 流解析
web/src/lib/markdown.js     Markdown 渲染
web/src/lib/appUtils.js     时间、容量、状态标签、路由路径、诊断文本等纯函数

web/src/components/base.jsx      通用 UI：Markdown、弹窗、登录页、快捷面板、工作空间选择
web/src/components/chat.jsx      对话工作台、消息、附件卡片、空状态
web/src/components/settings.jsx  配置中心：工作空间、模型、上下文模式、技能、MCP、运行记录、自动化、数据、安全
```

### 前端新增代码规则

1. 新增接口不要直接在组件深处 `fetch`，优先放到 `web/src/lib/*Api.js`。
2. `App.jsx` 可以持有状态和组合流程，但不要继续塞协议解析、请求细节或通用格式化函数。
3. 可复用 UI 放 `components/`；只被单一页面使用且和状态强绑定的回调可以留在 `App.jsx`，避免为了拆而拆。
4. 上传、SSE、Markdown 这类传输/协议能力保持独立文件，方便后续测试和替换。
5. 模型上下文设置对普通用户只展示自动、精简、更多历史和自定义；不要让用户必填底层 JSON 或消息数量。

## CSS 分层

`web/src/app.css` 只保留样式入口，真实样式拆到 `web/src/styles/`：

```text
web/src/app.css             CSS import 入口
web/src/styles/tokens.css   主题变量、暗色/亮色 token、产品视觉 token
web/src/styles/layout.css   应用外壳、侧栏、顶部栏、基础控件和桌面布局
web/src/styles/chat.css     消息流、Markdown、输入框、空状态、工作台状态条
web/src/styles/settings.css 配置中心、工作空间、模型、MCP 表单、技能和自动化表单
web/src/styles/overlays.css 登录页、弹窗、Toast、快捷面板、工作空间/会话操作浮层
web/src/styles/mobile.css   手机端布局、触控优化、安全区和小屏覆盖规则
web/src/styles/data.css     数据状态、备份、诊断和运行记录样式
```

CSS 允许按产品迭代继续覆盖，但新增规则必须进入对应模块，不再把大段样式追加到 `app.css`。

## 部署和数据规则

1. 生产部署必须使用 `/Volumes/KIOXIA/Docker/chatdock/compose.yaml`，不能用源码仓库示例 compose 直接重建生产容器。
2. 涉及生产数据前必须先确认挂载和备份 SQLite 三件套：`.sqlite`、`.sqlite-wal`、`.sqlite-shm`。
3. 修改后至少运行前端构建和后端测试；部署后必须真实请求健康接口或页面接口验证。
4. Git commit message 使用旧格式：`type(scope): 中文说明`。
