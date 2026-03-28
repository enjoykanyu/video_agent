# 小V视频AI助手 (Video AI Assistant)

**基于 CloudWeGo Eino 框架的多智能体视频AI助手系统**

Go 1.23+ | Eino 0.6.0 | Ollama Qwen3 | Milvus 2.5.10 | MIT License

[项目简介](#-项目简介) • [系统架构](#-系统架构) • [功能特性](#-功能特性) • [快速开始](#-快速开始) • [API文档](#-api文档) • [项目结构](#-项目结构)

***

## 📖 项目简介

小V视频AI助手是一个面向视频创作者和观众的智能助手系统，基于 **CloudWeGo Eino** 框架构建，集成了大语言模型、RAG知识检索、MCP工具调用等先进技术，提供视频分析、内容推荐、知识问答、创作辅助等全方位AI服务。

### 🎯 核心价值

- **🎬 视频智能分析**：深度分析视频内容、评论、弹幕，生成结构化报告
- **🤖 多智能体协同**：基于意图识别的智能路由，10+个专业Agent协同工作
- **📚 知识增强问答**：RAG技术加持的知识库问答，支持实时检索
- **🔧 MCP工具生态**：标准化的工具调用协议，易于扩展
- **⚡ 流式响应**：支持SSE流式输出，提升用户体验

***

## 🏗️ 系统架构

### 整体架构

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              用户交互层                                        │
│         (Web端 gRPC API / SSE流式接口)                     │
└─────────────────────────────────┬───────────────────────────────────────────┘
                                  │
┌─────────────────────────────────▼───────────────────────────────────────────┐
│                         编排层 (Eino Graph Engine)                          │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐  │
│  │  意图识别    │───▶│  Branch路由  │───▶│ Agent执行   │───▶│  结果总结   │  │
│  │  (LLM分类)   │    │ (条件分支)   │    │ (并行处理)   │    │ (响应生成)  │  │
│  └─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘  │
└─────────────────────────────────┬───────────────────────────────────────────┘
                                  │
┌─────────────────────────────────▼───────────────────────────────────────────┐
│                           Agent服务层                                        │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐          │
│  │ 视频总结  │ │ 评论分析  │ │ 视频推荐  │ │ 热门视频  │ │ 热门直播  │          │
│  │ 创作分析  │ │ 周报分析  │ │ RAG问答  │ │ 知识选择  │ │ 闲聊对话  │          │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘ └──────────┘          │
└─────────────────────────────────┬───────────────────────────────────────────┘
                                  │
┌─────────────────────────────────▼───────────────────────────────────────────┐
│                         MCP工具层 (Model Context Protocol)                  │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐          │
│  │ 视频服务  │ │ 向量检索  │ │ 数据存储  │ │ 弹幕分析  │ │ 评论分析  │          │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘ └──────────┘          │
└─────────────────────────────────┬───────────────────────────────────────────┘
                                  │
┌─────────────────────────────────▼───────────────────────────────────────────┐
│                           基础层                                          │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐          │
│  │  Ollama  │ │  Milvus  │ │  MinIO   │ │  etcd    │ │  Redis   │          │
│  │  (LLM)   │ │ (Vector) │ │(Object)  │ │(Config)  │ │ (Cache)  │          │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘ └──────────┘          │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 技术栈

| 组件        | 技术             | 版本      | 用途         |
| --------- | -------------- | ------- | ---------- |
| **编程语言**  | Go             | 1.23.0+ | 后端开发       |
| **AI框架**  | CloudWeGo Eino | v0.6.0  | 智能体编排      |
| **大模型**   | Ollama         | latest  | 本地LLM推理    |
| **模型版本**  | Qwen3          | 0.6b    | 中文对话模型     |
| **向量数据库** | Milvus         | v2.5.10 | 向量存储与检索    |
| **对象存储**  | MinIO          | latest  | 文件存储       |
| **API框架** | Gin            | v1.10.1 | HTTP API服务 |
| **通信协议**  | gRPC           | v1.73.0 | 高性能RPC     |
| **配置中心**  | etcd           | v3.5.18 | 服务配置       |
| **MCP协议** | mcp-go         | v0.43.0 | 工具调用协议     |

***

## ✨ 功能特性

### 1. 智能意图识别

系统通过LLM智能识别用户意图，支持10+种意图类型：

| 意图类型              | 优先级 | 功能描述     |
| ----------------- | --- | -------- |
| `RAG`             | P0  | 知识库问答检索  |
| `VideoSummary`    | P0  | 视频内容总结分析 |
| `CommentAnalysis` | P0  | 评论/弹幕分析  |
| `VideoRecommend`  | P0  | 个性化视频推荐  |
| `UserLikedVideos` | P1  | 用户点赞视频查询 |
| `HotVideo`        | P1  | 热门视频查询   |
| `HotLive`         | P1  | 热门直播查询   |
| `Report`          | P1  | 数据分析周报   |
| `Creative`        | P1  | 创作方向分析   |
| `Chat`            | P2  | 通用闲聊对话   |

### 2. Agent服务矩阵

```
┌─────────────────────────────────────────────────────────────────┐
│                        Agent服务矩阵                             │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  📊 数据分析类              🎬 视频内容类              💬 交互类   │
│  ┌──────────────┐          ┌──────────────┐         ┌──────────┐│
│  │ ReportAgent  │          │VideoSummary  │         │RAGAnswer ││
│  │  (周报分析)   │          │  (视频总结)   │         │ (知识问答)││
│  └──────────────┘          └──────────────┘         └──────────┘│
│  ┌──────────────┐          ┌──────────────┐         ┌──────────┐│
│  │CreativeAgent │          │CommentAnalysis│         │ChatAgent ││
│  │ (创作分析)   │          │ (评论分析)    │         │ (闲聊)   ││
│  └──────────────┘          └──────────────┘         └──────────┘│
│                                                                 │
│  🔥 热门趋势类              🎯 推荐类                           │
│  ┌──────────────┐          ┌──────────────┐                     │
│  │  HotVideo    │          │VideoRecommend│                     │
│  │ (热门视频)   │          │ (视频推荐)   │                     │
│  └──────────────┘          └──────────────┘                     │
│  ┌──────────────┐          ┌──────────────┐                     │
│  │   HotLive    │          │UserLikedVideo│                     │
│  │ (热门直播)   │          │ (点赞查询)   │                     │
│  └──────────────┘          └──────────────┘                     │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 3. RAG知识检索

- **文档向量化**：支持Markdown文档自动分块和向量化
- **相似度检索**：基于余弦相似度的向量检索（阈值0.75）
- **上下文增强**：检索结果注入Prompt，提升回答准确性
- **来源追溯**：答案附带引用来源，确保可信度

### 4. MCP工具生态

基于Model Context Protocol标准，实现可扩展的工具调用：

- **视频服务工具**：获取视频信息、评论、弹幕数据
- **向量检索工具**：Milvus向量数据库操作
- **数据分析工具**：用户行为分析、趋势统计

***

## 🚀 快速开始

### 环境要求

- **Go**: 1.23.0+
- **Docker & Docker Compose**: 用于部署基础设施
- **Ollama**: 本地大模型服务

### 1. 克隆项目

```bash
git clone <repository-url>
cd video_agent
```

### 2. 启动基础设施服务

```bash
# 启动 Milvus + MinIO + etcd + Attu
docker-compose up -d

# 查看服务状态
docker-compose ps
```

服务端口说明：

| 服务            | 端口    | 用途          |
| ------------- | ----- | ----------- |
| Milvus        | 19530 | 向量数据库       |
| Milvus HTTP   | 9091  | Milvus API  |
| MinIO API     | 9000  | 对象存储API     |
| MinIO Console | 9001  | MinIO管理界面   |
| etcd          | 2379  | 配置中心        |
| Attu          | 8000  | Milvus可视化工具 |

### 3. 启动 Ollama 服务

```bash
# 安装 Ollama (macOS)
brew install ollama

# 启动 Ollama 服务
ollama serve

# 拉取 Qwen3 模型 (新终端)
ollama pull qwen3:0.6b
```

### 4. 运行项目

```bash
# 安装依赖
go mod download

# 运行主服务
go run cmd/xiaov_server/main.go
```

服务启动成功后将监听 `:50090` 端口。

***

## 📡 API文档

### gRPC 服务接口

**服务地址**: `localhost:50090`

**Proto文件**: [proto/xiaov.proto](proto/xiaov.proto)

#### 接口列表

| 方法                  | 请求类型                     | 响应类型                      | 描述          |
| ------------------- | ------------------------ | ------------------------- | ----------- |
| `Chat`              | ChatRequest              | ChatResponse              | 普通聊天模式      |
| `ChatStream`        | ChatRequest              | stream ChatStreamResponse | 流式聊天模式(SSE) |
| `GetSessionHistory` | GetSessionHistoryRequest | GetSessionHistoryResponse | 获取会话历史      |
| `ClearSession`      | ClearSessionRequest      | ClearSessionResponse      | 清空会话        |
| `HealthCheck`       | HealthCheckRequest       | HealthCheckResponse       | 健康检查        |

#### 请求/响应格式

**ChatRequest**:

```protobuf
message ChatRequest {
    string user_id = 1;      // 用户ID（必填）
    string message = 2;      // 用户消息（必填）
    string session_id = 3;   // 会话ID（可选，用于保持上下文）
}
```

**ChatResponse**:

```protobuf
message ChatResponse {
    int32 code = 1;                    // 状态码：0-成功
    string message = 2;                // 状态描述
    string reply = 3;                  // 助手回复
    string session_id = 4;             // 会话ID
    string intent = 5;                 // 识别的意图类型
    int64 timestamp = 6;               // 时间戳（毫秒）
    map<string, string> metadata = 7;  // 元数据
}
```

### 调用示例

#### 使用 grpcurl 测试

```bash
# 安装 grpcurl
brew install grpcurl

# 普通聊天
grpcurl -plaintext localhost:50090 xiaovpb.XiaovService/Chat \
  -d '{
    "user_id": "user123",
    "message": "分析一下这个视频的评论区"
  }'

# 流式聊天
grpcurl -plaintext localhost:50090 xiaovpb.XiaovService/ChatStream \
  -d '{
    "user_id": "user123",
    "message": "推荐一些热门视频"
  }'

# 健康检查
grpcurl -plaintext localhost:50090 xiaovpb.XiaovService/HealthCheck
```

#### 使用 Go 客户端

```go
package main

import (
    "context"
    "log"
    "google.golang.org/grpc"
    pb "video_agent/proto_gen/proto"
)

func main() {
    conn, err := grpc.Dial("localhost:50090", grpc.WithInsecure())
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    client := pb.NewXiaovServiceClient(conn)
    
    resp, err := client.Chat(context.Background(), &pb.ChatRequest{
        UserId:  "user123",
        Message: "分析一下视频BV1xx411c7mD",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("Reply: %s, Intent: %s", resp.Reply, resp.Intent)
}
```

***

## 📁 项目结构

```
video_agent/
├── 📁 agent/                    # 早期示例代码
│   ├── first_agent.go
│   ├── graph.go
│   └── graph_with_rag.go
│
├── 📁 api/                      # HTTP API接口层
│   ├── gin_server.go           # Gin HTTP服务器
│   └── rag_api.go              # RAG相关API
│
├── 📁 cmd/                      # 应用程序入口
│   ├── mcp_server/             # MCP服务器入口
│   └── xiaov_server/           # 小V助手gRPC服务器入口
│       └── main.go             # 主程序入口 ⭐
│
├── 📁 data/                     # 数据存储
│   ├── rag_store/              # RAG文档存储
│   └── vector_store/           # 向量存储
│
├── 📁 docs/                     # 项目文档
│   ├── 01_PRODUCT_ARCHITECTURE.md    # 产品架构文档
│   ├── 02_TECHNICAL_ARCHITECTURE.md  # 技术架构文档
│   ├── VIDEO_AI_ASSISTANT_FRAMEWORK.md
│   └── MCP_ENTERPRISE_ARCHITECTURE.md
│
├── 📁 examples/                 # 示例代码
│   ├── branch_routing_example.go
│   ├── mcp_tool_example.go
│   ├── memory_example.go
│   └── rag_example.go
│
├── 📁 internal/                 # 内部实现（核心业务逻辑）⭐
│   ├── agent/                   # Agent系统
│   │   ├── agents/              # 各类Agent实现
│   │   │   ├── base/            # 基础Agent框架
│   │   │   ├── comment_analysis/# 评论分析Agent
│   │   │   ├── creative_analysis/# 创作分析Agent
│   │   │   ├── hot_live/        # 热门直播Agent
│   │   │   ├── hot_video/       # 热门视频Agent
│   │   │   ├── rag_answer/      # RAG问答Agent
│   │   │   ├── rag_selector/    # RAG选择器Agent
│   │   │   ├── report/          # 周报分析Agent
│   │   │   ├── summary/         # 总结Agent
│   │   │   ├── user_liked_videos/# 用户点赞视频Agent
│   │   │   ├── video_recommend/ # 视频推荐Agent
│   │   │   └── video_summary/   # 视频总结Agent
│   │   ├── biz/                 # 业务逻辑层
│   │   │   └── biz.go           # VideoAssistantUsecase ⭐
│   │   ├── graph/               # 图编排引擎
│   │   │   └── graph.go         # VideoGraph 核心编排 ⭐
│   │   ├── mcp/                 # MCP客户端
│   │   ├── prompt/              # Prompt模板
│   │   ├── state/               # 状态管理
│   │   └── types/               # 类型定义
│   ├── handler/                 # 请求处理器
│   ├── mcp/                     # MCP服务实现
│   └── memory/                  # 记忆系统
│
├── 📁 mcp/                      # MCP协议相关
├── 📁 mcp_client/               # MCP客户端
├── 📁 mcp_server/               # MCP服务器
├── 📁 package/                  # 客户端包
├── 📁 proto/                    # Protocol Buffers定义
│   └── xiaov.proto              # gRPC服务定义 ⭐
├── 📁 proto_gen/                # 生成的Protobuf代码
├── 📁 rag/                      # RAG系统
│   ├── rag_manager.go           # RAG管理器 ⭐
│   ├── retriever.go             # 向量检索器
│   └── index.go                 # 索引管理
├── 📁 tool/                     # 工具组件
├── 📁 volumes/                  # Docker数据卷
├── 📄 docker-compose.yml        # Docker Compose配置 ⭐
├── 📄 go.mod                    # Go模块定义
└── 📄 README.md                 # 项目说明
```

### 核心文件说明

| 文件路径                                                           | 说明                  |
| -------------------------------------------------------------- | ------------------- |
| [cmd/xiaov\_server/main.go](cmd/xiaov_server/main.go)          | 主程序入口，初始化并启动gRPC服务器 |
| [internal/agent/graph/graph.go](internal/agent/graph/graph.go) | 图编排引擎核心，定义节点和路由逻辑   |
| [internal/agent/biz/biz.go](internal/agent/biz/biz.go)         | 业务逻辑层，处理用户请求        |
| [proto/xiaov.proto](proto/xiaov.proto)                         | gRPC服务定义文件          |
| [rag/rag\_manager.go](rag/rag_manager.go)                      | RAG系统管理器            |

***

## 🔧 配置说明

### 环境变量

```bash
# Ollama 配置
export OLLAMA_HOST="http://localhost:11434"
export OLLAMA_MODEL="qwen3:0.6b"

# Milvus 配置
export MILVUS_HOST="localhost:19530"

# MCP 服务器配置
export MCP_SERVER_URL="http://localhost:8081/mcp/sse"
```

### 配置文件

系统支持通过 etcd 进行配置管理，配置文件位于 `internal/agent/config/` 目录。

***

## 🧪 测试

### 单元测试

```bash
# 运行所有测试
go test ./...

# 运行特定包测试
go test ./internal/agent/...

# 带覆盖率测试
go test -cover ./...
```

### 集成测试

```bash
# 启动测试环境
docker-compose -f docker-compose.test.yml up -d

# 运行集成测试
go test -tags=integration ./tests/...
```

***

## 📊 性能指标

| 指标        | 目标值      | 说明         |
| --------- | -------- | ---------- |
| 意图识别延迟    | < 500ms  | LLM分类时间    |
| Agent执行延迟 | < 1s     | 工具调用+LLM生成 |
| RAG检索延迟   | < 300ms  | 向量检索时间     |
| 流式首字节延迟   | < 200ms  | 首响应时间      |
| 并发处理能力    | 100+ QPS | 单实例处理能力    |

***

## 🛠️ 开发指南

### 添加新Agent

1. 在 `internal/agent/agents/` 下创建新目录
2. 实现 Agent 接口：

```go
type Agent interface {
    Name() string
    Execute(ctx context.Context, input string, state *types.State) (string, error)
}
```

1. 在 `internal/agent/graph/graph.go` 中注册新节点
2. 在 Branch 路由中添加意图匹配规则

### 添加MCP工具

1. 在 `internal/mcp/tools.go` 中定义工具
2. 实现工具处理逻辑
3. 注册到 MCP Registry

***

## 📚 相关文档

- [产品架构文档](docs/01_PRODUCT_ARCHITECTURE.md) - 详细的产品功能设计
- [技术架构文档](docs/02_TECHNICAL_ARCHITECTURE.md) - 系统技术架构详解
- [Eino框架文档](https://cloudwego.cn/zh/docs/eino/) - CloudWeGo Eino官方文档
- [MCP协议规范](https://modelcontextprotocol.io/) - Model Context Protocol规范

***

## 🤝 贡献指南

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 创建 Pull Request

***

## 📄 许可证

本项目采用 [MIT](LICENSE) 许可证开源。

***

## 🙏 致谢

- [CloudWeGo Eino](https://github.com/cloudwego/eino) - 优秀的Go语言AI应用开发框架
- [Ollama](https://ollama.com/) - 本地大模型服务
- [Milvus](https://milvus.io/) - 开源向量数据库
- [MCP](https://modelcontextprotocol.io/) - Model Context Protocol

***


<p align="center">
  <b>Star ⭐ 这个项目如果它对你有帮助!</b>
</p>
