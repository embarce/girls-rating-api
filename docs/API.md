# Girls Rating API 接口文档

> 手工维护的 HTTP 接口说明。机器可读版本见同目录 `swagger.json` / `swagger.yaml`（由 `make swagger` 从 handler 注释生成），在线调试见 `/swagger/index.html`。
>
> 改接口时请同步更新本文件（见文末「维护约定」）。

## 目录

- [通用约定](#通用约定)
- [接口一览](#接口一览)
- [1. 健康检查](#1-健康检查)
- [2. 随机图片](#2-随机图片)
- [3. 分页列表](#3-分页列表)
- [4. 入口页最新图片](#4-入口页最新图片)
- [5. 用户注册](#5-用户注册)
- [6. 用户登录](#6-用户登录)
- [7. 当前用户信息](#7-当前用户信息)
- [8. 上传图片](#8-上传图片)
- [9. 抽卡](#9-抽卡)
- [10. 管理端：作者 CRUD](#10-管理端作者-crud)
- [11. 管理端：图片改档](#11-管理端图片改档)
- [12. 邮件反馈](#12-邮件反馈)
- [13. Swagger UI](#13-swagger-ui)
- [实现约束与已知问题](#实现约束与已知问题)
- [维护约定](#维护约定)

## 通用约定

### Base URL

| 环境 | 地址 |
|---|---|
| 本地 | `http://localhost:8080` |
| Docker | `http://localhost:9090`（容器内 8080，见 `docker-compose.yml`） |

所有业务接口无 `/v1` 全局前缀；仅用户相关接口位于 `/api/v1` 下。

### 统一响应格式

所有接口（含错误）都返回同一个信封结构：

```json
{ "msg": "success", "code": 200, "data": {} }
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `msg` | string | 成功固定为 `"success"`；失败为错误描述（内部错误会直接透出 `err.Error()`） |
| `code` | int | 与 HTTP 状态码**恒等** |
| `data` | any | 成功时为业务数据；失败时固定为 `{}` |

### 认证方式

| 方式 | 请求头 | 适用接口 | 说明 |
|---|---|---|---|
| JWT | `Authorization: Bearer <access_token>` | `GET /api/v1/user` | HS256；过期时间 `JWT_EXPIRE` 小时（默认 24） |
| 固定 API Key | `X-API-Key: <UPLOAD_API_KEY>` | `POST /api/v1/upload/image`、`/api/v1/admin/*` | 服务端未配置 `UPLOAD_API_KEY` 时该路由一律 403（等于禁用上传/管理） |
| 匿名设备 | `X-Device-Id: <uuid>` | `/api/gacha/*` | 前端 `crypto.randomUUID()` 生成并 localStorage 持久化；缺失返回 400 |
| Turnstile | `X-Turnstile-Token` | `POST /api/v1/register`、`POST /api/v1/login`、`POST /api/v1/mail/contact` | Cloudflare Turnstile 人机验证；服务端未配置 `TURNSTILE_SECRET_KEY` 时不校验（中间件直接放行），配置后缺 token 或校验失败一律 403。token 由前端 widget 产出，一次性 |

认证失败统一返回 401，`msg` 为 `missing authorization header` / `invalid authorization format` / `invalid or expired token` / `missing X-API-Key header` / `invalid API key`。

### 通用错误码

| code | 含义 |
|---|---|
| 200 | 成功 |
| 201 | 创建成功（注册、上传、新建作者） |
| 400 | 请求参数错误（JSON 绑定失败、文件缺失、文件类型不支持、缺 `X-Device-Id`、rarity 非法） |
| 401 | 未认证 / 认证无效 |
| 403 | 上传 API Key 未配置 / Turnstile 人机验证失败或缺 token |
| 404 | 资源不存在（用户 / 作者 / 图片） |
| 409 | 冲突（用户名或邮箱已存在、作者改名撞唯一索引） |
| 500 | 服务器内部错误或依赖（MySQL / Redis / R2）异常 |

### 其他约定

- 请求/响应体均为 `application/json; charset=utf-8`，上传接口为 `multipart/form-data`。
- 时间字段为 RFC3339 字符串，例如 `"2026-11-17T22:08:37+08:00"`。
- 字段命名：出参为 camelCase（`resourceUrl`、`totalCount`），例外是用户相关接口直接返回 GORM 实体，使用 snake_case（`created_at`、`updated_at`）。

## 接口一览

| # | Method | Path | 认证 | 说明 |
|---|---|---|---|---|
| 1 | GET | `/health` | 无 | 健康检查 |
| 2 | GET | `/api/random` | 无 | 随机返回一批图片（Redis 随机池 v2） |
| 3 | GET | `/api/cosplay` | 无 | 图片资源分页列表（Redis 分页缓存） |
| 4 | GET | `/api/latest` | 无 | 入口页最新 N 张（默认 10，limit 1-20） |
| 5 | POST | `/api/v1/register` | 无 | 用户注册 |
| 6 | POST | `/api/v1/login` | 无 | 用户登录，签发 JWT |
| 7 | GET | `/api/v1/user` | JWT | 获取当前登录用户 |
| 8 | POST | `/api/v1/upload/image` | API Key | 上传图片到 R2 并落库（定档 rarity） |
| 9 | POST | `/api/gacha/draw` | 设备 | 每日抽卡（幂等补发） |
| 10 | GET | `/api/gacha/status` | 设备 | 抽卡状态 + 公示概率 |
| 11 | GET | `/api/gacha/collection` | 设备 | 卡册分页 |
| 12 | GET | `/api/v1/admin/authors` | API Key | 作者列表 / 存在性检查 |
| 13 | POST | `/api/v1/admin/authors` | API Key | 幂等创建作者 |
| 14 | GET/PATCH/DELETE | `/api/v1/admin/authors/:id` | API Key | 作者详情 / 更新 / 软删除 |
| 15 | POST | `/api/v1/admin/authors/:id/avatar` | API Key | 上传作者头像到 R2 |
| 16 | PATCH | `/api/v1/admin/images/:id` | API Key | 图片改档（rating/rarity/views/author_id） |
| 17 | POST | `/api/v1/mail/contact` | Turnstile | 用户反馈邮件（Resend 中继） |
| 18 | GET | `/swagger/*any` | 无 | Swagger UI |

业务主链路是 2、3、4、8、9、10、11；反馈 17 面向浏览器；管理端 12–16 供爬虫自动化使用；5、6、7 是早期脚手架产物，当前前端不依赖。

---

## 1. 健康检查

```
GET /health
```

无参数、无认证。

**响应 200**

```json
{
  "msg": "success",
  "code": 200,
  "data": { "message": "ok" }
}
```

> 只表示进程存活，不校验 MySQL / Redis / R2 连通性。

## 2. 随机图片

```
GET /api/random
```

无参数、无认证。用于首页随机瀑布流。

**响应 200** —— `data` 直接是数组，最多 100 条：

```json
{
  "msg": "success",
  "code": 200,
  "data": [
    {
      "resourceUrl": "https://static.example.com/images/2026/11/17/042913.jpg",
      "width": 1200,
      "height": 1600,
      "rating": 4.7,
      "views": "1k",
      "rarity": "SR",
      "author": {
        "name": "Embrace",
        "avatar": "https://www.girls-rating.com/images/avatars/avatar1.webp"
      }
    }
  ]
}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `resourceUrl` | string | 完整图片地址；`S3_HOST` 未配置时为 R2 相对 key |
| `width` / `height` | int | 像素尺寸，缺失为 0 |
| `rating` | float64 | 评分，恢复浮点（DB `decimal(10,1)` 原值，4.7 → 4.7） |
| `views` | string | 浏览量，形如 `"1k"` |
| `rarity` | string | 卡片等级 `"R"` / `"SR"` / `"SSR"`（上传时定档落库） |
| `author` | object | 来自 `authors` 表（`author_id` 解析，无效回退默认作者 id=1） |

**行为说明**

- 数据来自 Redis Set `cache:random_pool:v2:set`（池成员为 PhotoRow JSON v2，含 rarity/authorId），池内默认 2 万条（`RANDOM_POOL_SIZE`），每小时刷新（`RANDOM_POOL_REFRESH_SECONDS`）。v1 旧池 key 弃用。
- 返回顺序随机，同一请求内可能重复，**不保证**与数据库实时一致（新增图片由上传接口主动入池，最多 1 小时窗口）。
- Redis 异常时降级为 `ORDER BY RAND()` 直查，接口不返回 500。
- 池内无数据时返回 `"data": []`。

## 3. 分页列表

```
GET /api/cosplay?page=1&pageSize=10&author_id=
```

无认证。按 `create_time` 倒序返回图片资源。

**Query 参数**

| 参数 | 类型 | 必填 | 默认 | 说明 |
|---|---|---|---|---|
| `page` | int | 否 | 1 | 页码，从 1 开始；`< 1` 或非数字时回退为 1 |
| `pageSize` | int | 否 | 10 | 每页数量，1–100；超出范围或非数字时回退为 **10** |
| `author_id` | int64 | 否 | — | `authors.id`，传入时只返回该作者作品（作者专区） |

**响应 200**

```json
{
  "msg": "success",
  "code": 200,
  "data": {
    "totalCount": 137,
    "totalPage": 14,
    "pageSize": 10,
    "currPage": 1,
    "list": [
      {
        "id": "1859483273912345600",
        "resourceUrl": "https://static.example.com/images/2026/11/17/042913.jpg",
        "width": 1200,
        "height": 1600,
        "rating": 4.7,
        "views": "1k",
        "rarity": "R",
        "author": { "name": "Embrace", "avatar": "https://www.girls-rating.com/images/avatars/avatar1.webp" }
      }
    ]
  }
}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `totalCount` | int64 | 总条数 |
| `totalPage` | int64 | 总页数；`totalCount = 0` 时为 1 |
| `pageSize` | int | 服务端实际使用的每页数量 |
| `currPage` | int | 服务端实际使用的页码 |
| `list` | array | 列表，无数据时为 `[]`（不会是 `null`） |
| `list[].id` | **string** | 雪花 ID，字符串化返回（超出 JS 安全整数范围） |
| `list[].rating` | float64 | 评分，恢复浮点（原为截断取整） |
| `rarity` | string | 卡片等级 `"R"` / `"SR"` / `"SSR"` |
| `list[].author` | object | 来自 `authors` 表，含 `id`（作者专区跳转用） |
| `list[].resourceUrl` | string | 完整地址，规则同 `/api/random` |

**缓存行为**

- 结果按「页」缓存，key：`cache:image_resource:list:v1:page:{page}:pageSize:{pageSize}`，TTL `PAGE_CACHE_TTL_SECONDS`（默认 300s）。
- 上传成功后清空全部分页缓存，正常情况下新图立即可见。
- Redis 读写失败不影响响应，降级为直查 MySQL。

**错误**：500（MySQL 异常）。

## 4. 入口页最新图片

```
GET /api/latest?limit=10
```

无认证。按 `create_time` 倒序返回最新一批图片，供入口页（年龄门）高斯模糊背景墙使用。

**Query 参数**

| 参数 | 类型 | 必填 | 默认 | 说明 |
|---|---|---|---|---|
| `limit` | int | 否 | 10 | 数量，1–20；超出范围或非数字时回退为 **10** |

**响应 200** —— `data` 直接是数组（无分页包装），条目结构与 `/api/cosplay` 的 `list[]` 一致（含字符串化雪花 `id`）：

```json
{
  "msg": "success",
  "code": 200,
  "data": [
    {
      "id": "1859483273912345600",
      "resourceUrl": "https://static.example.com/images/2026/11/19/120000.jpg",
      "width": 1200,
      "height": 1600,
      "rating": 4.7,
      "views": "1k",
      "rarity": "SR",
      "author": { "name": "Embrace", "avatar": "https://www.girls-rating.com/images/avatars/avatar1.webp" }
    }
  ]
}
```

**行为说明**

- 复用分页列表查询（`create_time` DESC + 页缓存），与 `/api/cosplay` 第一页共享 `cache:image_resource:list:v1:page:1` 缓存条目。
- 上传成功后分页缓存被清空，正常情况下新图立即可见。
- Redis 异常不影响响应，降级直查 MySQL；库中无数据时返回 `"data": []`。

**错误**：500（MySQL 异常）。

## 5. 用户注册

```
POST /api/v1/register
Content-Type: application/json
```

**请求体**

| 字段 | 类型 | 必填 | 约束 |
|---|---|---|---|
| `username` | string | 是 | 3–50 字符，全局唯一 |
| `email` | string | 是 | 合法邮箱，全局唯一 |
| `password` | string | 是 | 至少 6 位；服务端 bcrypt 哈希存储 |

```json
{ "username": "testuser", "email": "test@example.com", "password": "123456" }
```

**响应 201** —— `data` 为用户实体（`users` 表，密码字段不返回）：

```json
{
  "msg": "success",
  "code": 201,
  "data": {
    "id": 1,
    "created_at": "2026-11-17T22:08:37+08:00",
    "updated_at": "2026-11-17T22:08:37+08:00",
    "username": "testuser",
    "email": "test@example.com",
    "nickname": "",
    "avatar": "",
    "status": 1
  }
}
```

**错误**

| code | msg | 场景 |
|---|---|---|
| 400 | 绑定错误明细 | 缺字段/格式不符 |
| 403 | `missing turnstile token` / `turnstile verification failed` | 人机验证缺 token 或 siteverify 未通过（配置 TURNSTILE_SECRET_KEY 后） |
| 409 | `username or email already exists` | 用户名或邮箱已占用 |
| 500 | 数据库错误 | 连接/写入异常 |

## 6. 用户登录

```
POST /api/v1/login
Content-Type: application/json
```

**请求体**

| 字段 | 类型 | 必填 |
|---|---|---|
| `username` | string | 是 |
| `password` | string | 是 |

```json
{ "username": "testuser", "password": "123456" }
```

**响应 200**

```json
{
  "msg": "success",
  "code": 200,
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
  }
}
```

| 字段 | 说明 |
|---|---|
| `access_token` | 访问令牌，`Authorization: Bearer <token>` 使用，有效期 `JWT_EXPIRE` 小时 |
| `refresh_token` | 7 天有效；**当前没有刷新接口消费它**，无实际用途 |

**错误**

| code | msg | 场景 |
|---|---|---|
| 400 | 绑定错误明细 | 缺字段 |
| 401 | `user not found` / `invalid password` | 用户不存在或密码错 |
| 403 | `missing turnstile token` / `turnstile verification failed` | 人机验证缺 token 或 siteverify 未通过（配置 TURNSTILE_SECRET_KEY 后） |
| 500 | `user is disabled` / 其他 | 账号被禁用（注：语义上应为 403）；数据库异常 |

## 7. 当前用户信息

```
GET /api/v1/user
Authorization: Bearer <access_token>
```

**响应 200** —— 结构与注册返回一致（`users` 实体，不含密码）：

```json
{
  "msg": "success",
  "code": 200,
  "data": {
    "id": 1,
    "created_at": "2026-11-17T22:08:37+08:00",
    "updated_at": "2026-11-17T22:08:37+08:00",
    "username": "testuser",
    "email": "test@example.com",
    "nickname": "",
    "avatar": "",
    "status": 1
  }
}
```

**错误**

| code | msg | 场景 |
|---|---|---|
| 401 | `missing authorization header` / `invalid authorization format` / `invalid or expired token` | 未带 token、格式不是 `Bearer <token>`、token 过期或签名错误 |
| 404 | `user not found` | token 合法但用户已被删除 |
| 500 | 数据库错误 | — |

## 8. 上传图片

```
POST /api/v1/upload/image
X-API-Key: <UPLOAD_API_KEY>
Content-Type: multipart/form-data
```

维护脚本专用，把图片写入 Cloudflare R2 并落库、清分页缓存、加入随机池。

**Form 字段**

| 字段 | 类型 | 必填 | 默认 | 说明 |
|---|---|---|---|---|
| `file` | file | 是 | — | 仅允许 `image/jpeg`、`image/png`、`image/webp`、`image/gif` |
| `width` | int | 否 | 自动检测 | 传则跳过解码探测 |
| `height` | int | 否 | 自动检测 | 同上 |
| `rating` | number | 否 | 4.0 | 传 0 视为未传 |
| `views` | string | 否 | `"1k"` | 传空视为未传 |
| `author_id` | int | 否 | 1 | 指向 `authors.id`；无效值回退默认作者；优先级高于 `author_name` |
| `author_name` | string | 否 | — | find-or-create 兜底通道（爬虫主流程建议先查/建作者拿 id） |
| `author_avatar` | string | 否 | — | 配合 `author_name` 创建作者时的头像外链 |
| `rarity` | string | 否 | 按 rating 派生 | 显式定档：`0/1/2` 或 `R/SR/SSR`；无法解析返回 400 |

定档规则（权威定义见 `docs/design/photo-system.md` §四）：显式 `rarity` 覆盖派生；缺省按 rating 阈值派生（`>=9 → SSR`、`>=7 → SR`，阈值可配 `PHOTO_SSR_MIN_RATING` / `PHOTO_SR_MIN_RATING`）。

```bash
curl -X POST http://localhost:8080/api/v1/upload/image \
  -H "X-API-Key: $UPLOAD_API_KEY" \
  -F "file=@./a.jpg" \
  -F "rating=4.7" \
  -F "views=2k"
```

**响应 201**

```json
{
  "msg": "success",
  "code": 201,
  "data": {
    "id": "1859483273912345600",
    "resourceUrl": "images/2026/11/17/042913.jpg",
    "width": 1200,
    "height": 1600,
    "rating": 4.7,
    "views": "2k",
    "rarity": "R",
    "author": { "name": "Embrace", "avatar": "https://www.girls-rating.com/images/avatars/avatar1.webp" }
  }
}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | string | 雪花 ID |
| `resourceUrl` | string | **R2 对象相对 key**，不拼 `S3_HOST`（与 2、3 号接口不同） |
| `rating` | float64 | 真实入库值，保留一位小数 |
| `rarity` | string | 实际落库档位（显式或派生结果），供爬虫确认 |
| `author` | object | 实际落库作者（回显） |

**错误**

| code | msg | 场景 |
|---|---|---|
| 400 | `missing file field` | 没有 `file` 字段 |
| 400 | `unsupported file type, allowed: jpeg, png, webp, gif` | MIME 不在白名单（`Content-Type` 缺失时按扩展名推断） |
| 400 | `invalid rarity, allowed: 0/1/2 or R/SR/SSR` | rarity 无法解析 |
| 401 | `missing X-API-Key header` / `invalid API key` | key 缺失或错误 |
| 403 | `upload API key not configured` | 服务端 `UPLOAD_API_KEY` 为空 |
| 500 | `failed to upload to r2` / `failed to save image resource` | 存储或落库失败 |

## 9. 抽卡

三个公开接口，统一要求请求头 `X-Device-Id`（缺失或超 64 字符返回 400）。设计见 `docs/design/gacha-system.md`。

### 8.1 POST /api/gacha/draw（每日抽卡，幂等补发）

每天每设备一次、固定 10 张；**当天已抽时不返回 409，而是补发当日快照**（`replayed: true`），前端凭此跳过动画。并发双击由唯一索引收敛到同一份结果。

**响应 200**

```json
{
  "msg": "success",
  "code": 200,
  "data": {
    "date": "2026-11-17",
    "replayed": false,
    "cards": [
      {
        "resourceUrl": "https://static.example.com/images/2026/11/17/042913.jpg",
        "width": 1200, "height": 1600,
        "rating": 4.7, "views": "1k",
        "rarity": "SSR",
        "author": { "name": "Embrace", "avatar": "https://..." }
      }
    ],
    "stats": { "totalDays": 3, "totalCards": 30, "ssr": 2, "sr": 7, "r": 21 }
  }
}
```

| 字段 | 说明 |
|---|---|
| `date` | 服务器时区（`GACHA_TIMEZONE`，默认 Asia/Shanghai）当日日期，前端一切「今天」以此为准 |
| `replayed` | true = 补发快照，不是新的随机事件，前端不得重播抽取动画 |
| `cards` | 10 张卡牌快照；单次内不重复；快照即存储，不受图片后续删除/改档影响 |
| `stats` | 该设备累计统计 |

抽样：固定权重掷档（`GACHA_SSR_WEIGHT` 6 / `GACHA_SR_WEIGHT` 24 / `GACHA_R_WEIGHT` 70）× 10 次，档内从 `cache:gacha_pool:v2:{ssr|sr|r}` 随机取；掷中档池空按 SSR→SR→R 降级；全空 → 500。Redis 不可用时 DB `ORDER BY RAND()` 按 rarity 回退。

**错误**：400 缺 `X-Device-Id`；500 池空/存储失败。没有 409。

### 8.2 GET /api/gacha/status

```json
{
  "msg": "success",
  "code": 200,
  "data": {
    "date": "2026-11-17",
    "drawnToday": false,
    "cards": null,
    "probabilities": { "R": 70, "SR": 24, "SSR": 6 },
    "stats": { "totalDays": 3, "totalCards": 30, "ssr": 2, "sr": 7, "r": 21 },
    "pool": { "R": 482, "SR": 96, "SSR": 11 }
  }
}
```

`drawnToday=true` 时 `cards` 为当日快照；`pool` 为各档池现存量（无 Redis 时为 `{}`），可缺省。

### 8.3 GET /api/gacha/collection?page=1&pageSize=30

卡册分页：全部快照按时间倒序展开，应用层切片分页（行数 = 天数，内存切片）。

| Query 参数 | 默认 | 说明 |
|---|---|---|
| `page` | 1 | 1–∞，非法回退 1 |
| `pageSize` | 30 | 1–100，非法回退 30 |

```json
{
  "msg": "success",
  "code": 200,
  "data": {
    "list": [ { "resourceUrl": "...", "rarity": "SSR", "author": { "id": 1, "name": "...", "avatar": "..." }, "drawDate": "2026-11-16" } ],
    "total": 30,
    "page": 1,
    "pageSize": 30,
    "stats": { "totalDays": 3, "totalCards": 30, "ssr": 2, "sr": 7, "r": 21 },
    "catalog": { "total": 28, "ssr": 2, "sr": 6, "r": 20 }
  }
}
```

每张卡带 `drawDate`（抽取日期）；`catalog` 为图鉴完成度统计（卡册内按 `resourceUrl` 去重的各档张数，与 `status.pool` 各档池存量相除即得完成度）。

## 10. 管理端：作者 CRUD

分组 `/api/v1/admin`，复用 `X-API-Key` 认证（与管理脚本同一信任域），面向爬虫自动化设计：**创建幂等、查询先行、重试安全**。设计见 `docs/design/photo-system.md` §5.2 / §5.4。

### 9.1 GET /api/v1/admin/authors?name=&keyword=&page=&pageSize=

`name` 精确匹配（**存在性检查**，命中返回单条 `total: 1`，未命中 `list: []`）；`keyword` 模糊分页（默认 pageSize 20）。

```json
{ "msg": "success", "code": 200, "data": { "list": [ { "id": 1, "created_at": "...", "updated_at": "...", "name": "Embrace", "avatar": "https://..." } ], "total": 1, "page": 1, "pageSize": 20 } }
```

### 9.2 POST /api/v1/admin/authors

请求体 `{ "name": "...", "avatar": "https://..." }`（name 必填，超 50 rune 截断）。**幂等**：name 已存在 → `200` + 既有记录 + `existed: true`；新建 → `201` + `existed: false`。爬虫崩溃重试不会产生重复。

```json
{ "msg": "success", "code": 201, "data": { "author": { "id": 2, "name": "...", "avatar": "" }, "existed": false } }
```

### 9.3 GET /api/v1/admin/authors/:id

详情；不存在/已软删 → `404`。

### 9.4 PATCH /api/v1/admin/authors/:id

请求体 `{ "name"?: "...", "avatar"?: "..." }`（指针字段，nil 跳过）。改名撞唯一索引 → `409`。

### 9.5 DELETE /api/v1/admin/authors/:id

**软删除**：同事务把 `name` 改写为 `{name}#del#{id}` 释放唯一槽位，允许重建同名；被引用图片经作者解析回退默认作者。

```json
{ "msg": "success", "code": 200, "data": { "affectedImages": 12 } }
```

### 9.6 POST /api/v1/admin/authors/:id/avatar

multipart `file` 字段（JPEG/PNG/WebP/GIF）→ R2 `avatars/` 前缀 → 更新 URL + 失效作者缓存。也可在创建/更新时直接填外链 URL。

## 11. 管理端：图片改档

```
PATCH /api/v1/admin/images/:id
X-API-Key: <UPLOAD_API_KEY>
Content-Type: application/json
```

请求体（指针字段，nil 跳过）：

| 字段 | 类型 | 说明 |
|---|---|---|
| `rating` | number | 改评分；有变且未显式给 rarity 时按阈值重算定档 |
| `rarity` | int | 显式定档 `0/1/2`（覆盖派生） |
| `views` | string | 改浏览量 |
| `author_id` | int | 改挂作者；无效值回退默认作者 |

副作用：落库 + 清分页缓存 + SADD 随机池与抽卡池新成员（旧成员由池定时刷新 RENAME 全量替换兜底）。

**响应 200** —— `data` 为统一照片结构（同 §3 `list[]`，含 id/rarity/author）。

**错误**：400 参数 / 401 认证 / 404 图片不存在。

## 12. 邮件反馈

```
POST /api/v1/mail/contact
X-Turnstile-Token: <turnstile-token>
Content-Type: application/json
```

公开接口：把用户反馈经 **Resend** 中继到支持邮箱（`MAIL_CONTACT_TO`），`Reply-To` 指向用户邮箱。受 Cloudflare Turnstile 人机验证保护——服务端配置了 `TURNSTILE_SECRET_KEY` 时必须携带前端 widget 产出的 `X-Turnstile-Token`（一次性），未配置时不校验。

**请求体**

| 字段 | 类型 | 必填 | 约束 |
|---|---|---|---|
| `email` | string | 是 | 合法邮箱，作为 Reply-To |
| `name` | string | 否 | ≤100 字符，缺省按「匿名用户」展示 |
| `message` | string | 是 | ≤5000 字符，正文（服务端 HTML 转义后拼信，防注入） |

**响应 200** —— `data: {}`。

**错误**

| code | msg | 场景 |
|---|---|---|
| 400 | 绑定错误明细 / `message too long` 等 | 参数错误 |
| 403 | `missing turnstile token` / `turnstile verification failed` | 人机验证缺 token 或 siteverify 未通过 |
| 502 | `resend: ...` | Resend 上游错误 |
| 503 | `mail not configured` | `RESEND_API_KEY` / `MAIL_FROM` / `MAIL_CONTACT_TO` 未配置齐 |

## 13. Swagger UI

```
GET /swagger/index.html
```

无认证。数据源为 `docs/swagger.json`，由 `make swagger`（`swag init -g cmd/main.go`）从 handler 注释生成。

---

## 实现约束与已知问题

以下是当前实现的真实行为，调用方/后续开发者需要知道：

1. **`rating` 精度已统一为 float**（2、3 号接口原为取整，现返回 `decimal(10,1)` 原值）；上传接口一直是原始 float。
2. **`id` 类型不一致**：列表/上传返回字符串，用户接口返回数字，`/api/random` 不返回 `id`。
3. **排序稳定性**：列表仅按 `create_time DESC` 排序，无 tiebreaker。`create_time` 为秒级精度，批量上传同秒写入的记录在深翻页时可能重复或漏出。
4. **分页参数静默纠正**：非法值不报 400，直接回退默认值（`pageSize=1000` 会变成默认值），前端需以响应里的 `currPage`/`pageSize` 为准。
5. **`page` 无上限**，深翻页走 `OFFSET`，页数很大时查询会变慢。
6. **空结果 `totalPage` 为 1**，出现「0 条数据却有 1 页」。
7. **`refresh_token` 不可用**：没有刷新端点，且签发时 `Subject` 用 `string(rune(userID))`，取值异常（见 `pkg/jwt/jwt.go`）。
8. **上传无大小限制**：文件全量读入内存（`io.ReadAll`），MIME 仅看 `Content-Type`/扩展名，不做真实内容嗅探。
9. **`author` 已实体化**：来自 `authors` 表（`author_id` 解析 + 5min 内存缓存），无效/已删作者回退默认作者 id=1；不再硬编码。
10. **Redis 是硬依赖**：启动时 Ping 失败直接退出，`service` 层中 `rdb == nil` 的降级分支实际不会被执行。
11. **池 key 已升到 v2**：`cache:random_pool:v2:set` 与 `cache:gacha_pool:v2:*` 成员为 PhotoRow JSON（含 rarity/authorId）；旧 v1 key 无消费者，可在确认无回滚需求后清理。
12. **抽卡每日幂等靠唯一索引**：`tb_girls_rating_gacha_draw` 的 `uq(device_id, draw_date)`；deviceId 可清 localStorage 重抽（纯娱乐已接受，可选 IP 限流未实现，留配置位）。
13. **增量迁移需手动执行**：`002_photo_upgrade.sql` / `003_gacha.sql` 只在 Docker 初始化空数据目录时自动跑；已有库需手工执行（含 authors 建表、seed、rarity 回填）。

## 维护约定

- 新增/修改接口的落地顺序：`models → repository → service → handler → api/handlers/server.go 路由 → swag 注释`，然后执行 `make swagger`。
- 同步更新本文件：接口一览表 + 对应章节 + （如果影响调用方）「实现约束与已知问题」。
- 本文件为手写文档，`docs/docs.go`、`docs/swagger.json`、`docs/swagger.yaml` 是生成产物，不要手改，也不要让 `swag` 的 `-o` 指向本文件。
