# Easy-RSS 架构设计文档

## 1. 项目概述

Easy-RSS 是一个基于 Golang 的自托管 RSS 阅读系统，支持：

- RSS/Atom 订阅源管理与定时抓取
- OPML 文件导入初始化
- **多级 AI 内容处理流水线**（广告过滤 → 质量评分 → 摘要/翻译）
- RSS 源质量评估
- 外部文章导入接口
- 单 Admin 管理后台
- 单二进制部署

---

## 2. 技术选型

| 组件     | 技术                                           |
| -------- | ---------------------------------------------- |
| 语言     | Go 1.22+                                       |
| Web 框架 | [Gin](https://github.com/gin-gonic/gin)        |
| ORM      | [GORM](https://gorm.io)                        |
| 数据库   | PostgreSQL 15+                                  |
| 定时任务 | [robfig/cron](https://github.com/robfig/cron)  |
| RSS 解析 | [gofeed](https://github.com/mmcdole/gofeed)    |
| 配置管理 | [Viper](https://github.com/spf13/viper)        |
| 日志     | [Zap](https://github.com/uber-go/zap)          |
| AI 接口  | OpenAI-compatible API（支持 OpenAI / DeepSeek 等）|
| 认证     | JWT                                             |
| 迁移     | GORM AutoMigrate + 自定义 seed                  |

---

## 3. 项目结构

```
easy-rss/
├── cmd/
│   └── server/
│       └── main.go              # 入口，初始化所有组件
├── internal/
│   ├── config/
│   │   └── config.go            # 配置定义与加载
│   ├── model/
│   │   ├── feed.go              # Feed 订阅源模型
│   │   ├── article.go           # Article 文章模型
│   │   ├── ai_result.go         # AI 处理结果模型
│   │   └── user.go              # Admin 用户模型
│   ├── repository/
│   │   ├── feed_repo.go
│   │   ├── article_repo.go
│   │   └── user_repo.go
│   ├── service/
│   │   ├── feed_service.go      # 订阅源 CRUD + OPML 导入
│   │   ├── fetcher_service.go   # RSS 抓取逻辑
│   │   ├── article_service.go   # 文章管理 + 外部导入
│   │   ├── ai_service.go        # AI 处理调度（流水线）
│   │   ├── ai_pipeline/
│   │   │   ├── pipeline.go      # 流水线引擎
│   │   │   ├── stage_filter.go  # Stage 1: 广告/垃圾过滤
│   │   │   └── stage_enrich.go  # Stage 2: 评分/摘要/翻译
│   │   └── auth_service.go      # 认证
│   ├── handler/
│   │   ├── feed_handler.go
│   │   ├── article_handler.go
│   │   ├── import_handler.go    # 外部导入接口
│   │   ├── ai_handler.go        # AI 相关接口
│   │   └── auth_handler.go
│   ├── middleware/
│   │   └── auth.go              # JWT 认证中间件
│   └── scheduler/
│       └── scheduler.go         # 定时任务调度
├── pkg/
│   ├── opml/
│   │   └── parser.go            # OPML 解析
│   └── aiutil/
│       └── client.go            # AI API 客户端封装
├── configs/
│   └── config.yaml              # 默认配置文件
├── docs/
│   └── architecture.md          # 本文档
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## 4. 数据模型

### 4.1 feeds — 订阅源

```sql
CREATE TABLE feeds (
    id            BIGSERIAL PRIMARY KEY,
    title         TEXT NOT NULL,
    url           TEXT NOT NULL UNIQUE,       -- RSS/Atom URL
    site_url      TEXT,                        -- 网站主页
    description   TEXT,
    category      TEXT DEFAULT '',
    etag          TEXT DEFAULT '',             -- HTTP ETag
    last_modified TEXT DEFAULT '',             -- HTTP Last-Modified
    last_fetched  TIMESTAMPTZ,
    fetch_error   TEXT DEFAULT '',
    quality_score FLOAT DEFAULT 0,            -- 源质量评分 (0~100)
    status        SMALLINT DEFAULT 1,         -- 1=active, 0=paused, -1=disabled
    created_at    TIMESTAMPTZ DEFAULT NOW(),
    updated_at    TIMESTAMPTZ DEFAULT NOW()
);
```

### 4.2 articles — 文章

```sql
CREATE TABLE articles (
    id             BIGSERIAL PRIMARY KEY,
    feed_id        BIGINT REFERENCES feeds(id) ON DELETE CASCADE,
    guid           TEXT NOT NULL,              -- 文章唯一标识 (RSS guid)
    title          TEXT NOT NULL,
    link           TEXT NOT NULL,
    author         TEXT DEFAULT '',
    content        TEXT DEFAULT '',            -- 原始内容
    published_at   TIMESTAMPTZ,
    source         TEXT DEFAULT 'rss',         -- rss / import / api
    ai_status      SMALLINT DEFAULT 0,        -- 0=pending, 1=filtered_out, 2=passed, 3=enriched
    created_at     TIMESTAMPTZ DEFAULT NOW(),
    updated_at     TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(feed_id, guid)
);
CREATE INDEX idx_articles_ai_status ON articles(ai_status);
CREATE INDEX idx_articles_feed_id ON articles(feed_id);
```

### 4.3 ai_results — AI 处理结果

```sql
CREATE TABLE ai_results (
    id              BIGSERIAL PRIMARY KEY,
    article_id      BIGINT REFERENCES articles(id) ON DELETE CASCADE UNIQUE,
    is_ad           BOOLEAN DEFAULT FALSE,       -- 是否广告
    is_meaningless  BOOLEAN DEFAULT FALSE,       -- 是否无意义
    filter_reason   TEXT DEFAULT '',              -- 过滤原因
    quality_score   FLOAT DEFAULT 0,             -- 文章质量评分 (0~100)
    summary         TEXT DEFAULT '',              -- AI 摘要
    summary_zh      TEXT DEFAULT '',              -- 中文翻译摘要
    translated_title TEXT DEFAULT '',             -- 中文标题
    tags            TEXT[] DEFAULT '{}',          -- AI 提取的标签
    processed_at    TIMESTAMPTZ DEFAULT NOW()
);
```

### 4.4 users — 管理员

```sql
CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ DEFAULT NOW()
);
```

---

## 5. AI 处理流水线

核心设计采用 **Pipeline 模式**，每篇文章经过多个 Stage 顺序处理，任何 Stage 可以终止流水线。

```
┌──────────┐     ┌──────────────┐     ┌──────────────────┐
│ 文章入库  │────▶│ Stage 1:初筛  │────▶│ Stage 2:评分/翻译 │
│ (pending) │     │ 广告/垃圾过滤  │     │ 质量评分+摘要+翻译 │
└──────────┘     └──────┬───────┘     └────────┬─────────┘
                        │                       │
                   ❌ 过滤丢弃             ✅ 标记为 enriched
                  (filtered_out)
```

### Stage 接口

```go
type Stage interface {
    Name() string
    Process(ctx context.Context, article *model.Article) (proceed bool, err error)
}
```

- `proceed=true`：进入下一 Stage
- `proceed=false`：终止流水线，文章被标记为 `filtered_out`

### Stage 1：初筛（Filter）

调用 AI API，Prompt 包含文章标题和内容摘要，返回 JSON：

```json
{
  "is_ad": false,
  "is_meaningless": false,
  "reason": ""
}
```

若 `is_ad` 或 `is_meaningless` 为 `true`，终止流水线。

### Stage 2：评分 & 摘要 & 翻译（Enrich）

调用 AI API，Prompt 包含文章全文，返回 JSON：

```json
{
  "quality_score": 85.5,
  "summary": "This article discusses...",
  "summary_zh": "本文讨论了...",
  "translated_title": "中文标题",
  "tags": ["golang", "architecture"]
}
```

### 可扩展性

新增 Stage 只需实现 `Stage` 接口并注册到 Pipeline，无需修改已有代码。

---

## 6. RSS 源质量评估

源质量评分基于以下维度（可配置权重）：

| 维度               | 计算方式                                      | 权重 |
| ------------------ | --------------------------------------------- | ---- |
| 文章通过率         | `passed_count / total_count * 100`            | 40%  |
| 平均质量分         | `avg(quality_score)` of passed articles       | 30%  |
| 更新频率           | 根据最近30天发文频率评分                        | 15%  |
| 抓取成功率         | 最近N次抓取的成功比例                          | 15%  |

评分范围 0~100，定期（每日）重新计算并更新 `feeds.quality_score`。

管理员可以根据质量分数：
- 自动暂停低于阈值的源（可配置）
- 在管理后台查看质量排行

---

## 7. API 设计

所有 API 以 `/api/v1` 为前缀，需要 JWT 认证（登录接口除外）。

### 7.1 认证

| Method | Path                | 说明       |
| ------ | ------------------- | ---------- |
| POST   | `/api/v1/auth/login`| 管理员登录 |

### 7.2 订阅源管理

| Method | Path                         | 说明              |
| ------ | ---------------------------- | ----------------- |
| GET    | `/api/v1/feeds`              | 订阅源列表        |
| POST   | `/api/v1/feeds`              | 添加订阅源        |
| PUT    | `/api/v1/feeds/:id`          | 更新订阅源        |
| DELETE | `/api/v1/feeds/:id`          | 删除订阅源        |
| POST   | `/api/v1/feeds/import/opml`  | 导入 OPML 文件    |
| POST   | `/api/v1/feeds/:id/fetch`    | 手动触发抓取      |

### 7.3 文章管理

| Method | Path                              | 说明                 |
| ------ | --------------------------------- | -------------------- |
| GET    | `/api/v1/articles`                | 文章列表（分页/筛选）|
| GET    | `/api/v1/articles/:id`            | 文章详情             |
| DELETE | `/api/v1/articles/:id`            | 删除文章             |
| POST   | `/api/v1/articles/:id/reprocess`  | 重新进行 AI 处理     |

### 7.4 外部导入

| Method | Path                        | 说明                  |
| ------ | --------------------------- | --------------------- |
| POST   | `/api/v1/import/articles`   | 批量导入文章          |

请求体：
```json
{
  "articles": [
    {
      "title": "文章标题",
      "link": "https://example.com/post",
      "content": "文章内容...",
      "author": "作者",
      "source": "wechat"
    }
  ]
}
```

### 7.5 AI & 质量

| Method | Path                        | 说明                  |
| ------ | --------------------------- | --------------------- |
| GET    | `/api/v1/feeds/quality`     | 源质量排行            |
| GET    | `/api/v1/stats`             | 系统统计信息          |

---

## 8. 定时任务

| 任务         | 默认间隔 | 说明                             |
| ------------ | -------- | -------------------------------- |
| RSS 抓取     | 30 min   | 遍历 active feeds，拉取新文章   |
| AI 处理      | 5 min    | 处理 pending 状态的文章          |
| 质量评估     | 24 h     | 重新计算所有 feed 的质量评分     |

---

## 9. 配置文件

```yaml
server:
  port: 8080
  mode: release              # debug / release

database:
  host: 127.0.0.1
  port: 5432
  user: postgres
  password: postgres
  dbname: easyrss
  sslmode: disable

auth:
  admin_username: admin
  admin_password: admin123   # 首次启动自动创建，建议启动后修改
  jwt_secret: change-me-in-production
  jwt_expire_hours: 72

fetcher:
  interval: 30m              # RSS 抓取间隔
  timeout: 30s               # 单次抓取超时
  user_agent: "EasyRSS/1.0"
  max_concurrent: 10         # 最大并发抓取数

ai:
  enabled: true
  provider: openai           # openai / deepseek
  api_key: ""
  base_url: "https://api.openai.com/v1"
  model: "gpt-4o-mini"
  process_interval: 5m       # AI 处理间隔
  max_concurrent: 5          # 最大并发 AI 请求数
  timeout: 60s

quality:
  evaluation_interval: 24h
  auto_disable_threshold: 20  # 低于此分数自动暂停源
  min_articles_for_eval: 10   # 至少N篇文章才参与评估

init:
  opml_file: ""              # 启动时导入的 OPML 文件路径
```

---

## 10. 部署

### 单二进制构建

```bash
# 编译
CGO_ENABLED=0 go build -o easyrss ./cmd/server

# 运行（可带 OPML 初始化）
./easyrss --config config.yaml --opml subscriptions.opml
```

### Makefile 目标

```makefile
build:     编译二进制
run:       本地运行
test:      运行测试
lint:      代码检查
migrate:   数据库迁移（内嵌于启动流程）
```

---

## 11. 启动流程

```
main()
  ├── 加载配置 (Viper)
  ├── 初始化数据库连接 (GORM + PostgreSQL)
  ├── AutoMigrate (建表)
  ├── Seed admin 用户（如不存在）
  ├── 如果指定 --opml，导入 OPML 文件
  ├── 初始化 AI Pipeline (注册 Stages)
  ├── 启动定时任务 (cron)
  │   ├── RSS 抓取任务
  │   ├── AI 处理任务
  │   └── 质量评估任务
  ├── 注册 HTTP 路由 (Gin)
  └── 启动 HTTP Server
```

---

## 12. 后续可扩展方向

- 更多 AI Stage（情感分析、分类、关键词提取等）
- Webhook 通知（新文章推送到 Telegram/Discord/Email）
- 全文搜索（集成 PostgreSQL FTS 或 Meilisearch）
- 多用户支持
- Reader 前端（Web/Mobile）
