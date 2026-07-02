# AiPlatform

AiPlatform 是一个基于 Go + Vue 的 AI 应用平台。后端使用 Gin 提供 API，前端使用 Vue 3 和 Element Plus 构建交互界面，当前包含用户认证、AI 对话、多模型切换、RAG 文件问答、图片识别、TTS 文字转语音等能力。

## 功能模块

- 用户模块：支持邮箱验证码注册、用户名或邮箱登录、JWT 鉴权。
- AI 对话：支持新建会话、历史会话、普通响应和 SSE 流式响应。
- 多模型接入：支持普通大模型、RAG 模型、MCP/工具调用模型、Ollama 模型等类型。
- RAG 文件问答：用户上传文件后切片、生成 embedding、写入 Redis 向量索引，聊天时检索相关 chunk 并拼接到 prompt。
- TTS 文字转语音：接入百度语音服务，创建异步语音合成任务并查询结果。
- 图片识别：基于本地 ONNX 模型做图片分类识别。
- 前端页面：包含登录、注册、聊天、文件上传、图片识别等页面。

## 技术栈

后端：

- Go
- Gin
- GORM
- MySQL
- Redis / RediSearch
- RabbitMQ
- JWT

前端：

- Vue 3
- Vue Router
- Element Plus
- Axios

## 项目结构

```text
.
├── common/          # MySQL、Redis、RabbitMQ、RAG、TTS、AI 模型等公共能力
├── config/          # TOML 配置加载
├── controller/      # HTTP 控制器
├── dao/             # 数据访问层
├── middleware/      # JWT 等中间件
├── model/           # GORM 数据模型
├── router/          # 路由注册
├── service/         # 业务逻辑
├── utils/           # 工具函数
├── vue-frontend/    # Vue 前端项目
├── go.mod
└── main.go
```

## 环境依赖

启动后端前需要准备：

- Go 1.20 或以上版本
- MySQL
- Redis Stack / RediSearch
- RabbitMQ
- 可用的大模型 API Key
- 百度语音服务 API Key 和 Secret Key，使用 TTS 时需要

前端建议使用 Node.js LTS 版本，例如 Node.js 20 或 22。

## 配置说明

主要配置文件：

```text
config/config.toml
```

需要根据本地环境修改：

- `mysqlConfig`：MySQL 地址、账号、密码、数据库名。
- `redisConfig`：Redis 地址、密码和 DB。
- `rabbitmqConfig`：RabbitMQ 地址、账号、密码和 vhost。
- `jwtConfig`：JWT 签发信息和密钥。
- `ragModelConfig`：RAG 使用的 embedding 模型、聊天模型、向量维度、chunk 大小、TopK 等。
- `voiceServiceConfig`：百度 TTS 的 API Key 和 Secret Key。

注意：实际部署时不要把真实密码、API Key、邮箱授权码提交到公开仓库，建议改成环境变量或独立的本地配置文件。

## 后端启动

先确保 MySQL、Redis、RabbitMQ 已启动，并且 `config/config.toml` 配置正确。

```bash
go mod tidy
go run .
```

默认监听地址来自配置：

```toml
[mainConfig]
host = "0.0.0.0"
port = 9090
```

服务启动后接口前缀为：

```text
/api/v1
```

## 前端启动

进入前端目录：

```bash
cd vue-frontend
npm install
npm run serve
```

打包：

```bash
npm run build
```

## 主要接口

用户相关：

- `POST /api/v1/user/captcha`：发送邮箱验证码
- `POST /api/v1/user/register`：注册
- `POST /api/v1/user/login`：登录，账号可以是用户名或邮箱

AI 聊天相关，需要 JWT：

- `GET /api/v1/AI/chat/sessions`：获取用户会话列表
- `POST /api/v1/AI/chat/send-new-session`：新建会话并发送消息
- `POST /api/v1/AI/chat/send`：向已有会话发送消息
- `POST /api/v1/AI/chat/history`：获取会话历史
- `POST /api/v1/AI/chat/send-stream-new-session`：新建会话并流式回复
- `POST /api/v1/AI/chat/send-stream`：已有会话流式回复

RAG 文件：

- `POST /api/v1/file/upload`：上传用户知识库文件

TTS：

- `POST /api/v1/AI/chat/tts`：创建文字转语音任务
- `GET /api/v1/AI/chat/tts/query`：查询文字转语音任务

图片识别：

- `POST /api/v1/image/recognize`：上传图片并返回识别结果

## RAG 流程

用户上传文件后，后端会把文件保存到用户目录，读取文本内容并切分为多个 chunk。每个 chunk 会生成 embedding 向量，然后写入 Redis 向量索引。

用户使用 RAG 模型聊天时，系统会根据用户问题检索 TopK 个相关 chunk，把检索结果拼接成 RAG prompt，再交给大模型生成回答。如果用户问题看起来是追问，系统会先结合最近历史对话重写检索 query，提升后续问题的召回效果。

当前 RAG 切片逻辑做了这些优化：

- 优先按段落切分，Markdown 标题会作为段落边界。
- 单段过长时按固定窗口切分。
- chunk overlap 优先从句子边界开始，避免重叠内容从句子中间截断。
- chunk metadata 会保留 `source`、`section`、`chunk_index`、`chunk_total`，检索结果会带入最终 prompt。

相关参数可以在 `ragModelConfig` 中配置：

```toml
chunkSize = 800
chunkOverlap = 150
topK = 5
```

## 当前实现边界

- 用户会话列表从 MySQL 读取，具体会话消息在用户点击后再懒加载到内存。
- 聊天消息先进入 `AIHelper.messages` 内存上下文，再通过 RabbitMQ 异步写入 MySQL。
- 每轮对话会检查请求中的模型类型，如果当前会话模型不同，会切换对应的 `AIModel`。
- 当前 RAG 仍是轻量文件问答模式，Redis 是向量检索层，MySQL 暂未保存文件和 chunk 元数据。
- 如果要演进成生产级 RAG，建议增加知识库表、文件表、chunk 表、索引状态、失败重试和索引重建能力。

## 后续优化方向

- RAG 知识库管理：支持一个用户多个知识库、一个知识库多个文件。
- RAG 元数据持久化：MySQL 保存文件、chunk、索引状态，Redis 只作为检索加速层。
- 索引生命周期：支持 pending、indexing、ready、failed 状态和失败重试。
- 可观测性：补充模型耗时、RAG 检索耗时、Redis 耗时、SSE 写入耗时等日志和指标。
- TTS 任务表：保存 task_id、request_hash、状态、错误信息和结果地址，支持幂等和历史查询。
- MCP/工具调用：补充工具权限控制、超时、fallback、严格协议解析。

## License

本项目使用 MIT License，详见仓库内的 `LICENSE`。
