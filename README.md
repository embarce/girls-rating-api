# Girls Rating API

基于 Go 构建的 RESTful API 服务，支持 MySQL、Redis、JWT 认证，可使用 Docker 部署。

## 技术栈

- **Web 框架**: [Gin](https://github.com/gin-gonic/gin)
- **ORM**: [GORM](https://gorm.io/)
- **数据库**: MySQL 8.0
- **缓存**: Redis 7
- **认证**: JWT (golang-jwt/jwt/v5)
- **配置管理**: Viper
- **参数验证**: go-playground/validator
- **容器化**: Docker & Docker Compose

## 项目结构

```
.
├── cmd/
│   └── main.go              # 应用入口
├── internal/
│   ├── config/              # 配置加载
│   ├── database/            # 数据库连接
│   ├── handlers/            # HTTP 处理器
│   ├── middleware/          # Gin 中间件
│   ├── models/              # GORM 模型
│   ├── repository/          # 数据访问层
│   └── service/             # 业务逻辑层
├── pkg/
│   ├── jwt/                 # JWT 服务
│   └── redis/               # Redis 客户端
├── migrations/              # 数据库迁移脚本
├── docker-compose.yml       # Docker Compose 配置
├── Dockerfile               # Docker 镜像构建
└── Makefile                 # 常用命令
```

## 快速开始

### 前置条件

- Go 1.25+
- Docker & Docker Compose
- MySQL 8.0+
- Redis 7+

### 本地开发

1. 克隆项目

```bash
git clone <repository-url>
cd girls-rating-api
```

2. 配置环境变量

```bash
cp .env.example .env
# 编辑 .env 文件配置数据库和 Redis 连接
```

3. 安装依赖

```bash
go mod download
```

4. 运行服务

```bash
make run
# 或
go run cmd/main.go
```

### 使用 Docker 部署

1. 启动所有服务

```bash
make docker-up
# 或
docker-compose up -d
```

2. 启动后请手动执行数据库迁移 SQL（例如 `migrations/001_init.sql`），以创建/更新表结构

3. 查看日志

```bash
docker-compose logs -f api
```

4. 停止服务

```bash
make docker-down
# 或
docker-compose down
```

## API 接口

完整接口说明（参数、响应示例、错误码、缓存行为）见 **[docs/API.md](docs/API.md)**。

| Method | Path | 认证 | 说明 |
|---|---|---|---|
| GET | `/health` | 无 | 健康检查 |
| GET | `/api/random` | 无 | 随机返回一批图片（Redis 随机池） |
| GET | `/api/cosplay` | 无 | 图片资源分页列表（Redis 分页缓存） |
| POST | `/api/v1/register` | 无 | 用户注册 |
| POST | `/api/v1/login` | 无 | 用户登录，签发 JWT |
| GET | `/api/v1/user` | JWT | 获取当前登录用户 |
| POST | `/api/v1/upload/image` | `X-API-Key` | 上传图片到 R2 并落库 |
| GET | `/swagger/index.html` | 无 | Swagger UI |

统一响应格式：

```json
{ "msg": "success", "code": 200, "data": {} }
```

启动服务后可访问 `http://localhost:8080/swagger/index.html` 在线调试。

## Make 命令

| 命令 | 说明 |
|------|------|
| `make run` | 运行应用 |
| `make build` | 编译应用 |
| `make test` | 运行测试 |
| `make docker-build` | 构建 Docker 镜像 |
| `make docker-up` | 启动 Docker 服务 |
| `make docker-down` | 停止 Docker 服务 |

## 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `APP_PORT` | 服务端口 | 8080 |
| `APP_ENV` | 运行环境 | development |
| `MYSQL_HOST` | MySQL 主机 | localhost |
| `MYSQL_PORT` | MySQL 端口 | 3306 |
| `MYSQL_USER` | MySQL 用户 | root |
| `MYSQL_PASSWORD` | MySQL 密码 | - |
| `MYSQL_DATABASE` | 数据库名 | girls_rating |
| `REDIS_HOST` | Redis 主机 | localhost |
| `REDIS_PORT` | Redis 端口 | 6379 |
| `REDIS_PASSWORD` | Redis 密码 | - |
| `JWT_SECRET` | JWT 密钥 | - |
| `JWT_EXPIRE` | JWT 过期时间 (小时) | 24 |
| `GIN_TRUSTED_PROXIES` | Gin 可信代理列表（逗号分隔，IP 或 CIDR） | 空表示不信任任何代理 |

## License

MIT
