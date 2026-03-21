# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
go run cmd/main.go          # Run application
go build -o bin/app ./cmd/main.go  # Build binary
go test -v ./...            # Run all tests
go test -v ./internal/service/user_test.go  # Run single test file
go test -v -run TestName ./...  # Run specific test function
go mod download             # Install dependencies
swag init -g cmd/main.go    # Generate Swagger docs
```

## Swagger

访问 `http://localhost:8080/swagger/index.html` 查看 API 文档。

```bash
make swagger                # Generate Swagger documentation
swag init -g cmd/main.go    # Regenerate docs after API changes（默认输出到 ./docs，包名 docs）
```

**注意：** `cmd/main.go` 需空白导入 `_ "girls-rating-api/docs"`，否则 `docs` 包里的 `init()` 不会执行，Swagger UI 会报无法加载 API 定义。不要使用另一套输出目录（如 `-o ./docs/swagger`）除非同步修改导入路径。

## Docker

```bash
docker-compose up -d        # Start MySQL, Redis, and API
docker-compose down         # Stop all services
docker build -t girls-rating-api:latest .
```

## Architecture

**Layered architecture** following clean architecture principles:

```
cmd/main.go (entry point)
    ↓
api/handlers (HTTP layer, Gin routes)
    ↓
internal/service (business logic, validation, JWT)
    ↓
internal/repository (data access, GORM)
    ↓
internal/models (GORM entities)
```

**Key packages:**
- `pkg/jwt` - JWT token generation and validation
- `pkg/redis` - Redis client wrapper
- `internal/middleware` - JWT authentication middleware
- `internal/config` - Viper-based configuration from .env

**Data flow:** Handler → Service → Repository → Database

## API Endpoints

| Method | Endpoint | Auth |
|--------|----------|------|
| GET | /health | No |
| GET | /api/random | No |
| POST | /api/v1/register | No |
| POST | /api/v1/login | No |
| GET | /api/v1/user | Yes (JWT) |

## Configuration

Loads from `.env` file or environment variables. Key vars:
- `APP_ENV` - development/debug/test (controls Gin mode)
- `APP_PORT` - Server port (default: 8080)
- `MYSQL_*` - Database connection
- `REDIS_*` - Redis connection
- `JWT_SECRET`, `JWT_EXPIRE`, `JWT_ISSUER` - JWT configuration
- `GIN_TRUSTED_PROXIES` - Comma-separated list of trusted proxy IPs/CIDRs

## Dependencies

- Gin v1.12 - HTTP framework
- GORM v1.31 - ORM
- Redis v8 - Cache
- JWT v5 - Authentication
- Viper v1.21 - Config
- Validator v10 - Request validation
- Swaggo v1.16 - Swagger documentation
- Gin-Swagger v1.6 - Swagger UI middleware

## Database Migrations

Manual SQL migrations in `migrations/` directory. Run `migrations/001_init.sql` to create initial schema.
