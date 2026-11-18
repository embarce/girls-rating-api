# CLAUDE.md

本文件为 Claude Code / AI 助手在本仓库中工作时提供指引。

## 项目定位

Girls Rating API —— 图片资源服务后端（Go + Gin + GORM + MySQL + Redis + Cloudflare R2）。核心能力只有三块：

1. `GET /api/random`：从 Redis 随机池中随机抽取图片（高并发读，避免 `ORDER BY RAND()`）
2. `GET /api/cosplay`：图片资源分页列表（Redis 分页缓存）
3. `POST /api/v1/upload/image`：上传图片到 R2 并落库（维护脚本使用，API Key 认证）

另有用户注册/登录/JWT 认证的骨架代码（`users` 表），是早期脚手架产物，当前业务主链路不依赖它。

## 常用命令

```bash
make run                    # go run cmd/main.go
make build                  # go build -o bin/app cmd/main.go
go build ./...              # 全量编译校验（改动后必跑）
go vet ./...                # 静态检查
go test -v ./...            # 目前没有任何 *_test.go，此命令仅验证编译
make swagger                # swag init -g cmd/main.go，改接口后必跑
make clean                  # rm -rf bin/
```

Docker：

```bash
docker build -t girls-rating-api:latest .
docker-compose up -d        # 仅启动 api 容器（9090 -> 容器内 8080），不含 MySQL/Redis
```

`make migrate`、`make mocks` 指向的 `cmd/migrate/main.go`、mockgen 生成目标并不存在，属于失效目标，不要依赖。

## 架构

手工依赖注入 + 四层分层，装配全部集中在 `cmd/main.go`（没有 DI 框架、没有 Wire）：

```
cmd/main.go                      读取配置 → 连 MySQL/Redis → 构造 repo → 构造 service → NewServer → Run
  ↓
api/handlers                      HTTP 层：参数解析、DTO 组装、错误码映射；Server 结构体持有所有 service
  ↓
internal/service                  业务逻辑：缓存读写、Redis 随机池、R2 上传、JWT 签发
  ↓
internal/repository               数据访问：GORM / Raw SQL
  ↓
internal/models                   GORM 实体（无自动迁移，表结构靠手写 SQL）
```

基础设施包：

| 包 | 职责 |
|---|---|
| `internal/config` | Viper 读 `.env` + `AutomaticEnv`，全部默认值硬编码在 `Load()` |
| `internal/database` | GORM 连接与连接池（MaxIdle 10 / MaxOpen 100） |
| `internal/middleware` | `Auth`（JWT Bearer）、`APIKeyAuth`（`X-API-Key`） |
| `pkg/jwt` | HS256 token 签发/解析，`Claims` 含 `user_id`、`username` |
| `pkg/redis` | `redis.Client` 简单包装 + 未被使用的 JSON `Cache` |
| `pkg/r2` | AWS SDK v2 S3 客户端，`BaseEndpoint` 指向 `<account>.r2.cloudflarestorage.com`，region 固定 `auto` |
| `pkg/response` | 统一 `{msg, code, data}` 响应 |
| `pkg/snowflake` | 图片资源主键生成（workerID 固定 0，单机可用） |

## 路由与认证

| Method | Path | 认证 |
|---|---|---|
| GET | `/health` | 无 |
| GET | `/api/random` | 无 |
| GET | `/api/cosplay` | 无 |
| POST | `/api/v1/register` | 无 |
| POST | `/api/v1/login` | 无 |
| GET | `/api/v1/user` | JWT（`Authorization: Bearer <token>`） |
| POST | `/api/v1/upload/image` | 固定 API Key（`X-API-Key`），未配置 key 时直接 403 |
| GET | `/swagger/*any` | 无 |

路由全部写在 `api/handlers/server.go` 的 `setupRouter()`。新增接口必须同时改这里，并同步更新 `docs/API.md`（手写的完整接口文档：参数表、响应示例、错误码、缓存行为）。`docs/` 下的 `docs.go`/`swagger.json`/`swagger.yaml` 是 `make swagger` 生成产物，不要手改。架构/方案类文档放 `docs/design/`（现有：`user-system.md` 用户体系 + Turnstile 人机验证、`task-system.md` 异步 AI 任务体系）。

## 统一响应格式

```json
{ "msg": "success", "code": 200, "data": {} }
```

- `code` 同时作为 HTTP 状态码和响应体字段，两者始终一致。
- 成功统一走 `response.Success(c, status, data)`，失败统一走 `response.Error(c, status, msg)`，`data` 固定为 `{}`，不要直接 `c.JSON`。
- 业务错误用 `service.ErrXxx` 哨兵值，handler 用 `errors.Is/==` 映射到状态码（如 `ErrUserExists` → 409，`ErrUserNotFound` → 401/404）。

## 关键实现机制（改代码前必读）

### 1. Redis 随机池

`internal/service/random.go`，是 `/api/random` 的性能核心：

- Key：`RANDOM_POOL_SET_KEY`（默认 `cache:random_pool:v1:set`），Redis **Set**，成员是 `models.ImageResourceRow` 的 JSON 字符串。
- 读取：`SRandMemberN(key, limit)`，池空时同步刷新一次再取。
- 刷新：`SETNX <key>:lock` 抢锁（TTL `RANDOM_POOL_LOCK_SECONDS`）→ 查库 Top N（`ORDER BY create_time DESC`）→ 写 `<key>:tmp:<nano>` → `RENAME` 覆盖旧池 → 设置 `2 * refreshInterval` 过期。用 tmp + RENAME 是为了刷新期间不出现空池。
- 后台：`NewRandomService` 启动时同步刷一次，之后 `refreshLoop` 按 `RANDOM_POOL_REFRESH_SECONDS` 定时刷（多实例靠锁互斥）。
- **降级**：Redis 不可用或取数失败时回退到 `GetRandomResources` 的 `ORDER BY RAND()`，不返回 500。改这部分时务必保留降级路径。

### 2. 分页缓存

`internal/service/image_resource.go`：

- Key 格式：`cache:image_resource:list:v1:page:{page}:pageSize:{pageSize}`
- TTL：`PAGE_CACHE_TTL_SECONDS`（默认 300s，≤0 则不缓存）
- 缓存值直接是 `repository.ListResult[models.ImageResource]` 的 JSON（含 `total`）
- Redis 读写失败一律忽略，不影响主流程
- **失效**：`internal/service/upload.go` 上传成功后用 `SCAN cache:image_resource:list:v1:page:*` 批量删 key，同时把新图片 `SAdd` 进随机池。改 key 格式时必须同步改这里的 pattern。

### 3. S3 图片地址拼接

- 数据库 `resource_url` 存的是 **R2 对象 key 相对路径**（如 `images/2026/01/01/123456.jpg`），不是完整 URL。
- 出参前用 `S3_HOST` 拼成完整地址（`random.go` 的 `buildS3URL`、`image_resource.go` 的 `BuildS3URL`，两处逻辑重复但都在用）。`S3_HOST` 为空时原样返回相对路径。
- 上传时由 `UploadService` 生成 key：`images/{yyyy/mm/dd}/{rand6}{ext}`。

### 4. 上传流程

`POST /api/v1/upload/image`（multipart）：

1. handler 校验 MIME（只允许 jpeg/png/webp/gif，`Content-Type` 缺失时按扩展名推断）
2. service `io.ReadAll` 全量读入内存 → 未传 width/height 时用 `image.DecodeConfig` 探测
3. 传 R2 拿回对象 key
4. 默认值：`rating=4.0`、`views="1k"`、`author_id=1`、`create_by="Embrace"`
5. 主键用 `snowflake.Generate()`，落库后清分页缓存 + 加随机池

注意：`author` 信息目前是硬编码（`Name: "Embrace"` + 固定头像 URL），`author_id` 在响应里不返回，属于演示态实现。

## 数据库

- **不在启动时自动迁移**：`cmd/main.go` 已明确注释掉迁移逻辑；`internal/database.AutoMigrate` 是残留的死代码，不要接回来。
- 表结构全部手动维护 `migrations/*.sql`。
- 现有表：
  - `users`（见 `migrations/001_init.sql`）：`id/created_at/updated_at/deleted_at/username/email/password/nickname/avatar/status`，`username`、`email` 唯一索引
  - `tb_girls_rating_image_resource`（**仅有 model 定义，仓库里没有对应迁移脚本**）：`id BIGINT / create_by / create_time / resource_url / width / height / author_id / rating DECIMAL(10,1) / views VARCHAR(255)`
- 改表时补 `migrations/00X_xxx.sql`，并同步更新 `internal/models` 中的 gorm tag（含 `comment`）。
- 随机/分页的 Raw SQL 用 `COALESCE` + `CAST(rating AS UNSIGNED)` 保证 JSON 字段类型稳定（空值和 decimal 不会让前端拿到 `null`）。

## 配置

`.env`（Viper 读取，也可用同名环境变量覆盖）。`.env` 已在 `.gitignore` 中。

| 变量 | 说明 | 默认 |
|---|---|---|
| `APP_PORT` / `APP_ENV` | 端口 / 环境（production→Release，test→Test，其余 Debug） | 8080 / development |
| `GIN_TRUSTED_PROXIES` | 逗号分隔 IP 或 CIDR；空表示不信任任何代理 | 空 |
| `MYSQL_*` | `HOST/PORT/USER/PASSWORD/DATABASE` | localhost/3306/root/-/girls_rating |
| `REDIS_*` | `HOST/PORT/PASSWORD/DB` | localhost/6379/-/0 |
| `JWT_SECRET` / `JWT_EXPIRE`（小时，默认 24） / `JWT_ISSUER` | JWT 配置 | - |
| `S3_HOST` | 图片公开访问前缀，如 `https://static.girls-rating.com` | 空 |
| `UPLOAD_API_KEY` | 上传接口固定 key；空 = 禁用上传（403） | 空 |
| `R2_ACCOUNT_ID` / `R2_ACCESS_KEY` / `R2_SECRET_KEY` / `R2_BUCKET` | R2 凭证，缺任一项启动即 `log.Fatalf` | - |
| `PAGE_CACHE_TTL_SECONDS` | 分页缓存 TTL | 300 |
| `RANDOM_POOL_SIZE` | 随机池条数 | 20000 |
| `RANDOM_POOL_REFRESH_SECONDS` | 池刷新间隔 | 3600 |
| `RANDOM_POOL_LOCK_SECONDS` | 刷新锁 TTL | 300 |
| `RANDOM_POOL_SET_KEY` | 池 Redis key | `cache:random_pool:v1:set` |

⚠️ `.env.example` 中 `REDIS_REEK`、`JWT_QGYODG` 是拼写错误（应为 `REDIS_HOST`、`JWT_SECRET`），且缺少 `PAGE_CACHE_TTL_SECONDS`、`RANDOM_POOL_*`、`GIN_TRUSTED_PROXIES`。直接 `cp .env.example .env` 会连不上 Redis，需要手工修正。

## Swagger

- 访问 `http://localhost:8080/swagger/index.html`
- `cmd/main.go` 必须空白导入 `_ "girls-rating-api/docs"`，否则 `docs` 包 `init()` 不执行，Swagger UI 报 unable to load API definition。
- 重新生成用默认输出目录（`./docs`，包名 `docs`），不要改成 `-o ./docs/swagger` 之类，否则导入路径对不上。
- handler 注释里的 `{object}` 引用的是本包内定义的响应结构体（如 `CosplayListResponse`、`ErrorResponse`），新增接口时按现有写法补全 `@Summary/@Tags/@Param/@Success/@Router`。

## 编码约定

- 注释、日志、commit 说明用中文；标识符用英文。commit 风格：`feat(scope): 描述` 或 `[feat] 描述`。
- 新增业务能力按 `models → repository → service → handler → server.go 路由 → swag 注释` 顺序落地，不要在 handler 里直连 DB。
- 列表类接口的 DTO 与 model 分离：handler 层定义 `XxxResponse`，不要直接把 GORM 实体返回给前端（除 `/api/v1/user` 这个脚手架接口）。
- 缓存/Redis/R2 相关失败默认「降级而非报错」，除非明确要暴露给调用方。
- 已知残留死代码：`database.AutoMigrate`、`models.BaseModel`、`redis.Cache`/`NewCache`、`response.StatusOK`。清理时确认无引用再删。
- 当前仓库无任何单元测试；改动核心逻辑（随机池刷新、缓存失效、上传默认值）后，至少 `go build ./...` + `go vet ./...` 通过，并本地跑起服务验证 `/api/random`、`/api/cosplay`。
