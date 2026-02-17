@echo off
REM ============================================================
REM Tender Monitor System - Windows Client Startup Script
REM ============================================================

REM ====== Configuration ======
REM Please modify to your captcha server address
set CAPTCHA_SERVICE=http://YOUR_SERVER_IP:5000

REM Other settings (usually no need to change)
set BROWSER_HEADLESS=false
set DATA_DIR=./data
set TRACES_DIR=./traces

REM =====================

echo.
echo ========================================
echo   Tender Monitor - Windows Client
echo ========================================
echo.
echo Configuration:
echo   Captcha Service: %CAPTCHA_SERVICE%
echo   Browser Mode:    Visible window
echo   Data Directory:  %DATA_DIR%
echo   Trace Directory: %TRACES_DIR%
echo.
echo ========================================
echo.

REM Check if Go is installed
go version >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Go not found. Please install Go 1.21+
    echo Download: https://go.dev/dl/
    pause
    exit /b 1
)

REM Test captcha service connectivity
echo [CHECK] Testing captcha service connection...
curl -s %CAPTCHA_SERVICE%/health >nul 2>&1
if errorlevel 1 (
    echo [WARNING] Cannot connect to captcha service: %CAPTCHA_SERVICE%
    echo           Please check:
    echo           1. Server address is correct
    echo           2. Server service is running
    echo           3. Firewall port 5000 is open
    echo.
    echo Continue anyway? The program will fall back to manual input.
    pause
) else (
    echo [SUCCESS] Captcha service connection OK
    echo.
)

REM Start main program
echo [STARTING] Launching main program...
echo.
go run main.go

REM If program exits abnormally
echo.
echo ========================================
echo Program exited
echo ========================================
pause
