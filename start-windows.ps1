# ============================================================
# 招标信息监控系统 - Windows PowerShell 启动脚本
# ============================================================

# ====== 配置区域 ======
# 请修改为你的验证码服务器地址
$env:CAPTCHA_SERVICE = "http://YOUR_SERVER_IP:5000"

# 其他配置（通常不需要修改）
$env:BROWSER_HEADLESS = "false"
$env:DATA_DIR = "./data"
$env:TRACES_DIR = "./traces"
# =====================

Write-Host ""
Write-Host "========================================"
Write-Host "  招标信息监控系统 - Windows 客户端"
Write-Host "========================================"
Write-Host ""
Write-Host "配置信息:"
Write-Host "  验证码服务: $env:CAPTCHA_SERVICE"
Write-Host "  浏览器模式: 有头模式 (可见窗口)"
Write-Host "  数据目录:   $env:DATA_DIR"
Write-Host "  轨迹目录:   $env:TRACES_DIR"
Write-Host ""
Write-Host "========================================"
Write-Host ""

# 检查 Go 是否安装
try {
    $goVersion = go version
    Write-Host "[成功] Go 环境已安装: $goVersion"
} catch {
    Write-Host "[错误] 未检测到 Go 环境，请先安装 Go 1.21+" -ForegroundColor Red
    Write-Host "       下载地址: https://go.dev/dl/"
    Read-Host "按任意键退出"
    exit 1
}

# 检查验证码服务连通性
Write-Host "[检查] 测试验证码服务连接..."
try {
    $response = Invoke-WebRequest -Uri "$env:CAPTCHA_SERVICE/health" -TimeoutSec 5 -UseBasicParsing
    Write-Host "[成功] 验证码服务连接正常" -ForegroundColor Green
    Write-Host ""
} catch {
    Write-Host "[警告] 无法连接到验证码服务: $env:CAPTCHA_SERVICE" -ForegroundColor Yellow
    Write-Host "        请检查:"
    Write-Host "        1. 服务器地址是否正确"
    Write-Host "        2. 服务器端服务是否启动"
    Write-Host "        3. 防火墙是否开放 5000 端口"
    Write-Host ""
    Write-Host "是否继续启动？程序会在需要验证码时降级到手动输入。" -ForegroundColor Yellow
    Read-Host "按 Enter 继续，或按 Ctrl+C 取消"
}

# 启动主程序
Write-Host "[启动] 正在启动主程序..."
Write-Host ""

try {
    go run main.go
} catch {
    Write-Host ""
    Write-Host "========================================"
    Write-Host "程序异常退出: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host "========================================"
}

# 程序退出
Write-Host ""
Write-Host "========================================"
Write-Host "程序已退出"
Write-Host "========================================"
Read-Host "按任意键关闭窗口"
