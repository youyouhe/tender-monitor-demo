# Windows 部署指南

> 由于山东省政府采购网对 Linux 环境有限制，本项目需要在 Windows 环境下运行

## 系统要求

- Windows 10/11 (64位)
- 至少 4GB 内存
- 稳定的网络连接（中国大陆）

---

## 一、安装依赖

### 1.1 安装 Go 语言

**下载：**
- 访问 https://go.dev/dl/
- 下载 Windows 安装包（如 `go1.21.6.windows-amd64.msi`）

**安装步骤：**
```cmd
# 1. 双击安装包，按默认路径安装（C:\Program Files\Go）
# 2. 安装完成后，打开命令提示符验证
go version
```

应该看到类似输出：
```
go version go1.21.6 windows/amd64
```

### 1.2 安装 Python（用于验证码识别）

**下载：**
- 访问 https://www.python.org/downloads/
- 下载 Python 3.9+ (64位)

**安装步骤：**
```cmd
# 1. 双击安装包
# 2. ⚠️ 重要：勾选 "Add Python to PATH"
# 3. 选择 "Install Now"
# 4. 验证安装
python --version
pip --version
```

### 1.3 安装 Git (可选，用于代码管理)

**下载：**
- 访问 https://git-scm.com/download/win
- 下载 Git for Windows

---

## 二、下载项目代码

### 方式 1：从 GitHub 克隆
```cmd
cd C:\
git clone https://github.com/youyouhe/tender-monitor-demo-01.git
cd tender-monitor-demo-01
```

### 方式 2：手动下载
1. 访问 https://github.com/youyouhe/tender-monitor-demo-01
2. 点击 "Code" → "Download ZIP"
3. 解压到 `C:\tender-monitor\`

---

## 三、安装 Python 依赖

```cmd
cd C:\tender-monitor\captcha-service
pip install -r requirements.txt
```

requirements.txt 内容：
```
flask==3.0.0
ddddocr==1.4.11
pillow==10.2.0
```

---

## 四、项目结构

```
C:\tender-monitor\
├── main.go                    # 主程序
├── go.mod                     # Go 依赖
├── go.sum
├── captcha-service/           # 验证码服务
│   ├── captcha_service.py
│   └── requirements.txt
├── traces/                    # 轨迹文件
│   ├── shandong_list.json
│   └── shandong_detail.json
├── static/                    # Web 界面
│   └── index.html
├── data/                      # 数据存储
│   └── tenders.db            # SQLite 数据库
└── screenshots/               # 截图（自动创建）
```

---

## 五、启动服务

### 5.1 启动验证码识别服务

打开**第一个命令提示符窗口**：

```cmd
cd C:\tender-monitor\captcha-service
python captcha_service.py
```

看到如下输出表示成功：
```
 * Running on http://127.0.0.1:5000
验证码识别服务已启动！
```

**保持此窗口运行！**

### 5.2 启动主程序

打开**第二个命令提示符窗口**：

```cmd
cd C:\tender-monitor
go run main.go
```

首次运行会自动下载依赖，看到如下输出表示成功：
```
[15:04:05] 正在启动浏览器...
[15:04:06] 访问首页: 山东省政府采购网...
[15:04:08] 步骤 1: 点击【采购公告】标签...
```

**浏览器会自动打开，进行自动化操作**

---

## 六、访问 Web 界面

主程序启动后，打开浏览器访问：

```
http://localhost:8080
```

你会看到紫色渐变的招标信息界面。

---

## 七、配置定时任务

### 方式 1：Windows 任务计划程序（推荐）

**创建批处理脚本：**

保存为 `C:\tender-monitor\run_scraper.bat`：

```batch
@echo off
REM 启动验证码服务
start /B python C:\tender-monitor\captcha-service\captcha_service.py

REM 等待 5 秒让验证码服务启动
timeout /t 5 /nobreak

REM 运行主程序
cd C:\tender-monitor
go run main.go

REM 保持窗口打开
pause
```

**设置定时任务：**

1. 按 `Win + R`，输入 `taskschd.msc`，打开任务计划程序
2. 点击右侧 "创建基本任务"
3. 名称：`招标信息采集`
4. 触发器：选择 "每天"，时间 `09:00`
5. 操作：选择 "启动程序"
   - 程序/脚本：`C:\tender-monitor\run_scraper.bat`
   - 起始于：`C:\tender-monitor`
6. 完成创建

### 方式 2：使用 PowerShell 脚本

保存为 `C:\tender-monitor\run_scraper.ps1`：

```powershell
# 启动验证码服务
Start-Process python -ArgumentList "C:\tender-monitor\captcha-service\captcha_service.py" -WindowStyle Hidden

# 等待服务启动
Start-Sleep -Seconds 5

# 运行主程序
Set-Location C:\tender-monitor
go run main.go
```

运行：
```cmd
powershell -ExecutionPolicy Bypass -File C:\tender-monitor\run_scraper.ps1
```

---

## 八、配置说明

### 8.1 修改搜索关键词

编辑 `main.go`，找到第 17 行：

```go
searchKeyword   = "软件"  // 改成你想搜索的关键词
```

### 8.2 修改采集频率

编辑 `main.go`，添加定时器（在 `main` 函数中）：

```go
func main() {
    // 每天 9:00 执行一次
    ticker := time.NewTicker(24 * time.Hour)
    defer ticker.Stop()

    for {
        if err := run(); err != nil {
            logStep("【致命错误】" + err.Error())
        }
        <-ticker.C
    }
}
```

### 8.3 验证码识别配置

编辑 `captcha-service/captcha_service.py`：

```python
# 如果识别率低，可以调整参数
ocr = ddddocr.DdddOcr(
    show_ad=False,
    beta=True  # 启用测试版模型，可能提升准确率
)
```

---

## 九、常见问题

### Q1: 浏览器无法启动

**解决方案：**
```cmd
# 安装 Chrome 浏览器
# Rod 会自动下载 Chromium，如果失败可以手动指定 Chrome 路径
```

在 `main.go` 中添加：
```go
l := launcher.New().
    Bin("C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe")
```

### Q2: 验证码识别失败

**解决方案：**
- 检查验证码服务是否运行（访问 http://localhost:5000/health）
- 尝试手动识别几次，让程序学习
- 考虑使用更强的 OCR 模型

### Q3: 数据库权限错误

**解决方案：**
```cmd
# 确保 data 目录有写权限
icacls C:\tender-monitor\data /grant Users:F
```

### Q4: 网络连接超时

**解决方案：**
- 确保使用中国大陆网络
- 检查防火墙设置
- 尝试关闭 VPN

### Q5: Go 依赖下载失败

**解决方案：**
```cmd
# 配置 Go 代理（中国大陆）
go env -w GOPROXY=https://goproxy.cn,direct
go mod download
```

---

## 十、性能优化

### 10.1 减少资源占用

编辑 `main.go`，启用无头模式：

```go
l := launcher.New().
    Headless(true).  // 改为 true，不显示浏览器窗口
```

### 10.2 加速采集

```go
const (
    slowMotionDelay = 200 * time.Millisecond  // 减少延迟
    pageLoadTimeout = 10 * time.Second        // 缩短超时
)
```

### 10.3 并发采集多个省份

```go
func main() {
    provinces := []string{"shandong", "guangdong", "beijing"}

    var wg sync.WaitGroup
    for _, province := range provinces {
        wg.Add(1)
        go func(p string) {
            defer wg.Done()
            runForProvince(p)
        }(province)
    }
    wg.Wait()
}
```

---

## 十一、日志和监控

### 11.1 启用详细日志

```go
import "log"

func init() {
    logFile, _ := os.OpenFile("scraper.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
    log.SetOutput(logFile)
}
```

### 11.2 监控服务状态

创建 `check_status.bat`：

```batch
@echo off
echo 检查验证码服务...
curl http://localhost:5000/health

echo.
echo 检查主程序...
curl http://localhost:8080/api/stats

pause
```

---

## 十二、备份和恢复

### 备份数据库

```batch
@echo off
set BACKUP_DIR=C:\tender-monitor\backups
set TIMESTAMP=%date:~0,4%%date:~5,2%%date:~8,2%_%time:~0,2%%time:~3,2%%time:~6,2%

mkdir %BACKUP_DIR% 2>nul
copy C:\tender-monitor\data\tenders.db %BACKUP_DIR%\tenders_%TIMESTAMP%.db

echo 备份完成: %BACKUP_DIR%\tenders_%TIMESTAMP%.db
```

### 恢复数据库

```cmd
copy C:\tender-monitor\backups\tenders_20260213_150405.db C:\tender-monitor\data\tenders.db
```

---

## 十三、安全建议

1. **不要在公网暴露端口**
   - Web 界面仅监听 `127.0.0.1:8080`（本地访问）
   - 如需远程访问，使用 VPN 或 SSH 隧道

2. **定期更新依赖**
   ```cmd
   go get -u all
   pip install --upgrade -r requirements.txt
   ```

3. **保护数据库**
   - 定期备份
   - 设置文件权限
   - 加密敏感信息

---

## 十四、技术支持

**问题反馈：**
- GitHub Issues: https://github.com/youyouhe/tender-monitor-demo-01/issues

**文档更新：**
- 项目 Wiki: https://github.com/youyouhe/tender-monitor-demo-01/wiki

**参考资料：**
- Go Rod 文档: https://go-rod.github.io/
- ddddocr 文档: https://github.com/sml2h3/ddddocr

---

## 附录：快速启动清单

```
□ 安装 Go 1.21+
□ 安装 Python 3.9+
□ 下载项目代码到 C:\tender-monitor
□ 安装 Python 依赖: pip install -r requirements.txt
□ 启动验证码服务: python captcha_service.py
□ 启动主程序: go run main.go
□ 访问 Web 界面: http://localhost:8080
□ 配置定时任务（可选）
□ 设置数据库备份（可选）
```

---

*最后更新：2026-02-13*
*适用版本：v1.0*
