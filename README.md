# ChatDock

ChatDock 是一个面向个人使用的自托管 AI 工作台。它把多模型对话、项目上下文、MCP 工具、附件、定时任务和执行过程放在同一个 Web 界面中，并将数据保存在你自己的 SQLite 数据库里。

ChatDock 支持 OpenAI Chat Completions 兼容接口，可连接 OpenAI、兼容网关或本地模型服务。它适合希望统一管理模型、提示词、工具和自动化，同时保留完整数据控制权的用户。

## 主要能力

- **多模型与多供应商**：管理多个 OpenAI 兼容供应商、API Key 和候选模型；每个会话可以单独切换模型。
- **主模型与备用模型**：主模型在尚未输出内容、也未执行工具前失败时，可自动切换到备用模型。
- **项目上下文**：为不同项目保存长期提示词，并预览全局提示词与项目提示词组合后的实际内容。
- **流式对话与后台生成**：切换会话后原任务可以继续运行；重新打开会话时可恢复事件流，也可以中断或追加引导。
- **完整会话管理**：搜索、置顶、重命名、复制、从指定消息创建分支、编辑并重新生成、Markdown 导出。
- **附件与图片**：上传、下载并在会话中引用附件；支持向具备视觉能力的模型发送图片。
- **MCP 工具**：连接 HTTP MCP Server，发现和调用工具，并配置允许、拒绝和人工确认规则。
- **工具调用确认**：命中 `confirm_tools` 的调用会暂停，等待用户在界面中允许或拒绝。
- **定时任务**：支持一次性、固定间隔和五段 Cron；可以选择无状态、携带上次结果或连续会话上下文。
- **AgentDock 任务视图**：可选连接 AgentDock Context API，在 ChatDock 中查看会话关联任务及运行进度。
- **本地数据与诊断**：SQLite 单文件存储，系统页展示数据库、WAL、备份和脱敏诊断信息。
- **响应式 Web 与 PWA**：桌面和移动端均可使用，前后端由同一个服务提供。

## 快速开始

推荐使用 Docker。下面的方式使用 Docker Volume 保存数据，不依赖仓库里的开发环境配置。

### 1. 构建镜像

```bash
git clone https://github.com/uvwt/chatdock.git
cd chatdock
docker build -t chatdock:local .
```

### 2. 准备登录配置

先生成一个随机 API Token：

```bash
openssl rand -hex 32
```

创建 `chatdock.env`：

```dotenv
CHATDOCK_AUTH_TOKEN=<粘贴随机 Token>
CHATDOCK_AUTH_USERNAME=admin
CHATDOCK_AUTH_CREDENTIAL=<设置一个高强度登录密码>
CHATDOCK_TIMEZONE=Asia/Shanghai
```

限制配置文件权限：

```bash
chmod 600 chatdock.env
```

`CHATDOCK_AUTH_TOKEN` 用于 API 鉴权；浏览器使用用户名和密码登录后，会在当前浏览器中保存后端返回的访问 Token。

### 3. 启动 ChatDock

```bash
docker volume create chatdock-data

docker run -d \
  --name chatdock \
  --restart unless-stopped \
  --env-file ./chatdock.env \
  -p 127.0.0.1:8720:8720 \
  -v chatdock-data:/data \
  chatdock:local
```

检查登录接口：

```bash
curl http://127.0.0.1:8720/api/auth/status
```

然后打开：

```text
http://127.0.0.1:8720
```

使用 `chatdock.env` 中的账号和密码登录。

## 首次配置模型

登录后进入 **配置中心 → 模型**：

1. 新增模型供应商；
2. 填写名称、OpenAI 兼容 Base URL、API Key 和默认模型；
3. 使用“测试连接”确认接口可用；
4. 选择 ChatDock 的默认供应商和默认模型；
5. 按需设置备用供应商、全局系统提示词、上下文数量和温度。

常见 Base URL 形式：

```text
https://api.openai.com/v1
http://127.0.0.1:11434/v1
https://your-compatible-gateway.example/v1
```

实际可用模型、鉴权方式和参数取决于所连接的服务。ChatDock 使用 OpenAI Chat Completions 兼容协议，不要求供应商必须是 OpenAI。

## 项目与会话

项目用于保存长期上下文，不会复制模型配置：

- 全局系统提示词适用于所有新对话；
- 项目提示词只应用于该项目下的会话；
- 会话可以不属于任何项目；
- 删除项目不会删除会话，原会话会转为普通会话；
- 每个会话可以覆盖默认模型选择。

生成过程中可以切换到其他会话，后端任务会继续运行。再次打开原会话时，ChatDock 会继续读取该任务的事件流。

## MCP 工具

在 **配置中心 → 工具** 中添加 MCP Server。下面是连接 AgentDock 的示例：

```json
{
  "servers": {
    "agentdock": {
      "url": "http://host.docker.internal:18766/mcp",
      "auth": {
        "type": "bearer",
        "token_env": "AGENTDOCK_TOKEN"
      },
      "allow_tools": [
        "recall_*",
        "skill_*",
        "workflow_template_manage",
        "task_manage"
      ],
      "deny_tools": [
        "private_note_manage"
      ],
      "confirm_tools": [
        "exec_command",
        "file_edit",
        "git_write"
      ],
      "timeout_ms": 90000,
      "cache_ttl_ms": 30000
    }
  }
}
```

同时把 MCP Token 加入 `chatdock.env`：

```dotenv
AGENTDOCK_TOKEN=<AgentDock Token>
```

重建容器后环境变量才会生效。

规则优先级：

1. `deny_tools` 始终优先；
2. `allow_tools` 为空时允许该 Server 暴露的全部工具；
3. `confirm_tools` 会在实际调用前暂停，等待浏览器中的人工确认；
4. `disabled: true` 可以临时停用 Server；
5. 工具名支持精确匹配、`prefix*`、`*suffix` 和 `*`。

在 Linux Docker 中访问宿主机服务时，可能还需要在 `docker run` 中加入：

```bash
--add-host=host.docker.internal:host-gateway
```

## 工具搜索与 Embeddings

MCP 工具较多时，ChatDock 可以先搜索候选工具，再选择实际调用目标。未配置向量服务时使用关键词搜索；配置 OpenAI 兼容 Embeddings 后可启用混合搜索。

```dotenv
CHATDOCK_EMBEDDING_BASE_URL=http://embedding-service:8000/v1
CHATDOCK_EMBEDDING_API_KEY=<可选 API Key>
CHATDOCK_EMBEDDING_MODEL=BAAI/bge-m3
```

这些环境变量提供启动配置；也可以在配置中心中维护对应设置。

## 附件与图片

普通附件会保存到 ChatDock 数据目录，并与会话关联。图片要发送给外部视觉模型时，模型服务必须能够访问 ChatDock 生成的签名图片地址。

远程部署可以设置：

```dotenv
CHATDOCK_PUBLIC_BASE_URL=https://chat.example.com
```

该地址必须是外部模型服务可访问的 HTTP 或 HTTPS 地址。仅在本机使用、或模型与 ChatDock 位于同一可信网络时，可以按实际网络结构配置内部地址。

## 定时任务

ChatDock 内置三种调度方式：

- `once`：在指定时间执行一次，完成后自动停用；
- `interval`：按分钟间隔循环执行；
- `cron`：使用一个或多个标准五段 Cron 表达式，并可指定 IANA 时区。

上下文模式：

- **每次独立执行**：每轮不携带历史，最省 Token；
- **带上次运行结果**：只把上一次结果加入上下文；
- **连续会话**：复用关联会话，保留完整上下文。

每次运行都会写入运行记录；也可以在配置中心手动点击“立即运行”进行验证。

## 可选连接 AgentDock 任务

需要在 ChatDock 中查看 AgentDock 任务时，可配置：

```dotenv
CHATDOCK_AGENTDOCK_CONTEXT_URL=http://host.docker.internal:18766/context
CHATDOCK_AGENTDOCK_CONTEXT_TOKEN=<AgentDock Token>
```

连接后，ChatDock 可以显示任务列表、步骤、阻塞状态和当前会话关联任务。实际任务仍由 AgentDock 运行和保存。

## 配置参考

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `CHATDOCK_ADDR` | `:8720` | 服务监听地址 |
| `CHATDOCK_DATA` | 系统用户配置目录 | SQLite、附件和应用数据目录；Docker 镜像默认 `/data` |
| `CHATDOCK_AUTH_TOKEN` | 空 | API Bearer Token |
| `CHATDOCK_AUTH_USERNAME` | 空 | 浏览器登录用户名 |
| `CHATDOCK_AUTH_CREDENTIAL` | 空 | 浏览器登录密码 |
| `CHATDOCK_PUBLIC_BASE_URL` | 空 | 模型读取签名图片时使用的公开地址 |
| `CHATDOCK_TIMEZONE` | 系统时区 | 定时任务默认时区 |
| `CHATDOCK_EMBEDDING_BASE_URL` | 空 | OpenAI 兼容 Embeddings Base URL |
| `CHATDOCK_EMBEDDING_API_KEY` | 空 | Embeddings API Key |
| `CHATDOCK_EMBEDDING_MODEL` | `BAAI/bge-m3` | Embeddings 模型 |
| `CHATDOCK_AGENTDOCK_CONTEXT_URL` | 空 | 可选 AgentDock Context API 地址 |
| `CHATDOCK_AGENTDOCK_CONTEXT_TOKEN` | 空 | AgentDock Context API Token |
| `CHATDOCK_WEB` | 空 | 调试时覆盖内嵌前端资源目录 |

## 数据与备份

ChatDock 默认使用 SQLite：

```text
<CHATDOCK_DATA>/chatdock.sqlite
```

数据目录还可能包含 WAL/SHM、附件和应用运行文件。模型 API Key、MCP Token、会话、提示词和任务配置都属于敏感数据，应作为一个整体保护。

建议：

- 定期使用 SQLite 在线备份，或停止 ChatDock 后再复制完整数据目录；
- 不要只复制 `chatdock.sqlite` 而忽略仍在使用的 `-wal` 文件；
- 不要把数据目录、环境文件或备份提交到 Git；
- 可以把备份目录只读挂载到容器 `/backups`，系统页会显示最近备份状态；
- 升级前先生成可验证的数据库快照。

使用 Docker Volume 时，删除或重建容器不会删除 `chatdock-data`。只有显式删除 Docker Volume 才会移除其中的数据。

## 升级

拉取新代码并重新构建镜像：

```bash
git pull
docker build -t chatdock:local .
```

然后使用与首次启动相同的 `docker run` 参数重建容器，并继续挂载原来的 `chatdock-data`。

ChatDock 不会在运行时自动迁移早期的“多工作空间”数据库。检测到旧表或旧 `workspace_id` 字段时会拒绝启动，避免静默破坏数据。此类旧数据库需要先创建独立快照，再使用：

```bash
go run ./cmd/chatdock-migrate-workspaces \
  -source /path/to/legacy-snapshot.sqlite \
  -target /path/to/current.sqlite \
  -global-workspace default
```

迁移工具不会原地修改源数据库，并会在发布目标文件前执行一致性检查。

## 安全建议

ChatDock 按单管理员、个人使用场景设计，不建议直接暴露在公网。

远程访问时建议：

- 继续让 ChatDock 只监听或映射到 `127.0.0.1`；
- 使用反向代理或 Tunnel 提供 HTTPS；
- 同时配置随机 `CHATDOCK_AUTH_TOKEN`、用户名和高强度密码；
- 限制 `chatdock.env`、数据目录和备份的宿主机权限；
- 不要通过 URL Query 传递 Token；
- 对可能修改文件、运行命令或写入外部系统的 MCP 工具配置 `confirm_tools`。

除登录状态接口和登录接口外，启用 Token 后的 API 请求需要：

```text
Authorization: Bearer <CHATDOCK_AUTH_TOKEN>
```

## 从源码运行

需要 Go、Node.js 和 npm：

```bash
make run
```

访问：

```text
http://127.0.0.1:8720
```

构建单个包含前端资源的 Go 二进制：

```bash
make build
./bin/chatdock
```

提交前执行完整检查：

```bash
make check
```

`make check` 会执行前端依赖与构建、CSS/结构守卫、前端测试、Go 格式、`go vet`、Go 测试和最终二进制构建。

更多实现和开发约束见 [架构文档](./docs/architecture.md)。
