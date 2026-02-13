# 招标信息监控系统

基于 **Go + Rod** 的极简政府招标信息自动采集系统。

## 🚀 一键部署到 Vercel

[![Deploy with Vercel](https://vercel.com/button)](https://vercel.com/new/clone?repository-url=https://github.com/youyouhe/tender-monitor-demo)

## 🎯 项目特点

- ✅ **极简架构** - 单文件Go程序 + SQLite数据库
- ✅ **轨迹驱动** - 使用Chrome Recorder录制操作，自动生成采集程序
- ✅ **智能验证码** - ddddocr自动识别 + 手动输入降级
- ✅ **两阶段采集** - 先列表后详情，按需采集
- ✅ **美观界面** - 原生HTML/JS，无需框架
- ✅ **一键部署** - 提供完整部署脚本

## 📁 目录结构

```
tender-monitor/
├── main.go                    # 主程序（爬虫+API+Web）
├── convert_trace.go           # 轨迹文件转换工具
├── deploy.sh                  # 部署脚本
├── README.md                  # 本文件
├── captcha-service/           # 验证码识别服务
│   ├── captcha_service.py     # Flask服务
│   ├── requirements.txt       # Python依赖
│   ├── Dockerfile             # Docker镜像
│   ├── docker-compose.yml     # Docker编排
│   ├── test_captcha.py        # 测试脚本
│   └── README.md              # 服务文档
├── static/
│   └── index.html             # Web界面
├── traces/                    # 轨迹文件目录
│   ├── shandong_list.json     # 山东省列表轨迹
│   ├── shandong_detail.json   # 山东省详情轨迹
│   └── ...                    # 其他省份
├── data/
│   └── tenders.db             # SQLite数据库
└── logs/                      # 日志文件
```

## 🚀 快速开始

### 方式一：使用部署脚本（推荐）

```bash
# 赋予执行权限
chmod +x deploy.sh

# 首次部署
./deploy.sh install

# 启动服务
./deploy.sh start

# 查看状态
./deploy.sh status

# 查看日志
./deploy.sh logs
```

### 方式二：手动部署

#### 1. 安装依赖

```bash
# Go 依赖
go mod init tender-monitor
go get github.com/go-rod/rod
go get github.com/mattn/go-sqlite3

# Python 依赖
cd captcha-service
pip install -r requirements.txt
cd ..
```

#### 2. 启动验证码服务

```bash
cd captcha-service
python captcha_service.py
# 或使用 Docker
docker-compose up -d
cd ..
```

#### 3. 编译并运行主程序

```bash
go build -o tender-monitor main.go
./tender-monitor
```

#### 4. 访问系统

打开浏览器访问：http://localhost:8080

## 📝 使用轨迹文件

### 录制轨迹

1. 打开 Chrome 浏览器
2. 打开开发者工具（F12）
3. 切换到 "Recorder" 标签页
4. 点击 "开始录制"
5. 执行采集操作：
   - **列表页轨迹**：搜索 → 输入验证码 → 点击查询
   - **详情页轨迹**：点击第一条记录 → 查看详情
6. 停止录制并导出 JSON 文件

### 转换轨迹

使用转换工具将 Chrome Recorder 格式转换为简化格式：

```bash
# 转换列表页轨迹
go run convert_trace.go recording_list.json list traces/province_list.json

# 转换详情页轨迹
go run convert_trace.go recording_detail.json detail traces/province_detail.json
```

### 轨迹文件格式

#### 列表页轨迹示例

```json
{
  "name": "山东省政府采购网-列表",
  "type": "list",
  "url": "http://www.ccgp-shandong.gov.cn/home",
  "steps": [
    {
      "action": "navigate",
      "url": "http://www.ccgp-shandong.gov.cn/home"
    },
    {
      "action": "click",
      "selector": "text/采购公告"
    },
    {
      "action": "input",
      "selector": "input[placeholder*='公告标题']",
      "value": "{{.Keyword}}"
    },
    {
      "action": "captcha",
      "image_selector": "img[src*='captcha']",
      "input_selector": "input[placeholder*='验证码']"
    },
    {
      "action": "click",
      "selector": "button:has(span:text('查询'))"
    },
    {
      "action": "extract",
      "type": "list",
      "selector": "tbody tr",
      "fields": {
        "title": "td:nth-child(3) span",
        "date": "td:nth-child(4)",
        "url": "td:nth-child(3) span"
      }
    }
  ]
}
```

#### 详情页轨迹示例

```json
{
  "name": "山东省政府采购网-详情",
  "type": "detail",
  "url": "{{.URL}}",
  "steps": [
    {
      "action": "navigate",
      "url": "{{.URL}}"
    },
    {
      "action": "extract",
      "type": "detail",
      "fields": {
        "amount": "span:contains('预算金额')",
        "contact": "span:contains('联系人')",
        "phone": "span:contains('联系电话')"
      }
    }
  ]
}
```

## 🔧 配置说明

### 环境变量

```bash
# 数据目录
DATA_DIR=./data

# 轨迹文件目录
TRACES_DIR=./traces

# 验证码服务地址
CAPTCHA_SERVICE=http://localhost:5000

# 浏览器无头模式（true/false）
BROWSER_HEADLESS=false
```

### 数据库结构

```sql
CREATE TABLE tenders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    province TEXT,              -- 省份
    title TEXT,                 -- 标题
    amount TEXT,                -- 预算金额
    publish_date TEXT,          -- 发布日期
    contact TEXT,               -- 联系人
    phone TEXT,                 -- 联系电话
    url TEXT UNIQUE,            -- 详情链接
    keywords TEXT,              -- 匹配关键词
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

## 📡 API 接口

### 1. 健康检查

```bash
GET /api/health
```

**响应：**
```json
{
  "status": "ok",
  "service": "tender-monitor",
  "version": "1.0.0"
}
```

### 2. 查询招标信息

```bash
GET /api/tenders?province=shandong&keyword=软件
```

**参数：**
- `province` - 省份（可选）
- `keyword` - 关键词（可选）

**响应：**
```json
{
  "success": true,
  "count": 10,
  "data": [
    {
      "id": 1,
      "province": "shandong",
      "title": "某市软件采购项目",
      "amount": "50万元",
      "publish_date": "2026-02-13",
      "contact": "张三",
      "phone": "0531-12345678",
      "url": "http://...",
      "keywords": "软件",
      "created_at": "2026-02-13T20:00:00Z"
    }
  ]
}
```

### 3. 启动采集任务

```bash
POST /api/collect
Content-Type: application/json

{
  "province": "shandong",
  "keywords": ["软件", "软件开发", "信息化"]
}
```

**响应：**
```json
{
  "success": true,
  "message": "采集任务已启动"
}
```

## 🧪 测试

### 测试验证码服务

```bash
cd captcha-service

# 健康检查
python test_captcha.py

# 识别测试（需要验证码图片）
python test_captcha.py captcha.png
```

### 测试主程序

```bash
# 编译
go build -o tender-monitor main.go

# 运行
./tender-monitor

# 测试API
curl http://localhost:8080/api/health
curl http://localhost:8080/api/tenders
```

## 📊 工作流程

### 采集流程

```
1. 用户触发采集任务
   ↓
2. 加载省份的轨迹文件（list + detail）
   ↓
3. 启动浏览器
   ↓
4. 阶段1：列表采集
   - 循环遍历关键词
   - 执行列表轨迹（导航 → 搜索 → 验证码 → 查询）
   - 提取列表数据（标题、日期、链接）
   ↓
5. 阶段2：详情采集
   - 筛选匹配关键词的项目
   - 执行详情轨迹（点击 → 提取详情）
   - 获取完整信息（预算、联系方式等）
   ↓
6. 保存到数据库（自动去重）
   ↓
7. 关闭浏览器
```

### 验证码处理

```
1. 截取验证码图片
   ↓
2. 保存到本地（便于调试）
   ↓
3. 调用验证码服务识别
   ↓
4. 识别成功？
   ├─ 是 → 自动输入
   └─ 否 → 降级到手动输入
```

## ⚙️ 服务管理

### 启动服务

```bash
./deploy.sh start
```

### 停止服务

```bash
./deploy.sh stop
```

### 重启服务

```bash
./deploy.sh restart
```

### 查看状态

```bash
./deploy.sh status
```

### 查看日志

```bash
# 主程序日志
tail -f logs/tender-monitor.log

# 验证码服务日志
tail -f logs/captcha.log

# 使用部署脚本
./deploy.sh logs
```

## 🐛 故障排查

### 验证码服务无法启动

```bash
# 检查端口占用
lsof -i :5000

# 查看日志
tail -f logs/captcha.log

# 手动启动测试
cd captcha-service
python captcha_service.py
```

### 主程序无法启动

```bash
# 检查端口占用
lsof -i :8080

# 查看日志
tail -f logs/tender-monitor.log

# 检查数据库
sqlite3 data/tenders.db "SELECT count(*) FROM tenders;"
```

### 浏览器无法启动

```bash
# 检查 Chrome/Chromium 是否已安装
which google-chrome
which chromium

# 安装浏览器（Ubuntu/Debian）
sudo apt-get install chromium-browser

# 安装浏览器（macOS）
brew install --cask google-chrome
```

### 验证码识别率低

1. 检查图片质量：查看 `data/captcha_*.png`
2. 考虑使用付费API（阿里云、腾讯云）
3. 手动输入降级（已内置）

## 🚀 扩展功能

### 添加新省份

1. 录制该省份的轨迹文件（list + detail）
2. 使用 `convert_trace.go` 转换格式
3. 保存到 `traces/` 目录
4. 在 Web 界面中添加该省份选项

### 定时采集

使用 cron 定时任务：

```bash
# 每天凌晨2点采集山东省
0 2 * * * cd /path/to/tender-monitor && curl -X POST http://localhost:8080/api/collect -d '{"province":"shandong","keywords":["软件","信息化"]}'
```

### 通知功能

在 `main.go` 中添加通知逻辑：

```go
// 发送微信通知
func sendWeChatNotification(tender *Tender) {
    // TODO: 实现微信推送
}

// 发送邮件通知
func sendEmailNotification(tender *Tender) {
    // TODO: 实现邮件推送
}
```

## 📚 参考资源

### 技术栈

- [Go](https://golang.org/) - 主程序语言
- [Rod](https://github.com/go-rod/rod) - 浏览器自动化
- [ddddocr](https://github.com/sml2h3/ddddocr) - 验证码识别
- [SQLite](https://www.sqlite.org/) - 数据库

### 相关文档

- [Chrome DevTools Recorder](https://developer.chrome.com/docs/devtools/recorder/)
- [Rod 使用指南](https://go-rod.github.io/)
- [ddddocr 使用文档](https://github.com/sml2h3/ddddocr)

## 📝 许可证

MIT License

## 👥 贡献

欢迎提交 Issue 和 Pull Request！

## 📞 联系方式

如有问题，请通过以下方式联系：

- 提交 GitHub Issue
- 发送邮件至：your-email@example.com

---

**版本：** 1.0.0
**最后更新：** 2026-02-13
