# Easy-RSS

一个基于 Golang 的自托管 RSS 阅读系统，支持**多级 AI 内容处理流水线**，自动过滤垃圾内容、评分、摘要和中文翻译。

## ✨ 特性

- 📡 **RSS/Atom 订阅**：定时抓取，支持并发拉取
- 📂 **OPML 导入**：启动时或通过 API 导入 OPML 文件
- 🤖 **多级 AI 处理流水线**
  - **Stage 1 — 初筛**：自动识别广告、垃圾、无意义内容
  - **Stage 2 — 精加工**：质量评分、生成摘要、翻译为中文
  - 可扩展：实现 `Stage` 接口即可添加新处理级别
- 📊 **RSS 源质量评估**：基于通过率、平均质量分、更新频率、抓取成功率综合评分
- 📥 **外部导入接口**：支持从微信公众号等渠道批量导入文章
- 🔐 **Admin 管理后台**：JWT 认证，单 Admin 账号
- 📦 **单二进制部署**：编译即用，无额外依赖

## 🛠 技术栈

| 组件 | 技术 |
|------|------|
| 语言 | Go 1.22+ |
| Web 框架 | Gin |
| ORM | GORM |
| 数据库 | PostgreSQL 15+ |
| 定时任务 | robfig/cron |
| RSS 解析 | gofeed |
| 配置 | Viper |
| 日志 | Zap |
| AI | OpenAI-compatible API |
| 认证 | JWT (golang-jwt) |

## 🚀 快速开始

### 1. 准备数据库

```bash
createdb easyrss
```

### 2. 配置

编辑 `configs/config.yaml`：

```yaml
database:
  host: 127.0.0.1
  port: 5432
  user: postgres
  password: postgres
  dbname: easyrss

ai:
  enabled: true
  api_key: "your-api-key-here"
  base_url: "https://api.openai.com/v1"  # 或 DeepSeek 等兼容接口
  model: "gpt-4o-mini"

auth:
  admin_username: admin
  admin_password: admin123    # 首次启动自动创建，建议之后修改
  jwt_secret: change-me-in-production
```

### 3. 编译 & 运行

```bash
# 编译
make build
# 或
CGO_ENABLED=0 go build -o bin/easyrss ./cmd/server

# 运行
./bin/easyrss --config configs/config.yaml

# 带 OPML 文件初始化
./bin/easyrss --config configs/config.yaml --opml subscriptions.opml
```

启动后服务监听 `http://localhost:8080`。

## 📡 API

所有接口以 `/api/v1` 为前缀，除登录外均需 JWT 认证。

### 认证

```bash
# 登录获取 Token
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'

# 后续请求携带 Token
curl -H "Authorization: Bearer <token>" http://localhost:8080/api/v1/feeds
```

### 订阅源管理

| Method | Path | 说明 |
|--------|------|------|
| GET | `/api/v1/feeds` | 订阅源列表 |
| POST | `/api/v1/feeds` | 添加订阅源 |
| GET | `/api/v1/feeds/:id` | 订阅源详情 |
| PUT | `/api/v1/feeds/:id` | 更新订阅源 |
| DELETE | `/api/v1/feeds/:id` | 删除订阅源 |
| POST | `/api/v1/feeds/import/opml` | 上传 OPML 导入 |
| POST | `/api/v1/feeds/:id/fetch` | 手动触发抓取 |
| GET | `/api/v1/feeds/quality` | 质量排行 |

### 文章管理

| Method | Path | 说明 |
|--------|------|------|
| GET | `/api/v1/articles` | 文章列表（支持 `feed_id`、`ai_status` 筛选） |
| GET | `/api/v1/articles/:id` | 文章详情（含 AI 结果） |
| DELETE | `/api/v1/articles/:id` | 删除文章 |
| POST | `/api/v1/articles/:id/reprocess` | 重新 AI 处理 |

### 外部导入

```bash
curl -X POST http://localhost:8080/api/v1/import/articles \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "articles": [
      {
        "title": "文章标题",
        "link": "https://example.com/post",
        "content": "文章内容...",
        "author": "作者",
        "source": "wechat"
      }
    ]
  }'
```

## 🤖 AI 处理流水线

文章通过 Pipeline 顺序处理，每个 Stage 可决定是否继续：

```
文章入库 (pending) → Stage 1: 初筛 → Stage 2: 评分/摘要/翻译 → enriched
                         ↓
                   filtered_out (广告/垃圾)
```

### 文章 AI 状态

| 状态值 | 含义 |
|--------|------|
| 0 | pending — 等待处理 |
| 1 | filtered_out — 被过滤（广告/无意义） |
| 2 | passed — 通过初筛 |
| 3 | enriched — 已完成全部 AI 处理 |

### 扩展自定义 Stage

实现 `Stage` 接口并在 `main.go` 中注册：

```go
type Stage interface {
    Name() string
    Process(ctx context.Context, article *model.Article) (proceed bool, err error)
}

// 注册
pipeline.RegisterStage(myCustomStage)
```

## 📊 源质量评估

每日自动评估所有订阅源质量（0~100 分）：

| 维度 | 权重 |
|------|------|
| 文章通过率 | 40% |
| 平均质量分 | 30% |
| 更新频率 | 15% |
| 抓取成功率 | 15% |

低于阈值（默认 20 分）的源将被自动禁用。

## ⏰ 定时任务

| 任务 | 默认间隔 |
|------|----------|
| RSS 抓取 | 30 分钟 |
| AI 处理 | 5 分钟 |
| 质量评估 | 24 小时 |

## 📁 项目结构

```
easy-rss/
├── cmd/server/main.go            # 入口
├── internal/
│   ├── config/                   # 配置
│   ├── model/                    # 数据模型
│   ├── repository/               # 数据库操作
│   ├── service/                  # 业务逻辑
│   │   └── ai_pipeline/          # AI 流水线
│   ├── handler/                  # HTTP 处理器
│   ├── middleware/               # JWT 中间件
│   └── scheduler/                # 定时任务
├── pkg/
│   ├── opml/                     # OPML 解析
│   └── aiutil/                   # AI API 客户端
├── configs/config.yaml           # 配置文件
├── docs/architecture.md          # 架构文档
└── Makefile
```

## 📄 License

MIT
