# Docker 部署指南

## 🐳 使用 Docker 一键部署

### 前提条件

确保已安装 Docker 和 Docker Compose：
```bash
docker --version
docker-compose --version
```

如果没有安装：
```bash
# Ubuntu/Debian
curl -fsSL https://get.docker.com | sh
sudo apt-get install docker-compose-plugin

# macOS
brew install docker docker-compose
```

---

## 🚀 快速启动

### 方式 1：使用 Docker Compose（推荐）

```bash
# 1. 克隆项目
git clone https://github.com/youyouhe/tender-monitor-demo.git
cd tender-monitor-demo

# 2. 构建并启动
docker-compose up -d

# 3. 查看日志
docker-compose logs -f

# 4. 访问
打开浏览器：http://localhost:8080
```

**就这么简单！** 🎉

---

### 方式 2：使用 Docker 命令

```bash
# 构建镜像
docker build -t tender-monitor .

# 运行容器
docker run -d \
  --name tender-monitor \
  -p 8080:8080 \
  -p 5000:5000 \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/logs:/app/logs \
  tender-monitor

# 查看日志
docker logs -f tender-monitor
```

---

## 📊 服务管理

### 查看状态

```bash
docker-compose ps
```

### 停止服务

```bash
docker-compose stop
```

### 重启服务

```bash
docker-compose restart
```

### 停止并删除

```bash
docker-compose down
```

### 查看日志

```bash
# 实时日志
docker-compose logs -f

# 最近100行
docker-compose logs --tail=100

# 只看主程序
docker-compose logs tender-monitor

# 只看错误
docker-compose logs | grep ERROR
```

---

## 🔧 配置说明

### 环境变量

在 `docker-compose.yml` 中可以配置：

```yaml
environment:
  - BROWSER_HEADLESS=true          # 无头模式（必须）
  - CAPTCHA_SERVICE=http://localhost:5000  # 验证码服务地址
  - DATA_DIR=/app/data             # 数据目录
  - TRACES_DIR=/app/traces         # 轨迹目录
```

### 端口映射

```yaml
ports:
  - "8080:8080"   # Web界面 - 可以改成其他端口
  - "5000:5000"   # 验证码服务
```

如果 8080 端口被占用，可以改成：
```yaml
ports:
  - "9090:8080"   # 访问 http://localhost:9090
  - "5000:5000"
```

### 数据持久化

```yaml
volumes:
  - ./data:/app/data      # 数据库
  - ./logs:/app/logs      # 日志
  - ./traces:/app/traces  # 轨迹文件
```

这样即使容器删除，数据也不会丢失。

---

## 🌐 部署到公网服务器

### 1. 准备服务器

推荐配置：
- CPU: 2核
- 内存: 2GB+
- 系统: Ubuntu 20.04/22.04
- 费用: 约 20-30 元/月（阿里云/腾讯云轻量应用服务器）

### 2. 安装 Docker

```bash
# SSH 登录服务器
ssh root@your-server-ip

# 安装 Docker
curl -fsSL https://get.docker.com | sh

# 启动 Docker
systemctl start docker
systemctl enable docker
```

### 3. 部署项目

```bash
# 克隆项目
git clone https://github.com/youyouhe/tender-monitor-demo.git
cd tender-monitor-demo

# 启动服务
docker-compose up -d

# 查看日志
docker-compose logs -f
```

### 4. 配置防火墙

```bash
# 开放端口
ufw allow 8080
ufw allow 5000  # 可选，如果只内部调用可以不开

# 或者只允许特定IP访问
ufw allow from YOUR_IP to any port 8080
```

### 5. 访问

```
http://your-server-ip:8080
```

---

## 🔒 安全建议

### 1. 使用 Nginx 反向代理

```bash
# 安装 Nginx
apt-get install nginx

# 配置文件：/etc/nginx/sites-available/tender-monitor
server {
    listen 80;
    server_name your-domain.com;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}

# 启用配置
ln -s /etc/nginx/sites-available/tender-monitor /etc/nginx/sites-enabled/
nginx -t
systemctl reload nginx
```

### 2. 配置 HTTPS

```bash
# 安装 Certbot
apt-get install certbot python3-certbot-nginx

# 获取证书
certbot --nginx -d your-domain.com

# 自动续期
certbot renew --dry-run
```

### 3. 限制访问

在 `docker-compose.yml` 中添加：

```yaml
environment:
  - ALLOWED_IPS=192.168.1.100,203.0.113.0/24
```

---

## 🔧 故障排查

### 容器无法启动

```bash
# 查看详细错误
docker-compose logs

# 检查端口占用
netstat -tulpn | grep 8080

# 重新构建
docker-compose down
docker-compose build --no-cache
docker-compose up -d
```

### 浏览器无法启动

**现象：** `browser not found`

**解决：** 确保 Dockerfile 中安装了 chromium

### 验证码服务失败

```bash
# 进入容器
docker-compose exec tender-monitor bash

# 手动测试验证码服务
curl http://localhost:5000/health

# 查看 Python 依赖
pip3 list | grep ddddocr
```

### 数据库锁定

```bash
# 停止容器
docker-compose stop

# 删除锁文件
rm data/tenders.db-journal

# 重启
docker-compose start
```

---

## 📊 监控和维护

### 查看资源使用

```bash
# 查看容器资源
docker stats tender-monitor

# 查看磁盘使用
du -sh data/ logs/
```

### 定期备份

```bash
# 备份数据库
cp data/tenders.db backups/tenders-$(date +%Y%m%d).db

# 自动备份脚本
cat > /etc/cron.daily/backup-tender << 'EOF'
#!/bin/bash
cd /path/to/tender-monitor-demo
cp data/tenders.db backups/tenders-$(date +%Y%m%d).db
find backups/ -mtime +30 -delete
EOF

chmod +x /etc/cron.daily/backup-tender
```

### 更新项目

```bash
# 拉取最新代码
git pull

# 重新构建
docker-compose down
docker-compose build
docker-compose up -d
```

---

## 🎯 性能优化

### 限制资源使用

在 `docker-compose.yml` 中添加：

```yaml
deploy:
  resources:
    limits:
      cpus: '2'
      memory: 2G
    reservations:
      cpus: '1'
      memory: 512M
```

### 日志轮转

```yaml
logging:
  driver: "json-file"
  options:
    max-size: "10m"
    max-file: "3"
```

---

## 💰 成本估算

### 阿里云轻量应用服务器

- **2核2G配置**：约 30 元/月
- **流量**：1TB/月（足够）
- **带宽**：3-5 Mbps

### 腾讯云轻量应用服务器

- **2核2G配置**：约 25 元/月
- **流量**：500GB/月
- **带宽**：4 Mbps

### AWS/DigitalOcean

- **基础配置**：约 $5-10/月（35-70 元）

---

## 🎉 完成！

部署成功后，你将拥有：

- ✅ 完整的招标监控系统
- ✅ 自动验证码识别
- ✅ 美观的 Web 界面
- ✅ 数据持久化存储
- ✅ 随时可访问的公网服务

访问：`http://your-server-ip:8080` 或 `https://your-domain.com`

---

**需要帮助？** 随时找我！ 😊
