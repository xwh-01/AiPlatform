# AiPlatform

AiPlatform 是一个基于 Go + Vue 的 AI 应用平台，后端使用 Gin 提供接口，前端使用 Vue 3 和 Element Plus 构建聊天界面。项目目前包含用户登录注册、AI 对话、多模型切换、RAG 文件问答、图片识别、TTS 文字转语音等能力。

## 功能模块

- 用户模块：邮箱验证码注册、账号登录、JWT 鉴权。
- AI 聊天：支持新建会话、历史会话、普通回复和流式回复。
- 多模型接入：支持普通大模型、Ollama、RAG 模型、工具调用模型等类型扩展。
- RAG 知识库：用户上传文件后切分 chunk，写入 Redis 向量索引，聊天时检索相关内容并拼接给大模型回答。
- TTS：接入百度语音服务，创建异步语音合成任务并查询任务结果。
- 图片识别：提供图片相关 AI 能力接口。
- 前端界面：Vue 3 单页应用，包含登录、注册、菜单、聊天、图片识别等页面。

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
- Redis，RAG 向量检索需要 Redis Stack / RediSearch 能力
- RabbitMQ
- 可用的大模型 API Key
- 百度语音服务 API Key 和 Secret Key，使用 TTS 时需要

前端建议使用 Node.js LTS 版本，例如 Node.js 20 或 22。

## 配置说明

主要配置文件在：

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
- `POST /api/v1/user/login`：登录

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

## RAG 流程

用户上传文件后，后端会将文件保存到用户目录，并把文件内容切分为多个 chunk。每个 chunk 会生成 embedding 向量并写入 Redis 向量索引。

用户使用 RAG 模型聊天时，系统会根据用户问题检索 TopK 个相似 chunk，把检索结果拼接成 RAG prompt，再交给大模型生成回答。当前 chunk 和检索参数可以在 `ragModelConfig` 中配置：

```toml
chunkSize = 800
chunkOverlap = 150
topK = 5
```

## 开发提示

- 用户会话列表从 MySQL 读取，会话消息在用户点击具体会话时再懒加载到内存。
- 每轮对话会检查请求中的模型类型，如果当前会话模型不同，会切换对应的 AIModel。
- 聊天消息先进入内存上下文，再通过 RabbitMQ 异步写入 MySQL。
- 前端依赖目录 `node_modules/`、构建产物和本地环境文件已通过 `.gitignore` 排除。

## License

本项目使用 MIT License，详见 `LICENSE`。
