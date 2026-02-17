# ============================================================
# Tender Monitor System - Windows PowerShell Startup Script
# ============================================================

# ====== Configuration ======
# Please modify to your captcha server address
$env:CAPTCHA_SERVICE = "http://YOUR_SERVER_IP:5000"

# Other settings (usually no need to change)
$env:BROWSER_HEADLESS = "false"
$env:DATA_DIR = "./data"
$env:TRACES_DIR = "./traces"
# =====================

Write-Host ""
Write-Host "========================================"
Write-Host "  Tender Monitor - Windows Client"
Write-Host "========================================"
Write-Host ""
Write-Host "Configuration:"
Write-Host "  Captcha Service: $env:CAPTCHA_SERVICE"
Write-Host "  Browser Mode:    Visible window"
Write-Host "  Data Directory:  $env:DATA_DIR"
Write-Host "  Trace Directory: $env:TRACES_DIR"
Write-Host ""
Write-Host "========================================"
Write-Host ""

# Check if Go is installed
try {
    $goVersion = go version
    Write-Host "[SUCCESS] Go installed: $goVersion"
} catch {
    Write-Host "[ERROR] Go not found. Please install Go 1.21+" -ForegroundColor Red
    Write-Host "        Download: https://go.dev/dl/"
    Read-Host "Press Enter to exit"
    exit 1
}

# Test captcha service connectivity
Write-Host "[CHECK] Testing captcha service connection..."
try {
    $response = Invoke-WebRequest -Uri "$env:CAPTCHA_SERVICE/health" -TimeoutSec 5 -UseBasicParsing
    Write-Host "[SUCCESS] Captcha service connection OK" -ForegroundColor Green
    Write-Host ""
} catch {
    Write-Host "[WARNING] Cannot connect to captcha service: $env:CAPTCHA_SERVICE" -ForegroundColor Yellow
    Write-Host "          Please check:"
    Write-Host "          1. Server address is correct"
    Write-Host "          2. Server service is running"
    Write-Host "          3. Firewall port 5000 is open"
    Write-Host ""
    Write-Host "Continue anyway? The program will fall back to manual input." -ForegroundColor Yellow
    Read-Host "Press Enter to continue, or Ctrl+C to cancel"
}

# Start main program
Write-Host "[STARTING] Launching main program..."
Write-Host ""

try {
    go run main.go
} catch {
    Write-Host ""
    Write-Host "========================================"
    Write-Host "Program exited with error: $($_.Exception.Message)" -ForegroundColor Red
    Write-Host "========================================"
}

# Program exited
Write-Host ""
Write-Host "========================================"
Write-Host "Program exited"
Write-Host "========================================"
Read-Host "Press Enter to close"
