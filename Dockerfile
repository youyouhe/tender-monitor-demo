FROM golang:1.21-bullseye AS builder

# 安装浏览器依赖
RUN apt-get update && apt-get install -y \
    chromium \
    chromium-driver \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# 复制 Go 模块文件
COPY go.mod ./
RUN go mod download

# 复制源码
COPY main.go convert_trace.go ./
COPY static/ ./static/
COPY traces/ ./traces/

# 编译
RUN CGO_ENABLED=1 GOOS=linux go build -o tender-monitor main.go

# 最终镜像
FROM debian:bullseye-slim

# 安装运行时依赖
RUN apt-get update && apt-get install -y \
    chromium \
    ca-certificates \
    python3 \
    python3-pip \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# 复制编译好的程序
COPY --from=builder /app/tender-monitor .
COPY --from=builder /app/static ./static
COPY --from=builder /app/traces ./traces

# 复制验证码服务
COPY captcha-service/ ./captcha-service/

# 安装 Python 依赖
RUN pip3 install --no-cache-dir -r captcha-service/requirements.txt

# 创建数据目录
RUN mkdir -p data logs

# 暴露端口
EXPOSE 8080 5000

# 启动脚本
COPY docker-entrypoint.sh /
RUN chmod +x /docker-entrypoint.sh

ENTRYPOINT ["/docker-entrypoint.sh"]
