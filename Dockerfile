# 多阶段构建 - 构建阶段
FROM golang:1.25-alpine AS builder

WORKDIR /app

# 安装必要的工具
RUN apk add --no-cache git

# 复制 go.mod 和 go.sum
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 编译应用
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd/main.go

# 运行阶段
FROM alpine:latest

WORKDIR /root/

# 安装 ca 证书（用于 HTTPS 请求）
RUN apk --no-cache add ca-certificates tzdata

# 从构建阶段复制二进制文件
COPY --from=builder /app/main .

# 暴露端口
EXPOSE 8080

# 启动应用
CMD ["./main"]
