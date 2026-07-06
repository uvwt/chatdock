# ChatDock 代码分层说明

ChatDock 是 Go 后端 + React 前端的单体项目，生产形态是 Go embed + Docker Compose 单服务部署。当前治理目标是：后端业务边界清晰、前端状态和协议逻辑逐步下沉到 `lib/` 与组件层、CSS 新增规则进入明确模块，避免继续把补丁堆进入口文件。

## 顶层目录

```text
cmd/chatdock/              进程入口，只负责读取环境变量并启动 App
internal/chatdock/         Go 后端核心包，保持单 package，按职责拆文件
internal/chatdock/llm/     OpenAI 兼容请求、流式、工具调用和 embedding
internal/chatdock/mcp/     MCP 客户端、配置、缓存和过滤规则
internal/chatdock/model/   DTO、配置和模型类型
internal/chatdock/store/   SQLite 存储、迁移、会话、任务、附件、供应商
web/                       前端源码与 Go embed 入口
web/src/components/        React 组件层
web/src/lib/               前端 API、传输层、协议解析和纯函数
web/src/styles/            前端 CSS 模块
scripts/                   本地检查、生产部署保护和辅助脚本
```

## 后端分层

后端当前保持 `internal/chatdock` 单 package。新增代码优先按业务域拆文件，不为了层级感提前抽 interface。

```text
app.go / routes.go              App 组装、HTTP 路由和服务启动
setup.go / workspaces.go        首次配置、工作空间列表、切换和初始化
handler.go                      JSON/SSE 读写 helper
handler_health.go               健康检查
handler_config.go               模型配置与 Prompt 工作空间兼容入口
handler_mcp.go                  MCP 配置、工具列表、测试和工具调用
handler_tasks.go                自动化任务 CRUD 与立即运行
handler_sessions.go             会话 CRUD、导出、置顶、复制、重命名、工具事件详情
handler_chat.go                 普通聊天、流式聊天入口
chat_jobs.go                    后台聊天任务与事件流
scheduler.go                    定时任务扫描与触发
attachments.go                  附件上传、落盘、公开签名图片 URL
model_providers.go              模型供应商测试、候选模型和 API handler
system_status.go / data_status.go 系统状态和数据健康状态
preflight_tools.go              工具预检和 AgentDock 上下文提示
runs.go / session_*.go          工具运行记录、会话标题、搜索和懒加载事件
```

`internal/chatdock/store/` 负责 SQLite 生命周期、schema migration、旧 JSON 迁移、工作空间、会话、定时任务、运行记录、附件、工具向量和模型供应商。模型供应商存储按 CRUD 入口、持久化、公开 DTO、校验/规范化、Key 策略拆分。Schema 变更必须通过 `schema_migrations.go` 增加版本，不再只靠裸 `ALTER TABLE` 兜底。

### 后端新增代码规则

1. 新增 HTTP handler 先放到对应 `handler_*.go`；没有明确归属时再扩展新的业务文件，不回填到巨型 `handler.go`。
2. Store 仍保持直接方法调用，不提前抽 repository/interface；只有真实需要跨 package 复用时再讨论 package 拆分。
3. Schema 变更必须新增 migration 版本，并考虑旧库重复字段、生产备份和回滚证据。
4. 复杂业务流程需要中文注释说明原因、约束和坑点，不写“解释语法”的无效注释。
5. 聊天上下文默认使用 `context_mode=auto`：最近消息保留原文，更早消息在模型请求前提炼成系统摘要；只有 `custom` 模式才把 `max_context_messages` 当作硬性最近消息数。

## 前端分层

`web/src/App.jsx` 只保留应用状态编排、路由状态、事件处理和组件组合。API 请求、SSE、上传、Markdown、工具事件协议和纯函数放到 `web/src/lib/`；可复用界面放到 `web/src/components/`。

```text
web/src/App.jsx                 应用状态、页面编排、事件处理
web/src/main.jsx                React 入口和样式入口

web/src/lib/http.js             JSON API client、鉴权 header 和错误归一化
web/src/lib/chatApi.js          聊天流、后台任务事件流
web/src/lib/sessionApi.js       会话列表、CRUD、导出
web/src/lib/settingsApi.js      配置中心、工作空间、模型、MCP、自动化、数据状态 API
web/src/lib/upload.js           文件上传传输层，封装 XHR 和进度回调
web/src/lib/sse.js              SSE 流解析
web/src/lib/toolEvents.js       工具调用事件合并、展示文案和 message parts 更新
web/src/lib/modelProviderForm.js 模型供应商表单 Key、模型名和选择器纯函数
web/src/hooks/useSettingsData.js 配置页数据状态和加载器 hook
web/src/hooks/useAttachments.js  文件上传、附件列表、下载和 ready attachment 派生状态
web/src/lib/appUtils.js         时间、容量、状态标签、路由路径、诊断文本等纯函数

web/src/components/base.jsx     通用 UI：Markdown、弹窗、登录页、快捷面板、工作空间选择
web/src/components/chat.jsx     对话工作台、消息、附件卡片、空状态
web/src/components/settings.jsx 配置中心：工作空间、模型、供应商、MCP、自动化、数据、安全
```

### 前端新增代码规则

1. 新增接口不要直接在组件深处 `fetch`，优先放到 `web/src/lib/*Api.js`。
2. `App.jsx` 可以持有页面编排和跨域流程，但配置页数据加载放入 `useSettingsData`，附件上传下载放入 `useAttachments`，不要继续塞请求细节或通用格式化函数。
3. 可复用 UI 放 `components/`；只被单一页面使用且和状态强绑定的回调可以留在 `App.jsx`，避免为了拆而拆。
4. 上传、SSE、Markdown、工具事件协议这类传输/协议能力保持独立文件，方便后续测试和替换。
5. 模型上下文设置对普通用户只展示自动、精简、更多历史和自定义；不要让用户必填底层 JSON 或消息数量。
6. 前端结构守卫放在 `scripts/check-frontend.mjs`，新增重构规则时同步加入检查；纯函数优先放入 `web/src/lib/` 并在守卫中添加最小断言。

## CSS 分层

`web/src/app.css` 只保留样式入口，真实样式拆到 `web/src/styles/`：

```text
web/src/app.css                     CSS import 入口，不写真实规则
web/src/styles/tokens.css           主题变量、暗色/亮色 token、产品视觉 token
web/src/styles/layout.css           应用外壳、侧栏、顶部栏、基础控件和桌面布局
web/src/styles/chat.css             消息流、Markdown、输入框、空状态、工作台状态条
web/src/styles/settings.css         配置页样式入口，只保留 settings 子模块 import
web/src/styles/settings/*.css       配置中心子模块：基础、MCP、页面布局、模型、主题、移动端、最终视觉层
web/src/styles/overlays.css         登录页、弹窗、Toast、快捷面板、工作空间/会话操作浮层
web/src/styles/mobile.css           手机端布局、触控优化、安全区和小屏覆盖规则
web/src/styles/data.css             数据状态、备份、诊断和运行记录样式
web/src/styles/legacy-overrides.css 空兼容层；不允许新增真实样式
```

新增样式必须进入对应模块。`legacy-overrides.css` 只能保持空兼容层，禁止继续追加 `!important` 或真实业务规则。

## 部署和数据规则

1. 生产部署必须使用 `/Volumes/KIOXIA/Docker/chatdock/compose.yaml`，不能在源码仓库目录用 compose 重建生产容器。
2. 源码仓库只保留 `compose.dev.yaml` 作为开发示例；生产操作使用 `make deploy-prod`，部署后用 `make prod-check` 检查容器 label 和 `/data` mount。
3. 涉及生产数据前必须先确认挂载和备份 SQLite 三件套：`.sqlite`、`.sqlite-wal`、`.sqlite-shm`。
4. 修改后至少运行前端构建和后端测试；部署后必须真实请求健康接口或页面接口验证。
5. Git commit message 使用旧格式：`type(scope): 中文说明`。
