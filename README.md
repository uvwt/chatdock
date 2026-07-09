# ChatDock

ChatDock 是一个自用的轻量 AI 对话中控台，目标是：提示词可控、上下文可控、模型可控、尽量省 token，并可选接入 MCP 工具。

## 当前功能

- Go 后端 + Vite/React 前端，生产构建后通过 `//go:embed` 嵌入最终 Go 二进制。
- SQLite 单文件存储，默认数据文件为 `chatdock.sqlite`。
- OpenAI Chat Completions 兼容接口。
- 可配置 Base URL / API Key / Model / System Prompt。
- 可配置最近上下文消息数，用来控制 token 消耗。
- 可通过 `chat_template_kwargs.enable_thinking` 控制模型思考开关。
- 可隐藏 `<think>...</think>` 思考内容。
- 工作空间：每个空间独立保存模型配置、MCP 配置、自动化任务和会话。
- 自动化任务：每个工作空间可维护一组本地定时提示，支持一次性、每日、间隔执行，并把运行结果写入任务运行记录和关联会话。
- 会话创建、列表、删除、重命名、置顶、全文复制、复制会话、Markdown 导出；会话列表显示最近消息摘要，搜索同时匹配标题和摘要，置顶会话固定在列表顶部。
- 无 MCP 工具时保留真正流式输出；启用 MCP 工具时通过 SSE 输出工具调用事件和最终回答。
- MCP HTTP JSON-RPC 客户端：支持 `tools/list`、`tools/call`、Bearer token、server 超时、工具列表缓存、工具 allow/deny/confirm 规则。
- 产品化前端：独立账号密码登录页、配置中心抽屉、工作空间切换器、快捷指令面板（`⌘/Ctrl K`）、移动端会话操作面板、空状态引导和 PWA manifest。
- 数据状态页：展示数据库大小、WAL 状态、工作空间/会话数量，并自动探测同级 `backups` 目录中的最近数据库备份和数据库备份列表，配置备份文件不会进入产品界面。
- 自助诊断：数据状态页会标记数据库备份健康状态和最近备份年龄；安全页与快捷指令可复制脱敏后的诊断信息，默认只暴露目录名和文件名，不输出本机绝对路径。

## 项目结构

```text
chatdock/
├── cmd/chatdock/          # 程序入口
├── internal/chatdock/     # 后端核心代码
├── web/                   # Vite/React 前端源码、构建脚本和 Go embed 包
│   ├── src/               # React 前端源码
│   ├── public/            # PWA manifest、图标等静态资源
│   ├── dist/              # 生产构建产物，由 make web-build 生成，不提交
│   ├── embed.go           # //go:embed dist，供 Go 后端托管
│   └── package.json
├── deploy/                # launchd 示例
├── Dockerfile
├── compose.dev.yaml       # 源码目录开发示例；生产不要用这个文件部署
├── Makefile
└── go.mod
```

## 本机运行

```bash
make run
```

访问：

```text
http://127.0.0.1:8720
```

环境变量：

```bash
CHATDOCK_ADDR=:8720
CHATDOCK_DATA=~/.config/chatdock   # 可选；默认使用系统用户配置目录下的 chatdock
CHATDOCK_WEB=/path/to/web/dist     # 可选；为空时使用二进制内嵌的 web/dist
CHATDOCK_AUTH_TOKEN=your-token              # 可选；设置后 API/MCP 需要 Bearer Token，静态前端仍可访问
CHATDOCK_AUTH_USERNAME=admin                 # 可选；启用账号密码登录页时使用
CHATDOCK_AUTH_CREDENTIAL=your-pass           # 可选；启用账号密码登录页时使用
CHATDOCK_EMBEDDING_BASE_URL=http://m3/v1     # 可选；OpenAI 兼容 /embeddings，用于工具向量混合搜索
CHATDOCK_EMBEDDING_API_KEY=your-embedding-key # 可选；M3 embedding 服务密钥
CHATDOCK_EMBEDDING_MODEL=BAAI/bge-m3         # 可选；默认 BAAI/bge-m3
```

## 前后端一体化构建

ChatDock 采用类似 Memos 的一体化部署方式：

1. `web/src` 通过 Vite/React 构建到 `web/dist`。
2. `web/embed.go` 使用 `//go:embed dist` 将构建产物嵌入 Go 二进制。
3. Go `net/http` 在同一个端口下托管静态前端、后端 API 和 MCP 相关能力。
4. 非 `/api`、非 `/mcp` 的页面路径会 fallback 到 `index.html`，支持 SPA 前端路由；API/MCP 缺失路由保持后端 404。

常用命令：

```bash
make web-build   # 安装/校验前端依赖并生成 web/dist
make build       # 构建内嵌前端的单个 Go 二进制
make run         # 先构建前端，再 go run
make js-check        # 检查前端配置、lib/hook 脚本语法
make css-check       # check CSS modules and budgets
make bundle-check    # build front-end and check bundle budget
make frontend-test   # 运行前端纯逻辑测试
make check           # fmt-check + frontend-lint + frontend-test + bundle-check + vet + test + build
```

`make check` 会先运行前端结构守卫、CSS 健康预算、前端纯逻辑测试，生成前端 dist，并执行：`fmt-check`、`go vet ./...`、`go test ./...`、`go build`。前端守卫会确保 `app.css` 和 `styles/settings.css` 都只作为 import 入口，配置页数据加载保留在 `useSettingsData`，附件上传下载保留在 `useAttachments`，聊天展示/工具事件详情不回流到 `App.jsx`，历史样式只允许保留空兼容层。仓库还包含 GitHub Actions 最小 CI，覆盖 CSS 健康预算、前端测试、前端构建、Go 测试、vet、commit message 格式和 `git diff --check`。如果只想调试磁盘静态目录，可以设置 `CHATDOCK_WEB=/path/to/web/dist` 覆盖内嵌资源。

## 数据存储

ChatDock 使用 SQLite 作为持久化存储，默认数据库路径为：

```text
<用户配置目录>/chatdock/chatdock.sqlite
```

也可以用 `CHATDOCK_DATA` 指定数据目录。旧版 JSON 数据首次启动时会自动迁移进 SQLite：`config.json`、`mcp.json`、`scheduled_tasks.json` 和 `sessions/*.json` 都会导入数据库。旧 JSON 文件不会被自动删除，可作为迁移备份。

MCP 配置、自动化任务和会话都按工作空间隔离保存到 SQLite。Schema 变更通过 `schema_migrations` 记录版本，旧库升级时会自动补齐字段。

定时任务当前支持：

- `once`：一次性任务，到点运行后自动禁用。
- `daily`：每日固定 `HH:MM` 运行。
- `interval`：按分钟间隔循环运行。

服务启动后内置调度器每 30 秒扫描一次所有提示词空间的到期任务。每个任务第一次运行时会创建一个“定时任务：<标题>”专属会话，之后运行结果持续追加到该会话。前端也提供“立即运行”按钮，便于手动验证任务提示词。

## MCP 配置示例

```json
{
  "servers": {
    "agentdock": {
      "url": "http://127.0.0.1:18766/mcp",
      "auth": {
        "type": "bearer",
        "token_env": "AGENTDOCK_TOKEN"
      },
      "allow_tools": ["recall_*", "skill_*", "workflow_template_manage", "task_manage"],
      "deny_tools": ["private_note_manage"],
      "confirm_tools": ["exec_command", "file_edit", "git_write"],
      "timeout_ms": 90000,
      "cache_ttl_ms": 30000
    }
  }
}
```

规则说明：

- `allow_tools` 为空时默认允许该 server 暴露的工具。
- `deny_tools` 优先级高于 `allow_tools`。
- `confirm_tools` 当前会阻止模型自动调用，并返回“需要人工确认”。
- `disabled: true` 可临时禁用某个 MCP server。
- 工具名匹配支持精确匹配、前缀通配 `xxx*`、后缀通配 `*xxx` 和 `*`。

## Docker Compose

Dockerfile 是自包含多阶段构建：先用 Node 构建 Vite/React 前端，再用 Go 编译嵌入前端资源的二进制，最终镜像只包含运行时二进制和数据目录。

源码目录只保留开发示例 compose：

```bash
make dev-up
curl http://127.0.0.1:8720/api/health
```

Mac mini 生产部署必须使用专用目录，防止把 `/data` 挂到源码仓库导致“数据像丢失”：

```bash
make prod-check
make deploy-prod
```

生产期望：

```text
compose: /Volumes/KIOXIA/Docker/chatdock/compose.yaml
/data:   /Volumes/KIOXIA/Docker/chatdock/data
```

如果需要手动使用 Docker Compose，请显式指定开发示例：`docker compose -f compose.dev.yaml up -d --build`。

Compose 不需要再挂载或配置 `CHATDOCK_WEB`，前端页面、后端 API 和 MCP 相关能力都运行在 `8720` 同一个端口。默认还会把 `./backups` 只读挂载到容器 `/backups`，用于配置中心展示最近数据库备份状态。

### 生产部署说明

仓库 `compose.dev.yaml` 是本地开发示例，默认把当前目录的 `./data` 挂载到容器 `/data`。当前 Mac mini 生产实例使用外置盘数据目录 `/Volumes/KIOXIA/Docker/chatdock/data`，并把 `/Volumes/KIOXIA/Docker/chatdock/backups` 只读挂载到容器 `/backups` 展示数据库备份状态。生产更新统一使用 `make deploy-prod`，它会从 `/Volumes/KIOXIA/Docker/chatdock/compose.yaml` 构建并在启动后检查容器 label 和 `/data` mount。

## macOS launchd

先构建：

```bash
make build
```

复制并按实际路径修改。默认使用二进制内嵌的 `web/dist`，不需要配置 `CHATDOCK_WEB`；只有调试磁盘静态目录时才额外设置 `CHATDOCK_WEB=/path/to/web/dist`。

```bash
cp deploy/com.uvwt.chatdock.plist.example ~/Library/LaunchAgents/com.uvwt.chatdock.plist
launchctl load ~/Library/LaunchAgents/com.uvwt.chatdock.plist
```

## 安全边界

ChatDock 默认按“本机私用工具”设计，不建议公网裸奔。API Key、MCP token、会话内容和系统提示词都会落在本地 SQLite 数据库中，应避免把数据目录提交到 Git 或暴露给非可信用户。

如果设置了 `CHATDOCK_AUTH_TOKEN`，除静态页面资源外的 API 都需要：

```text
Authorization: Bearer <token>
```

生产环境建议同时设置 `CHATDOCK_AUTH_TOKEN`、`CHATDOCK_AUTH_USERNAME` 和 `CHATDOCK_AUTH_CREDENTIAL`。前端会显示独立登录页，登录成功后只把后端返回的 Bearer token 保存到浏览器 localStorage；后端不再兼容 URL query token。

## 后续可继续增强

- MCP 人工确认队列和前端确认弹窗。
- Token 估算与上下文摘要压缩。
- 会话收藏、置顶和批量导出。
- 可选敏感配置加密。
