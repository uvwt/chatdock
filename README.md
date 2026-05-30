# ChatDock

一个自用的轻量 AI 对话中控台。目标是先做到：提示词可控、上下文可控、模型可控、尽量省 token。

## 当前功能

- Go 后端，标准最小项目结构
- 静态网页前端，零前端构建链
- JSON 文件存储，暂不引入数据库
- OpenAI Chat Completions 兼容接口
- 可配置 Base URL / API Key / Model / System Prompt
- 可配置最近上下文消息数，用来控制 token 消耗
- 会话创建、列表、删除、消息持久化

## 项目结构

```text
chatdock/
├── cmd/chatdock/          # 程序入口
├── internal/chatdock/     # 核心业务代码
│   ├── app.go             # HTTP server 和路由
│   ├── config.go          # 模型配置默认值与校验
│   ├── handler.go         # API handler
│   ├── llm.go             # OpenAI 兼容模型调用
│   ├── store.go           # JSON 文件存储
│   └── types.go           # DTO / Entity
├── web/                   # 静态前端
├── data/                  # 运行时数据，git 忽略
├── Makefile
└── go.mod
```

## 运行

```bash
make run
```

或：

```bash
go run ./cmd/chatdock
```

访问：

```text
http://127.0.0.1:8720
```

## 构建

```bash
make build
./bin/chatdock
```

## 环境变量

```bash
CHATDOCK_ADDR=:8720
CHATDOCK_DATA=data
CHATDOCK_WEB=web
```

## 下一步

- 上下文摘要压缩
- Token 统计
- MCP Server 配置
- 工具白名单
- 定时任务
- Gotify / Pusher 通知
