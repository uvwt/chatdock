# ChatDock

ChatDock 是一个自用的轻量 AI 对话中控台，目标是：提示词可控、上下文可控、模型可控、尽量省 token，并可选接入 MCP 工具。

## 当前功能

- Go 后端，静态网页前端，无前端构建链。
- JSON 文件存储，默认不引入数据库。
- OpenAI Chat Completions 兼容接口。
- 可配置 Base URL / API Key / Model / System Prompt。
- 可配置最近上下文消息数，用来控制 token 消耗。
- 可通过 `chat_template_kwargs.enable_thinking` 控制模型思考开关。
- 可隐藏 `<think>...</think>` 思考内容。
- 提示词空间：每个空间独立保存模型配置、MCP 配置和会话。
- 会话创建、列表、删除、重命名、搜索、Markdown 导出。
- 无 MCP 工具时保留真正流式输出；启用 MCP 工具时通过 SSE 输出工具调用事件和最终回答。
- MCP HTTP JSON-RPC 客户端：支持 `tools/list`、`tools/call`、Bearer token、server 超时、工具列表缓存、工具 allow/deny/confirm 规则。

## 项目结构

```text
chatdock/
├── cmd/chatdock/          # 程序入口
├── internal/chatdock/     # 后端核心代码
├── web/                   # 静态前端：index.html + app.css + app.js + markdown.js + mcp.js
├── deploy/                # launchd 示例
├── data/                  # 运行时数据，git 忽略
├── Dockerfile
├── compose.yaml
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
CHATDOCK_DATA=data
CHATDOCK_WEB=web
```

## 构建与验证

```bash
make fmt
make check
```

`make check` 会执行：`fmt-check`、`go vet ./...`、`go test ./...`、`go build`。

## MCP 配置示例

MCP 配置按提示词空间保存到 `data/prompts/<name>/mcp.json`。示例：

```json
{
  "servers": {
    "agentdock": {
      "url": "http://127.0.0.1:18766/mcp",
      "auth": {
        "type": "bearer",
        "token_env": "AGENTDOCK_TOKEN"
      },
      "allow_tools": ["memory_*", "desktop_screenshot"],
      "deny_tools": ["memory_delete"],
      "confirm_tools": ["desktop_*", "file_write", "git_push"],
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

```bash
docker compose up -d --build
curl http://127.0.0.1:8720/api/health
```

## macOS launchd

先构建：

```bash
make build
```

复制并按实际路径修改：

```bash
cp deploy/com.uvwt.chatdock.plist.example ~/Library/LaunchAgents/com.uvwt.chatdock.plist
launchctl load ~/Library/LaunchAgents/com.uvwt.chatdock.plist
```

## 安全边界

ChatDock 默认按“本机私用工具”设计，不建议公网裸奔。API Key、MCP token、会话内容和系统提示词都会落在本地数据目录中；虽然文件以 `0600` 写入，但仍应避免把数据目录提交到 Git 或暴露给非可信用户。

## 后续可继续增强

- MCP 人工确认队列和前端确认弹窗。
- Token 估算与上下文摘要压缩。
- 会话收藏、置顶、批量导出。
- 可选敏感配置加密。
