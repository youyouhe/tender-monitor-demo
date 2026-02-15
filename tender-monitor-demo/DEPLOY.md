# 部署到免费网站指南

## 🚀 方案一：Railway.app（推荐）

### 优势
- ✅ 完全免费（每月 $5 额度，足够个人使用）
- ✅ 支持 Go 语言
- ✅ 自动 HTTPS
- ✅ GitHub 集成
- ✅ 自动重启

### 部署步骤

#### 1. 准备 GitHub 仓库

```bash
cd /workspace/group/tender-monitor

# 初始化 Git
git init
git add .
git commit -m "Initial commit: 招标监控系统"

# 推送到 GitHub（需要先创建仓库）
git remote add origin https://github.com/你的用户名/tender-monitor.git
git branch -M main
git push -u origin main
```

#### 2. 部署到 Railway

1. 访问 https://railway.app/
2. 使用 GitHub 账号登录
3. 点击 "New Project"
4. 选择 "Deploy from GitHub repo"
5. 选择你的 `tender-monitor` 仓库
6. Railway 会自动检测 Go 项目并部署

#### 3. 配置环境变量（可选）

在 Railway 项目设置中添加：
- `PORT=8080`
- `BROWSER_HEADLESS=true`

#### 4. 获取访问地址

部署完成后，Railway 会提供一个 URL：
`https://你的项目名.railway.app`

### 限制

⚠️ **免费版限制：**
- Railway 免费版没有浏览器支持（无法运行 Rod）
- **解决方案：** 需要将爬虫功能改为后台 API 调用

---

## 🚀 方案二：Render.com

### 优势
- ✅ 完全免费（静态站点 + Web 服务）
- ✅ 自动 HTTPS
- ✅ GitHub 集成

### 部署步骤

1. 访问 https://render.com/
2. 使用 GitHub 登录
3. 点击 "New +"
4. 选择 "Web Service"
5. 连接 GitHub 仓库
6. 配置：
   - **Name:** tender-monitor
   - **Build Command:** `go build -o tender-monitor main.go`
   - **Start Command:** `./tender-monitor`
7. 点击 "Create Web Service"

### 限制

⚠️ **免费版限制：**
- 15 分钟无活动会休眠
- 无浏览器环境

---

## 🚀 方案三：Fly.io

### 优势
- ✅ 免费额度充足
- ✅ 支持 Docker
- ✅ 全球 CDN

### 部署步骤

#### 1. 安装 Fly CLI

```bash
curl -L https://fly.io/install.sh | sh
```

#### 2. 登录

```bash
flyctl auth login
```

#### 3. 创建 Dockerfile

```bash
# 已创建 Dockerfile（见下方）
```

#### 4. 初始化并部署

```bash
flyctl launch
flyctl deploy
```

---

## 📦 为云部署优化的 Dockerfile

由于云平台限制，我为你创建了优化版本：

### 方案：仅部署前端 + API（不含浏览器）

**适用场景：** 查看已采集的数据，不执行新采集

---

## 🏠 方案四：本地部署 + 内网穿透（最完整）

### 使用 Cloudflare Tunnel（免费）

#### 1. 安装 cloudflared

```bash
# macOS
brew install cloudflare/cloudflare/cloudflared

# Linux
wget https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64
chmod +x cloudflared-linux-amd64
sudo mv cloudflared-linux-amd64 /usr/local/bin/cloudflared
```

#### 2. 启动本地服务

```bash
./deploy.sh start
```

#### 3. 创建隧道

```bash
cloudflared tunnel --url http://localhost:8080
```

你会得到一个公网 URL：
`https://随机字符串.trycloudflare.com`

**优势：**
- ✅ 完全免费
- ✅ 不需要公网 IP
- ✅ 支持完整功能（包括浏览器）
- ✅ 自动 HTTPS

---

## 🌐 方案五：Vercel（仅前端）

### 部署纯静态前端

如果只想展示界面（不含后端功能）：

```bash
# 将 static/index.html 部署到 Vercel
vercel --prod
```

---

## 💡 推荐方案对比

| 方案 | 费用 | 完整功能 | 难度 | 推荐度 |
|-----|------|---------|------|--------|
| **Cloudflare Tunnel** | 免费 | ✅ 完整 | 简单 | ⭐⭐⭐⭐⭐ |
| Railway.app | 免费 | ❌ 无浏览器 | 简单 | ⭐⭐⭐ |
| Fly.io | 免费 | ❌ 无浏览器 | 中等 | ⭐⭐⭐ |
| Render.com | 免费 | ❌ 无浏览器 | 简单 | ⭐⭐ |
| Vercel | 免费 | ❌ 仅前端 | 简单 | ⭐⭐ |

---

## 🎯 我的建议

### 如果你想要完整功能（采集 + 展示）

**使用：Cloudflare Tunnel + 本地运行**

```bash
# 1. 启动服务
./deploy.sh start

# 2. 开启隧道
cloudflared tunnel --url http://localhost:8080

# 3. 访问公网 URL
```

### 如果只想展示已采集的数据

**使用：Railway.app 或 Render.com**

需要修改代码，禁用采集功能，只保留查询和展示。

---

## 🔧 快速测试

### 本地测试（立即可用）

```bash
cd /workspace/group/tender-monitor

# 快速编译运行
go run main.go
```

然后在浏览器打开：**http://localhost:8080**

---

需要我帮你配置哪种部署方案？我可以：

1. **创建 Cloudflare Tunnel 配置**（推荐，完整功能）
2. **优化代码用于 Railway/Render 部署**（仅展示功能）
3. **创建 Docker 配置用于 Fly.io**
4. **先本地测试看看效果**
