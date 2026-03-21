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

- Go 1.26+
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

2. 查看日志

```bash
docker-compose logs -f api
```

3. 停止服务

```bash
make docker-down
# 或
docker-compose down
```

## API 接口

### 健康检查

```bash
GET /health
```

### 用户注册

```bash
POST /api/v1/register
Content-Type: application/json

{
  "username": "testuser",
  "email": "test@example.com",
  "password": "123456"
}
```

### 用户登录

```bash
POST /api/v1/login
Content-Type: application/json

{
  "username": "testuser",
  "password": "123456"
}
```

响应:
```json
{
  "code": 200,
  "message": "login successful",
  "data": {
    "access_token": "eyJhbGc...",
    "refresh_token": "eyJhbGc..."
  }
}
```

### 获取用户信息

```bash
GET /api/v1/user
Authorization: Bearer <access_token>
```

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

## License

MIT
