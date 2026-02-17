@echo off
REM ============================================================
REM 招标信息监控系统 - Windows 启动脚本
REM ============================================================

REM ====== 配置区域 ======
REM 请修改为你的验证码服务器地址
set CAPTCHA_SERVICE=http://YOUR_SERVER_IP:5000

REM 其他配置（通常不需要修改）
set BROWSER_HEADLESS=false
set DATA_DIR=./data
set TRACES_DIR=./traces

REM =====================

echo.
echo ========================================
echo   招标信息监控系统 - Windows 客户端
echo ========================================
echo.
echo 配置信息:
echo   验证码服务: %CAPTCHA_SERVICE%
echo   浏览器模式: 有头模式 (可见窗口)
echo   数据目录:   %DATA_DIR%
echo   轨迹目录:   %TRACES_DIR%
echo.
echo ========================================
echo.

REM 检查 Go 是否安装
go version >nul 2>&1
if errorlevel 1 (
    echo [错误] 未检测到 Go 环境，请先安装 Go 1.21+
    echo 下载地址: https://go.dev/dl/
    pause
    exit /b 1
)

REM 检查验证码服务连通性
echo [检查] 测试验证码服务连接...
curl -s %CAPTCHA_SERVICE%/health >nul 2>&1
if errorlevel 1 (
    echo [警告] 无法连接到验证码服务: %CAPTCHA_SERVICE%
    echo         请检查:
    echo         1. 服务器地址是否正确
    echo         2. 服务器端服务是否启动
    echo         3. 防火墙是否开放 5000 端口
    echo.
    echo 是否继续启动？程序会在需要验证码时降级到手动输入。
    pause
) else (
    echo [成功] 验证码服务连接正常
    echo.
)

REM 启动主程序
echo [启动] 正在启动主程序...
echo.
go run main.go

REM 如果程序异常退出
echo.
echo ========================================
echo 程序已退出
echo ========================================
pause
