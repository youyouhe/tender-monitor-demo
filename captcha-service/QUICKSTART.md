# 快速开始 - 验证码服务部署

## 🚀 一键部署（推荐）

```bash
# 1. 进入目录
cd captcha-service

# 2. 安装依赖（仅首次）
./start-server.sh install

# 3. 启动服务（后台运行）
./start-server.sh daemon

# 4. 测试服务
curl http://localhost:5000/health
```

就这么简单！服务已启动在 `http://0.0.0.0:5000`

---

## 📋 常用命令

```bash
# 查看帮助
./start-server.sh help

# 前台启动（查看实时日志）
./start-server.sh start

# 后台启动
./start-server.sh daemon

# 停止服务
./start-server.sh stop

# 重启服务
./start-server.sh restart

# 查看状态
./start-server.sh status

# 查看日志
./start-server.sh logs

# 测试服务
./start-server.sh test
```

---

## ⚙️ 高级选项

### 自定义端口

```bash
./start-server.sh daemon --port=8000
```

### 多进程模式（高并发）

```bash
./start-server.sh daemon --workers=4
```

### 组合使用

```bash
./start-server.sh daemon --port=8000 --workers=4
```

---

## 🔍 验证部署

### 1. 健康检查

```bash
curl http://localhost:5000/health
```

**期望输出：**
```json
{
  "status": "ok",
  "service": "captcha-ocr",
  "version": "2.0.0",
  "engines": {
    "ddddocr": {
      "engine": "ddddocr",
      "available": true
    },
    "qwen": {
      "engine": "qwen",
      "available": false
    }
  }
}
```

### 2. 测试识别（如果有测试图片）

```bash
curl -X POST http://localhost:5000/ocr \
  -F "file=@test_image.png"
```

---

## 🔧 systemd 服务（生产环境）

如果需要开机自启和自动重启，配置 systemd：

### 1. 创建服务文件

```bash
sudo nano /etc/systemd/system/captcha-service.service
```

### 2. 写入配置（替换路径和用户名）

```ini
[Unit]
Description=Captcha Recognition Service
After=network.target

[Service]
Type=simple
User=YOUR_USERNAME
WorkingDirectory=/path/to/tender-monitor-demo/captcha-service
Environment="PATH=/path/to/tender-monitor-demo/captcha-service/venv/bin"
ExecStart=/path/to/tender-monitor-demo/captcha-service/venv/bin/uvicorn app:app --host 0.0.0.0 --port 5000
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

### 3. 启用并启动

```bash
sudo systemctl daemon-reload
sudo systemctl enable captcha-service
sudo systemctl start captcha-service
sudo systemctl status captcha-service
```

### 4. 管理服务

```bash
# 启动
sudo systemctl start captcha-service

# 停止
sudo systemctl stop captcha-service

# 重启
sudo systemctl restart captcha-service

# 查看状态
sudo systemctl status captcha-service

# 查看日志
sudo journalctl -u captcha-service -f
```

---

## 🐳 Docker 部署（可选）

### 基础版（ddddocr）

```bash
docker-compose up -d
```

### Qwen2-VL 版（需要 GPU）

```bash
docker-compose -f docker-compose.qwen.yml up -d
```

---

## 🌐 外网访问配置

### 1. 防火墙开放端口

**UFW（Ubuntu/Debian）：**
```bash
sudo ufw allow 5000/tcp
sudo ufw reload
```

**Firewalld（CentOS/RHEL）：**
```bash
sudo firewall-cmd --permanent --add-port=5000/tcp
sudo firewall-cmd --reload
```

**iptables：**
```bash
sudo iptables -A INPUT -p tcp --dport 5000 -j ACCEPT
```

### 2. 云服务器安全组

如果是阿里云、腾讯云、AWS 等，需要在控制台配置：
- 入站规则
- 协议：TCP
- 端口：5000
- 源地址：0.0.0.0/0 或指定 IP

---

## 🧪 验证远程访问

从 Windows 或其他机器测试：

```powershell
# 替换 YOUR_SERVER_IP 为实际服务器 IP
curl http://YOUR_SERVER_IP:5000/health
```

---

## 📊 监控和日志

### 查看实时日志

```bash
./start-server.sh logs
```

### 查看历史日志

```bash
cat logs/captcha-service.log
```

### 查看系统资源占用

```bash
# 查看进程
ps aux | grep uvicorn

# 查看端口
sudo netstat -tlnp | grep 5000

# 查看资源占用
top -p $(cat /tmp/captcha-service.pid)
```

---

## 🐛 故障排查

### 服务启动失败

```bash
# 1. 查看详细错误
cat logs/captcha-service.log

# 2. 前台启动查看错误
./start-server.sh start

# 3. 检查依赖
source venv/bin/activate
pip list | grep -E "fastapi|uvicorn|ddddocr"
```

### 端口被占用

```bash
# 查看占用进程
sudo lsof -i :5000

# 结束进程
sudo kill -9 PID

# 或更换端口
./start-server.sh daemon --port=5001
```

### 无法访问服务

```bash
# 1. 检查服务状态
./start-server.sh status

# 2. 检查防火墙
sudo ufw status

# 3. 检查监听地址
sudo netstat -tlnp | grep 5000

# 4. 测试本地访问
curl http://localhost:5000/health

# 5. 测试远程访问（从服务器）
curl http://0.0.0.0:5000/health
```

---

## 🔄 更新服务

```bash
# 1. 停止服务
./start-server.sh stop

# 2. 更新代码
git pull

# 3. 更新依赖
source venv/bin/activate
pip install -r requirements.txt --upgrade

# 4. 重启服务
./start-server.sh daemon
```

---

## 📚 相关文档

- [API 接口文档](./API.md)
- [远程部署指南](../REMOTE_DEPLOY.md)
- [Qwen2-VL 部署](./README_QWEN.md)

---

**版本：** 1.0.0
**最后更新：** 2026-02-18
