# ChatDock 代码分层说明

ChatDock 是 Go 后端 + React 前端的单体项目，生产形态是 Go embed + Docker Compose 单服务部署。当前治理目标是：后端业务边界清晰、前端状态和协议逻辑逐步下沉到 `lib/` 与组件层、CSS 新增规则进入明确模块，避免继续把补丁堆进入口文件。

## 顶层目录

```text
cmd/chatdock/              进程入口，只负责读取环境变量和建立系统信号上下文
cmd/chatdock-migrate-workspaces/ 旧工作空间数据库的一次性迁移入口
internal/app/              进程级依赖组装和服务生命周期
internal/httpapi/          HTTP Server、路由、handler 与跨域请求编排
internal/agentdock/        AgentDock HTTP、上下文缓存和任务事件解析
internal/attachment/       附件文件名规范化和文本提取
internal/chatoutput/       流式输出、工具时间线和消息 checkpoint
internal/legacyworkspace/  旧工作空间数据库离线迁移
internal/llm/              OpenAI 兼容请求、流式、工具调用和 embedding
internal/mcp/              MCP 客户端、配置、缓存和过滤规则
internal/model/            DTO、配置和模型类型
internal/modelprovider/    模型供应商记录、Key 策略、校验和持久化
internal/schedule/         Cron、时区、任务校验和下一次运行计算
internal/store/            当前 SQLite schema、会话、任务、附件和运行记录
internal/toolapproval/     工具人工确认等待、决策和状态流转
internal/toolschema/       工具参数 JSON Schema 校验
web/                       前端源码与 Go embed 入口
web/src/components/        React 组件层
web/src/lib/               前端 API、传输层、协议解析和纯函数
web/src/styles/            前端 CSS 模块；大文件用同名子目录分块，入口只 import
scripts/                   本地检查、生产部署保护和辅助脚本
```

## 后端分层

`internal/app` 是唯一进程组合根，只负责创建 `httpapi.Server`、启动监听，并在系统上下文结束时执行有界关闭。`internal/httpapi` 负责 HTTP 路由、鉴权、handler 和跨域请求编排，不承载外部系统客户端、离线迁移、供应商内部记录或纯调度计算。稳定且依赖方向清楚的能力直接位于 `internal/` 下；不按 handler/service/repository 机械分层，也不引入 `common`、`utils` 或空泛 manager。

```text
internal/app/run.go             进程级启动、监听错误和有界关闭
internal/httpapi/server.go      Server 依赖组装、HTTP 生命周期和后台任务注册
internal/httpapi/routes.go      产品路由注册
internal/httpapi/handler.go     JSON/SSE 请求响应边界
internal/httpapi/handler_*.go   配置、MCP、会话、聊天和自动化任务 handler
internal/httpapi/chat_jobs.go   后台聊天任务与事件流编排
internal/httpapi/scheduler.go   定时任务扫描与触发编排
internal/httpapi/attachments.go 附件上传、落盘、公开签名图片 URL
internal/httpapi/model_providers.go 模型供应商测试、候选模型和 API handler
internal/httpapi/agentdock_tasks.go AgentDock 任务 HTTP 适配
```

`internal/store/` 只负责当前 SQLite 生命周期、schema、项目、会话、定时任务、运行记录、附件和工具向量。模型供应商内部记录、Key 策略、校验和 meta 持久化由 `modelprovider` 负责，Store 只编排事务与全局模型配置。应用启动显式拒绝旧工作空间 schema；离线转换完整归属 `legacyworkspace`，不再与运行时 Store 混在同一个包。

`agentdock` 是具体外部 HTTP 边界；`chatoutput` 承接模型流式输出到消息 checkpoint 的完整链路；`attachment` 和 `toolschema` 承接无状态的附件预处理与工具参数校验；`toolapproval` 持有人工确认的等待 channel、超时和 SQLite 状态流转；`schedule` 只包含无数据库依赖的 Cron、时区、任务校验和下一次运行计算。依赖方向统一由组合根指向具体能力包。

Store 暂时不按实体继续拆成多个 repository：会话、定时任务运行、附件引用和聊天任务存在必须同事务提交的不变量。后续只有在能保持这些原子边界时，才继续拆具体存储服务，不能用代理方法或跨包回调制造“看起来分层”的结构。

### 后端新增代码规则

1. 新增 HTTP handler 先放到 `internal/httpapi/handler_*.go`；外部网络、持久化记录或稳定领域规则不得因为访问 Server/Store 方便而继续堆进 HTTP 编排层。
2. Store 保持直接方法调用，不提前抽 repository/interface；只有数据库读写和外部 HTTP 这类真实边界使用最小接口或具体 Client。
3. 破坏性 Schema 变更直接维护当前 schema；生产升级必须在独立迁移包和命令中提供完整备份、转换与验证，不在应用启动时隐式修改旧库。
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
web/src/lib/settingsApi.js      配置中心、项目、全局模型、MCP、自动化、数据状态 API
web/src/lib/upload.js           文件上传传输层，封装 XHR 和进度回调
web/src/lib/sse.js              SSE 流解析
web/src/lib/toolEvents.js       工具调用事件合并、展示文案和 message parts 更新
web/src/lib/sessionPresentation.js 会话列表和定时任务运行会话的纯展示派生逻辑
web/src/lib/modelProviderForm.js 模型供应商表单 Key、模型名和选择器纯函数
web/src/hooks/useSettingsData.js 配置页数据状态和加载器 hook
web/src/hooks/useAttachments.js  文件上传、附件列表、下载和 ready attachment 派生状态
web/src/lib/appUtils.js         时间、容量、状态标签、路由路径、诊断文本等纯函数

web/src/components/base.jsx     通用 UI：Markdown、弹窗、登录页、快捷面板
web/src/components/chat.jsx     对话工作台、消息、附件卡片、空状态
web/src/components/appChrome.jsx 主应用壳层：Sidebar、Topbar、ComposerBar
web/src/components/managementPages.jsx 项目与定时任务管理页
web/src/components/settings.jsx 配置中心：模型、供应商、MCP、系统与数据
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
web/src/styles/settings/*.css       配置中心子模块：基础、MCP、页面布局、模型、主题、移动端、布局壳层和视觉收敛层
web/src/styles/overlays.css         登录页、弹窗、Toast、快捷面板和会话操作浮层
web/src/styles/mobile.css           手机端布局、触控优化、安全区和小屏覆盖规则
web/src/styles/data.css             数据状态、备份、诊断和运行记录样式
```

新增样式必须进入对应模块。配置页不得继续新增“final/override”式补丁文件；确实需要跨模块收敛时放入明确命名的 shell/visual 层并同步 CSS 健康预算。禁止新增空兼容层或用 `!important` 堆叠覆盖旧规则。

## 持久化与生命周期不变量

以下边界属于业务正确性的一部分，后续修改不能拆回多次独立提交：

- 创建/删除项目必须只影响项目记录和会话归属；删除项目后会话保留并转为普通会话。
- 保存全局模型配置或供应商时，供应商元数据、全局配置和工具向量缓存失效必须原子提交；数据库或 JSON 读取错误必须显式返回，不能静默替换成默认值。
- 发起对话时，用户消息、附件绑定、会话模型和 ChatJob 创建必须一次提交；定时任务的启动与完成也必须让会话消息、任务状态和运行记录同生共死。
- 相同 SHA-256 的附件统一引用 `attachment_blobs` 中的 canonical path；删除会话或附件引用变更后必须重算引用数，零引用且文件已丢失的 Blob 才允许由新上传接管路径。
- 一次性生产迁移必须在独立工具中完成数据转换和结果校验；失败时保留原数据库备份，不允许应用带着半迁移数据启动。

所有长期 goroutine 都必须由 `httpapi.Server` 的生命周期入口启动，继承服务 context，并计入统一等待组。`internal/app` 接收进程上下文并触发有界 `Shutdown`；SIGTERM 会先停止接收新任务、取消现有请求和后台任务，再等待 HTTP 与后台工作退出。超时前不能提前关闭仍可能被使用的 Store，显式 `Close` 才执行强制等待和最终资源回收。

远程图片探测使用独立 Transport：连接阶段重新解析域名、拒绝私网和特殊用途地址，并直接拨号已验证公网 IP，避免 DNS 校验与实际连接之间的时间差绕过。

## 部署和数据规则

1. 生产部署必须使用 `/Volumes/KIOXIA/Docker/chatdock/compose.yaml`，不能在源码仓库目录用 compose 重建生产容器。
2. 源码仓库只保留 `compose.dev.yaml` 作为开发示例；生产操作使用 `make deploy-prod`。部署流程必须先用 `make prod-check` 验证容器 label 和 `/data` mount，再用 `make prod-health` 携带容器现有鉴权令牌等待应用健康响应。
3. 涉及生产数据前必须先确认挂载和备份 SQLite 三件套：`.sqlite`、`.sqlite-wal`、`.sqlite-shm`。
4. 新建数据目录和上传目录使用 `0700`，SQLite 与附件使用 `0600`；历史数据权限只在完成备份并确认挂载后显式迁移，应用启动时不隐式 chmod。
5. 修改后至少运行前端构建和后端测试；部署后必须真实请求健康接口或页面接口验证。
6. Git commit message 使用旧格式：`type(scope): 中文说明`。
