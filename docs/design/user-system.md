# 用户体系设计文档（简化版）

| 项目 | 内容 |
|---|---|
| 状态 | 草案 v0.3（会话管理中移到 Redis 白名单） |
| 日期 | 2026-11-17 |
| 关联文档 | [`docs/API.md`](../API.md)、[`CLAUDE.md`](../../CLAUDE.md) |
| 设计原则 | **能不加就不加**：不建会话表、不做 refresh 轮转、不加登录日志表；登录态用 Redis 一个 key 表达 |

---

## 一、方案概要

一句话：**`users` 表 + 7 天 JWT（带 `jti`）+ Redis 白名单管登录态 + 注册过 Cloudflare Turnstile**。

```
注册 ──► Turnstile 校验 ──► INSERT users ──► 签发 JWT ──┐
                                                       │
登录 ──► bcrypt 校验 ────────────────────► 签发 JWT ───┤
                                                       ├──► SET {env}:auth:sess:{uid}:{jti}
                                                       │
每个请求 ──► 验签 JWT ──► EXISTS 该 session key ──► 放行
```

心智模型只有三句话：

- **登录** = 写一个 key
- **鉴权** = 看这个 key 在不在
- **登出 / 踢人** = 删这个 key

JWT 在这里只是**载体**：负责携带 `user_id`、`jti` 并保证不可伪造，让服务端不必每次都查库就能拿到身份；**登录态本身由 Redis 决定**，所以"删缓存就下线"成立。

存储分工：

| 存什么 | 放哪 | 说明 |
|---|---|---|
| 用户账号 | MySQL `users` | 已有表，结构不动 |
| 登录态（有效与否） | **Redis** `{env}:auth:sess:{uid}:{jti}` | 删除即失效，TTL 与 JWT 一致 |
| 身份信息 | JWT claims | 自包含，验签即得，不查库 |
| 限流计数 / 验证码去重 | Redis | 计数器 + TTL |

没有会话表、没有 refresh token、没有过期记录清理任务（Redis TTL 自动回收）。

---

## 二、要解决的问题

| # | 问题 | 处理 |
|---|---|---|
| 1 | 注册接口对任何人开放，可被脚本批量灌号 | 强制 Turnstile |
| 2 | 登录错误区分「用户不存在 / 密码错误」，可枚举账号 | 统一为 `invalid credentials` |
| 3 | 没有登出、封禁要等 token 过期才生效 | Redis 白名单，删 key 立即生效 |
| 4 | handler 直接返回 `*models.User` 实体 | 改用 DTO |

**不改**的：bcrypt cost 10、`users` 表结构、主键类型。

---

## 三、人机验证（Cloudflare Turnstile）

### 3.1 前端

```html
<script src="https://challenges.cloudflare.com/turnstile/v0/api.js" async defer></script>

<form>
  <input name="username" />
  <input name="email" />
  <input name="password" type="password" />
  <div class="cf-turnstile"
       data-sitekey="<TURNSTILE_SITE_KEY>"
       data-action="register"
       data-theme="auto"
       data-language="zh-cn"></div>
  <button type="submit">注册</button>
</form>
```

- widget 会往表单注入隐藏字段 `cf-turnstile-response`，前端把它作为 `turnstileToken` 提交。
- **token 只能用一次**：提交失败后必须调 `turnstile.reset()` 重新出 token，否则第二次一定报 `timeout-or-duplicate`。
- sitekey 直接写死在前端即可，省掉一个下发接口。

### 3.2 服务端校验

新建 `pkg/turnstile/client.go`，只用标准库 `net/http`，不引 SDK。

| 项 | 值 |
|---|---|
| Endpoint | `POST https://challenges.cloudflare.com/turnstile/v0/siteverify` |
| 必填参数 | `secret`、`response` |
| 可选参数 | `remoteip`（传客户端 IP）、`idempotency_key`（重试时用） |
| 响应字段 | `success`、`error-codes[]`、`hostname`、`action`、`challenge_ts` |
| Token 有效期 | 300 秒 |
| Token 长度上限 | 2048 字符（超长直接拒绝，不打上游） |

**必须校验的四项**（只看 `success` 是不够的）：

1. `success == true`
2. `action == "register"` —— 防止把别处拿到的 token 拿来注册
3. `hostname` 在白名单内（`TURNSTILE_EXPECTED_HOSTNAME`）
4. token 未被用过（见 3.3）

### 3.3 一次性兜底（三行代码，别省）

Cloudflare 官方明确：token 去重是**最终一致**的，只承诺「拒绝 5 秒后 99% 的重复请求」，也就是说**并发提交可能都成功**。所以本地加一道锁：

```
SET {env}:auth:ts:{sha256(token)} 1 NX EX 300
```

- `NX` 失败 → 已用过，返回 `CAPTCHA_INVALID`，提示前端 reset。
- TTL 300s 与 token 有效期一致，自动过期，不用清理。

### 3.4 错误码映射

| Cloudflare `error-codes` | HTTP | 前端动作 |
|---|---|---|
| `missing-input-response` | 400 | 提示先完成验证 |
| `invalid-input-response` / `timeout-or-duplicate` / `bad-request` | 400 | `turnstile.reset()` 后重试 |
| `invalid-input-secret` / `missing-input-secret` | 503 | 服务端配置错，**记 error 日志**，不暴露细节 |
| `internal-error` / 网络超时 / 非 2xx | 503 | 提示「验证服务暂不可用，请稍后重试」 |

上游超时 3 秒（`TURNSTILE_TIMEOUT_MS`）。校验服务不可用时**注册直接拒绝**（fail-close）——注册低频，宁可让用户稍后重试也不能放开。

---

## 四、JWT 与登录态管理

### 4.1 Token 结构

| 项 | 值 |
|---|---|
| 算法 | HS256 |
| 有效期 | **7 天**（`.env` 里 `JWT_EXPIRE=168`，单位小时，配置代码无需改） |
| Claims | `user_id`、`username`、**`jti`**、`iat`、`exp`、`iss` |
| 传输 | `Authorization: Bearer <token>` |
| 前端存放 | `localStorage` |

`jti`（16 字节随机 hex）是白名单方案的必要前提：没有它，同一用户的多次登录无法区分，也就无法「只登出当前设备」。

### 4.2 Redis 白名单

| 操作 | Redis 命令 |
|---|---|
| 登录 / 注册成功 | `SET {env}:auth:sess:{user_id}:{jti} 1 EX <剩余秒数>` |
| 每次鉴权 | `EXISTS {env}:auth:sess:{user_id}:{jti}` |
| 登出当前设备 | `DEL {env}:auth:sess:{user_id}:{jti}` |
| 踢掉某用户全部会话 | `SCAN {env}:auth:sess:{user_id}:*` + `DEL` |
| 单设备登录（可选） | 登录前先执行上面那条「踢全部」 |

设计要点：

- **key 里带 `user_id`**：鉴权时 JWT 里有 `user_id` 和 `jti`，能直接拼出完整 key 精确命中；封号时又能用前缀批量删除，**不需要额外维护索引 Set**（这是比 v0.1 那套方案省下来的关键一步）。
- **value 只存 `"1"`**：身份信息从 JWT 拿，不必冗余。10 万会话也就几 MB。
- **TTL 与 JWT `exp` 严格一致**：写入时算 `exp - now`。两者语义对齐，不会出现「Redis 说有效但 JWT 已过期」或反过来的矛盾。
- **不续期**：不做滑动 TTL。JWT `exp` 是绝对上限，Redis TTL 只是它的镜像，语义单一、不会撕扯。若将来想「活跃就保持登录」，见 4.5。

### 4.3 故障策略：fail-open

| Redis 情况 | 行为 |
|---|---|
| key 存在 | 放行 |
| key 不存在（miss） | **401 `TOKEN_REVOKED`**，前端跳登录 |
| Redis 查询报错 / 超时 | **放行**，记 warn 日志，退化为纯 JWT 校验 |

为什么故障时放行而不是拒绝：

- JWT 签名已经验过了，身份可信，只是不知道「是否被登出/封禁」；
- 如果 fail-close，Redis 抖动一下全站用户被踢下线，可用性代价远大于安全收益；
- 故障窗口内失去的只是「即时踢人」能力，这是可接受的降级。

反过来，**Redis 正常时踢人是立即生效的**，这正是这个方案要买的东西。

### 4.4 代价与取舍

| 维度 | 纯自包含 JWT（v0.2） | JWT + Redis 白名单（v0.3，当前） |
|---|---|---|
| 每次鉴权开销 | 0 次 IO | 1 次 `EXISTS`（~0.2ms，同机房） |
| 登出 | 前端删 token，服务端仍认 | **真失效** |
| 封禁生效时间 | 最长 7 天 / 需另做黑名单 | **立即** |
| 同一账号多设备 | 无法区分、无法单独下线 | 可区分、可单独踢 |
| Redis 故障 | 完全无影响 | fail-open 降级，仅失去踢人能力 |
| 新增代码 | — | 1 个 service（约 60 行）+ 中间件 5 行 |
| 新增表 | — | 无 |

结论：多一次 0.2ms 的 Redis 往返，换来「删除即登出」的完整会话控制，在小系统里是划算的。

### 4.5 如果将来想做「活跃即保持登录」

把 JWT `exp` 设成上限（如 30 天），Redis TTL 设不活跃超时（如 7 天），每次鉴权用 `GETEX` 续期（Redis 6.2+；go-redis v8 若未暴露该方法，用 `pipeline{EXISTS, EXPIRE}` 合并成一次往返）。

注意此时**权威是 Redis**，JWT `exp` 只是硬上限：一旦 JWT 过期，即便 Redis key 还在也必须重新登录（以 JWT 为准，避免无限续期）。

### 4.6 为什么仍然不要 refresh token

白名单已经解决了「登出 / 踢人」，refresh 剩下的唯一价值是「短 access + 长 refresh」以缩小 token 泄露窗口。小系统 7 天 token 可接受，不值得为此引入轮转、重放检测和过期清理。详见附录 A。

---

## 五、接口

| Method | Path | 认证 | 说明 |
|---|---|---|---|
| POST | `/api/v1/auth/register` | Turnstile | 注册，返回 JWT，写 session |
| POST | `/api/v1/auth/login` | 无 | 登录，返回 JWT，写 session |
| POST | `/api/v1/auth/logout` | JWT | 删当前 session key；**幂等**，无论 key 在不在都返回 200 |
| GET | `/api/v1/user` | JWT | 当前用户信息（DTO） |

旧的 `/api/v1/register`、`/api/v1/login` 直接替换为上面的路径，不做 deprecated 兼容层。

### 5.1 注册

```json
POST /api/v1/auth/register
{
  "username": "testuser",
  "email": "Test@Example.com",
  "password": "Str0ngPassw0rd",
  "turnstileToken": "0.xxxxx..."
}
```

`201`

```json
{
  "msg": "success",
  "code": 201,
  "data": {
    "token": "eyJhbGciOi...",
    "expiresIn": 604800,
    "user": {
      "id": 1,
      "username": "testuser",
      "email": "test@example.com",
      "nickname": "",
      "avatar": "",
      "status": 1
    }
  }
}
```

### 5.2 登录

```json
POST /api/v1/auth/login
{ "identifier": "testuser", "password": "Str0ngPassw0rd" }
```

`200` 结构同上。`identifier` 同时支持用户名和邮箱。失败统一 `401`，`msg` 固定 `invalid credentials`。

### 5.3 登出

```json
POST /api/v1/auth/logout
Authorization: Bearer <token>
```

从 claims 取 `user_id + jti`，`DEL` 对应 key。返回 `200 {}`。token 已过期也返回 200（幂等），前端只管清 localStorage。

### 5.4 错误响应

沿用现有 `{msg, code, data}`，`code` 与 HTTP 状态码一致。新增可选字段 `bizCode`，只在 auth 接口使用，便于前端区分「验证码错」和「参数错」（都是 400）：

```json
{ "msg": "人机验证未通过，请重试", "code": 400, "bizCode": "CAPTCHA_INVALID", "data": {} }
```

| bizCode | HTTP | 说明 |
|---|---|---|
| `VALIDATION_FAILED` | 400 | 字段格式/长度不合法 |
| `CAPTCHA_REQUIRED` | 400 | 没传 `turnstileToken` |
| `CAPTCHA_INVALID` | 400 | token 无效/过期/已用/action 不匹配 |
| `CAPTCHA_UNAVAILABLE` | 503 | 验证服务不可用 |
| `INVALID_CREDENTIALS` | 401 | 账号或密码错误（统一） |
| `TOKEN_REVOKED` | 401 | session key 不存在（已登出/被踢） |
| `ACCOUNT_DISABLED` | 403 | `status != 1` |
| `RATE_LIMITED` | 429 | 触发限流，`data.retryAfter` |
| `USERNAME_TAKEN` / `EMAIL_TAKEN` | 409 | 注册重复 |

给 `pkg/response` 加 `ErrorWithBiz`，原 `Error` 不动。

---

## 六、Redis key 一览

| Key | TTL | 阈值 | 用途 |
|---|---|---|---|
| `{env}:auth:sess:{uid}:{jti}` | 7d（= JWT exp） | — | 登录态白名单 |
| `{env}:auth:rl:register:ip:{ip}` | 1h | 5 | 单 IP 注册上限 |
| `{env}:auth:fail:id:{identifier}` | 15min | 5 | 单账号登录失败上限 |
| `{env}:auth:ts:{sha256(token)}` | 300s | — | 验证码一次性 |

限流用固定窗口，`INCR` 后仅首次设 TTL（避免滑动窗口被慢慢磨）：

```go
n, _ := rdb.Incr(ctx, key).Result()
if n == 1 {
    rdb.Expire(ctx, key, ttl)
}
if n > limit {
    return ErrRateLimited
}
```

登录成功时 `DEL {env}:auth:fail:id:{identifier}`。

> 不做渐进式验证码（失败后要求 Turnstile）——需要前端二次渲染 widget，交互复杂。直接用 429 + 15 分钟冷却。

---

## 七、安全清单

| 项 | 措施 |
|---|---|
| 密码存储 | bcrypt cost 10（保持现状） |
| 密码策略 | 8–64 **字节**（bcrypt 只取前 72 字节，必须显式限长，否则静默截断） |
| 密码传输 | 仅 HTTPS；日志不记密码和 token 原文 |
| 账号枚举 | 登录统一错误文案；注册的 409 由 Turnstile + IP 限流兜底 |
| 撞库 | 单账号 5 次失败冷却 15 分钟 |
| 会话控制 | Redis 白名单，删除即失效；key 不对外暴露（只出现在 JWT 的 `jti`） |
| 越权 | 用户身份一律取自 JWT claims，绝不信任 body 里的 `user_id` |
| 密钥 | `TURNSTILE_SECRET_KEY`、`JWT_SECRET` 只从环境变量读；生产缺失即启动失败 |
| token 泄露 | 7 天自然过期 + 可随时 `DEL` session key 吊销 |

---

## 八、配置

`.env` 新增 / 调整：

| 变量 | 说明 | 默认 |
|---|---|---|
| `TURNSTILE_ENABLED` | 总开关，`false` 跳过校验（仅本地开发） | `true` |
| `TURNSTILE_SECRET_KEY` | 服务端 secret，生产必填 | - |
| `TURNSTILE_VERIFY_URL` | 校验地址；大陆用户多时改 `https://challenges.cloudflare-cn.com/turnstile/v0/siteverify` | 官方地址 |
| `TURNSTILE_EXPECTED_HOSTNAME` | 允许的 hostname，逗号分隔 | 空=不校验（仅开发） |
| `TURNSTILE_TIMEOUT_MS` | 上游超时 | `3000` |
| `JWT_EXPIRE` | **改为 `168`**（小时 = 7 天） | 现有 24 |
| `SESSION_ENABLED` | 是否启用 Redis 白名单校验；`false` 退回纯 JWT 模式（应急开关） | `true` |

`SESSION_ENABLED=false` 是一条实用的回滚路径：万一 Redis 出问题或白名单逻辑有 bug，改配置重启即可退化成 v0.2 行为，不需要改代码。

另需修掉 `.env.example` 里两个拼错的键（`REDIS_REEK` → `REDIS_HOST`、`JWT_QGYODG` → `JWT_SECRET`）。

---

## 九、代码改动清单

| 层 | 文件 | 变更 |
|---|---|---|
| pkg | `pkg/turnstile/client.go` | **新增** Verifier + 错误码映射（约 120 行） |
| pkg | `pkg/jwt/jwt.go` | Claims 加 `jti`；签发时生成 |
| service | `internal/service/session.go` | **新增** `Create/Exists/Delete/DeleteAllByUser`（约 60 行） |
| service | `internal/service/captcha.go` | **新增** 四项校验 + Redis 去重（约 60 行） |
| service | `internal/service/user.go` | 改造 `Register`/`Login`：加验证码、返回 `(user, token, jti)` |
| handlers | `api/handlers/auth.go` | **新增** register / login / logout |
| handlers | `api/handlers/user.go` | 返回 `UserResponse` DTO |
| middleware | `internal/middleware/auth.go` | 验签后加 `EXISTS` 白名单检查（fail-open） |
| middleware | `internal/middleware/ratelimit.go` | **新增** 固定窗口（约 40 行） |
| response | `pkg/response/response.go` | 新增 `ErrorWithBiz` |
| config | `internal/config/config.go` | 新增 `Turnstile` 配置块 + `SESSION_ENABLED` |
| router | `api/handlers/server.go` | 替换 `/api/v1/auth/*` 路由 |
| 文档 | `docs/API.md` | 同步接口契约 |

**不引第三方依赖**：Turnstile 用 `net/http`，`jti` 用 `crypto/rand`，限流用现有 `go-redis`。

**不改表**：`users` 结构保持现状。唯一的数据处理是邮箱小写归一化（写入前 `strings.ToLower(strings.TrimSpace(email))`），存量数据用一条 SQL 修正：

```sql
UPDATE users SET email = LOWER(email) WHERE email <> LOWER(email);
```

---

## 十、后续业务（本期不实现，仅确定方向）

### 收藏

```sql
CREATE TABLE IF NOT EXISTS `user_favorites` (
  `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `user_id`    INT UNSIGNED NOT NULL,
  `image_id`   BIGINT NOT NULL COMMENT 'tb_girls_rating_image_resource.id',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_user_image` (`user_id`, `image_id`),
  KEY `idx_user_time` (`user_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户收藏';
```

接口：`POST /api/v1/favorites`（body 传 `imageId`）、`DELETE /api/v1/favorites/:imageId`、`GET /api/v1/favorites`（分页，复用现有分页约定）。唯一索引保证重复收藏幂等。

### 生成图片

生成是异步任务，需要任务表记录状态与结果：

```sql
CREATE TABLE IF NOT EXISTS `image_generation_tasks` (
  `id`          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `user_id`     INT UNSIGNED NOT NULL,
  `prompt`      TEXT NOT NULL,
  `status`      TINYINT NOT NULL DEFAULT 0 COMMENT '0=排队 1=生成中 2=成功 3=失败',
  `result_url`  VARCHAR(500) NULL DEFAULT NULL,
  `error_msg`   VARCHAR(255) NULL DEFAULT NULL,
  `created_at`  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `finished_at` DATETIME(3) NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_user_time` (`user_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='图片生成任务';
```

生成接口需要按用户限配额（Redis 计数器 + `user_id` 维度）。
> 完整设计已单独成文：[`task-system.md`](./task-system.md)。该文档把表定为 `image_tasks`，并补齐状态机、租约与僵尸回收、退避重试、取消、配额和 worker 设计；本节仅作方向记录。

---

## 十一、测试

| 范围 | 用例 |
|---|---|
| Turnstile 客户端 | `httptest.Server` 模拟成功、7 个 error-code、超时、非 2xx、`action` 不匹配、hostname 不在白名单 |
| 验证码去重 | 同一 token 第二次提交被拒 |
| 注册 | 正常、用户名重复、邮箱重复（大小写不同）、弱密码、超长密码（>64 字节） |
| 登录 | 成功、密码错、用户不存在（文案一致）、禁用用户 403 |
| 会话 | 登录后鉴权通过；`DEL` key 后立即 401；`SESSION_ENABLED=false` 时不查 Redis；Redis 报错时放行（fail-open） |
| 限流 | 第 6 次注册 / 第 6 次登录失败返回 429 |

用 Turnstile 官方测试 key（sitekey `1x00000000000000000000AA` + 对应测试 secret）可在本地跑通全流程。

---

## 十二、里程碑

| 阶段 | 内容 |
|---|---|
| M1 | Turnstile + 验证码去重 + 注册/登录改造 + Redis 白名单 + 限流 + DTO + 文档。**用户体系可用** |
| M2 | 收藏功能（表 + 3 个接口 + 前端联调） |
| M3 | 生成图片（异步任务 + 配额） |

---

## 附录 A：演进记录与不再讨论的选项

### A.1 三轮演进

| 版本 | 会话方案 | 结论 |
|---|---|---|
| v0.1 | MySQL 会话表 + refresh 轮转 + 重放检测 + 登录日志表 | 过度设计，砍掉 |
| v0.2 | 纯自包含 JWT，Redis 不参与鉴权 | 简单，但登出与封禁不生效 |
| **v0.3** | **JWT 带 `jti` + Redis 白名单** | **当前方案，成本最低的完整会话控制** |

### A.2 仍然不做

| 方案 | 否决原因 | 什么时候该重新考虑 |
|---|---|---|
| refresh token + 轮转 + 重放检测 | 白名单已解决登出/踢人；refresh 剩余价值仅是缩小泄露窗口 | 有明确的 token 泄露风险，或需要「access 极短 + 无感续期」 |
| MySQL 会话表 | 白名单一个 key 就够，不需要查询历史会话 | 需要「查看登录设备列表 / 按设备管理」 |
| `user_login_logs` 审计表 | 小系统没人看；结构化日志够用 | 有合规要求或盗号投诉排查 |
| 渐进式验证码（登录失败后要求 Turnstile） | 前端二次渲染 widget，交互复杂；429 冷却更简单 | 出现大规模撞库 |
| 滑动续期（active = 保持登录） | 需要 JWT 长 exp + Redis 短 TTL + 每次续期，语义变复杂 | 用户抱怨 7 天不够（见 4.5 的做法） |
| `bizCode` 全站推广 | 只在 auth 接口用，不动老契约 | 前端统一错误处理时 |

### A.3 升级信号（出现任一条再补复杂度）

1. 需要「查看登录设备 / 按设备下线」→ 会话表或加 IP/UA 到 session value
2. 用户抱怨 7 天不够 → 滑动续期（4.5）
3. 出现撞库或盗号 → 登录日志 + 渐进式验证码
4. Redis 频繁抖动导致「莫名掉线」→ 重新评估 fail-open 策略或引入本地缓存

---

## 附录 B：参考资料

- Cloudflare Turnstile · 服务端校验：<https://developers.cloudflare.com/turnstile/get-started/server-side-validation/>
- Cloudflare Turnstile · 测试密钥：<https://developers.cloudflare.com/turnstile/troubleshooting/testing/>
