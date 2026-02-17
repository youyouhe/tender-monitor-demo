# 远程部署指南 - 分离式架构

## 🎯 架构说明

```
┌─────────────────┐         HTTP          ┌─────────────────┐
│   Windows PC    │ ─────────────────────>│  Linux Server   │
│                 │                        │                 │
│  Go 主程序       │ POST /ocr (验证码)    │  Captcha 服务    │
│  (浏览器爬虫)    │ <─────────────────────│  (FastAPI)      │
│  SQLite 数据库   │    识别结果返回        │  ddddocr/Qwen   │
└─────────────────┘                        └─────────────────┘
```

**优势：**
- Windows 运行浏览器，GUI 支持好
- Linux 服务器运行验证码服务，资源利用率高
- 验证码服务可以被多个客户端共享
- 可以部署 GPU 版本的 Qwen2-VL（如果服务器有显卡）

---

## 📋 一、服务器端部署（Linux）

### 1.1 准备环境

```bash
# 更新系统
sudo apt update && sudo apt upgrade -y

# 安装 Python 3.10+
python3 --version  # 确保 >= 3.10

# 安装依赖
sudo apt install -y python3-pip python3-venv
```

### 1.2 部署验证码服务

```bash
# 1. 克隆或上传项目
git clone https://github.com/youyouhe/tender-monitor-demo.git
cd tender-monitor-demo/captcha-service

# 2. 创建虚拟环境
python3 -m venv venv
source venv/bin/activate

# 3. 安装依赖（基础版：仅 ddddocr）
pip install -r requirements.txt

# 4. (可选) 如果服务器有 GPU，安装 Qwen2-VL
# pip install transformers torch torchvision pillow qwen-vl-utils
# export HF_ENDPOINT=https://hf-mirror.com  # 使用国内镜像

# 5. 测试服务
python3 test_captcha.py
```

### 1.3 启动服务（生产模式）

**方式 1：直接启动（测试用）**
```bash
uvicorn app:app --host 0.0.0.0 --port 5000
```

**方式 2：使用 systemd（推荐生产环境）**

创建服务文件：
```bash
sudo nano /etc/systemd/system/captcha-service.service
```

内容：
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

启动服务：
```bash
sudo systemctl daemon-reload
sudo systemctl enable captcha-service
sudo systemctl start captcha-service
sudo systemctl status captcha-service
```

**方式 3：使用 Docker（最简单）**

```bash
cd captcha-service
docker-compose up -d
```

### 1.4 配置防火墙

```bash
# 开放 5000 端口
sudo ufw allow 5000/tcp
sudo ufw reload

# 或者使用 iptables
sudo iptables -A INPUT -p tcp --dport 5000 -j ACCEPT
```

### 1.5 测试服务可访问性

```bash
# 在服务器上测试
curl http://localhost:5000/health

# 在外部测试（替换为你的服务器 IP）
curl http://YOUR_SERVER_IP:5000/health
```

---

## 💻 二、Windows 客户端配置

### 2.1 克隆项目

```powershell
# 使用 Git Bash 或 PowerShell
git clone https://github.com/youyouhe/tender-monitor-demo.git
cd tender-monitor-demo
```

### 2.2 配置环境变量

**方式 1：PowerShell（临时）**
```powershell
$env:CAPTCHA_SERVICE = "http://YOUR_SERVER_IP:5000"
$env:BROWSER_HEADLESS = "false"
go run main.go
```

**方式 2：创建启动脚本 `start.bat`**
```batch
@echo off
set CAPTCHA_SERVICE=http://YOUR_SERVER_IP:5000
set BROWSER_HEADLESS=false
set DATA_DIR=./data
set TRACES_DIR=./traces

echo ========================================
echo 招标信息监控系统 - Windows 客户端
echo ========================================
echo.
echo 验证码服务: %CAPTCHA_SERVICE%
echo 浏览器模式: 有头模式 (可见窗口)
echo.

go run main.go
```

**方式 3：创建 `.env` 文件（需要修改代码加载）**
```env
CAPTCHA_SERVICE=http://YOUR_SERVER_IP:5000
BROWSER_HEADLESS=false
DATA_DIR=./data
TRACES_DIR=./traces
```

### 2.3 测试连接

```powershell
# 测试能否访问验证码服务
curl http://YOUR_SERVER_IP:5000/health
```

### 2.4 运行程序

```powershell
# 直接运行
start.bat

# 或者在 PowerShell 中
$env:CAPTCHA_SERVICE = "http://YOUR_SERVER_IP:5000"
go run main.go
```

### 2.5 访问 Web 界面

打开浏览器访问：`http://localhost:8080`

---

## 🔧 三、配置参数说明

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `CAPTCHA_SERVICE` | `http://localhost:5000` | 验证码服务地址（**必须修改**） |
| `DATA_DIR` | `./data` | 数据目录（数据库、截图等） |
| `TRACES_DIR` | `./traces` | 轨迹文件目录 |
| `BROWSER_HEADLESS` | `false` | 是否无头模式（Windows 建议 false） |

**重要：** 将 `YOUR_SERVER_IP` 替换为你的实际服务器 IP 或域名！

---

## 🧪 四、测试流程

### 4.1 服务器端测试

```bash
# 1. 测试健康检查
curl http://localhost:5000/health

# 2. 测试验证码识别（需要准备测试图片）
curl -X POST http://localhost:5000/ocr \
  -F "file=@test_captcha.png" \
  -H "Content-Type: multipart/form-data"
```

### 4.2 Windows 端测试

```powershell
# 1. 测试主服务健康检查
curl http://localhost:8080/api/health

# 2. 测试采集（山东省）
curl -X POST http://localhost:8080/api/collect `
  -H "Content-Type: application/json" `
  -d '{"province":"shandong","keywords":["软件"]}'

# 3. 测试采集（广东省）
curl -X POST http://localhost:8080/api/collect `
  -H "Content-Type: application/json" `
  -d '{"province":"guangdong","keywords":["软件"]}'

# 4. 查询结果
curl "http://localhost:8080/api/tenders?province=guangdong&keyword=软件"
```

---

## 🐛 五、故障排查

### 5.1 验证码服务无法访问

**检查清单：**
```bash
# 1. 服务是否运行
sudo systemctl status captcha-service

# 2. 端口是否监听
sudo netstat -tlnp | grep 5000

# 3. 防火墙是否开放
sudo ufw status

# 4. 服务器 IP 是否正确
ip addr show
```

**常见问题：**
- ❌ 防火墙未开放 5000 端口
- ❌ 云服务器安全组未配置
- ❌ 服务未绑定 `0.0.0.0`（只监听 localhost）
- ❌ Windows 客户端防火墙阻止出站连接

### 5.2 Windows 浏览器无法启动

**解决方案：**
1. 确保 Windows 有足够磁盘空间
2. 检查是否安装了 Chrome 或 Chromium
3. 尝试删除 `./data/browser-data` 目录
4. 设置 `BROWSER_HEADLESS=false` 查看错误信息

### 5.3 采集失败：连接被拒绝

**可能原因：**
- 目标网站检测到自动化工具
- IP 被封禁
- 网站需要登录或特殊权限
- 轨迹文件选择器已过期（网站改版）

---

## 🚀 六、高级配置

### 6.1 使用域名（推荐）

**服务器端（Nginx 反向代理）：**
```nginx
server {
    listen 80;
    server_name captcha.yourdomain.com;

    location / {
        proxy_pass http://127.0.0.1:5000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

**Windows 配置：**
```batch
set CAPTCHA_SERVICE=http://captcha.yourdomain.com
```

### 6.2 HTTPS 配置（使用 Let's Encrypt）

```bash
sudo apt install certbot python3-certbot-nginx
sudo certbot --nginx -d captcha.yourdomain.com
```

**Windows 配置：**
```batch
set CAPTCHA_SERVICE=https://captcha.yourdomain.com
```

### 6.3 多服务器负载均衡

如果验证码服务压力大，可以部署多个实例：

```batch
REM Windows 轮询使用不同服务器
set CAPTCHA_SERVICE=http://server1.example.com:5000
REM 或
set CAPTCHA_SERVICE=http://server2.example.com:5000
```

---

## 📊 七、性能优化

### 7.1 服务器端

```bash
# 增加 uvicorn workers（多核 CPU）
uvicorn app:app --host 0.0.0.0 --port 5000 --workers 4

# 使用 gunicorn（更强的并发）
pip install gunicorn
gunicorn -w 4 -k uvicorn.workers.UvicornWorker app:app --bind 0.0.0.0:5000
```

### 7.2 Windows 端

```powershell
# 多个采集任务并行（启动多个实例）
# 实例 1 - 端口 8080
go run main.go

# 实例 2 - 端口 8081（需要修改代码支持端口配置）
# $env:PORT = "8081"
# go run main.go
```

---

## 📝 八、监控和日志

### 8.1 服务器日志

```bash
# systemd 日志
sudo journalctl -u captcha-service -f

# 应用日志（如果写入文件）
tail -f /path/to/captcha-service/logs/app.log
```

### 8.2 Windows 日志

```powershell
# 重定向输出到文件
go run main.go > logs.txt 2>&1
```

---

## 🔒 九、安全建议

1. ⚠️ **不要直接暴露验证码服务到公网**
   - 使用 VPN 或内网穿透
   - 或者配置 API Key 认证

2. ⚠️ **定期更新依赖**
   ```bash
   pip install --upgrade -r requirements.txt
   ```

3. ⚠️ **限制访问 IP**
   ```bash
   # 只允许特定 IP 访问
   sudo ufw allow from YOUR_WINDOWS_IP to any port 5000
   ```

---

## 📞 十、支持

遇到问题请检查：
1. GitHub Issues: https://github.com/youyouhe/tender-monitor-demo/issues
2. 服务器端日志：`sudo journalctl -u captcha-service -f`
3. Windows 端日志：查看控制台输出

---

**版本：** 1.0.0
**最后更新：** 2026-02-18
