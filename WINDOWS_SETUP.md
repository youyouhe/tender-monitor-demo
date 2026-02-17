# Windows Client Setup Guide

## 🚀 Quick Start (3 Steps)

### Step 1: Clone the Repository

```powershell
git clone https://github.com/youyouhe/tender-monitor-demo.git
cd tender-monitor-demo
```

### Step 2: Configure Server Address

Edit `start-windows.bat` or `start-windows.ps1`:

**Change this line:**
```batch
set CAPTCHA_SERVICE=http://YOUR_SERVER_IP:5000
```

**To your actual server IP:**
```batch
set CAPTCHA_SERVICE=http://192.168.1.100:5000
```

### Step 3: Run the Script

**Option A: Double-click** `start-windows.bat`

**Option B: PowerShell**
```powershell
.\start-windows.ps1
```

**Option C: Command Prompt**
```cmd
start-windows.bat
```

---

## 📋 Prerequisites

### Required

- **Go 1.21+** - [Download](https://go.dev/dl/)
- **Git** - [Download](https://git-scm.com/download/win)

### Optional

- **Chrome/Chromium** - Automatically downloaded by go-rod if not found

---

## ⚙️ Configuration

### Environment Variables

You can set these in the script or via Windows environment:

| Variable | Default | Description |
|----------|---------|-------------|
| `CAPTCHA_SERVICE` | `http://YOUR_SERVER_IP:5000` | **Must change!** Your server address |
| `BROWSER_HEADLESS` | `false` | `true` = invisible, `false` = visible |
| `DATA_DIR` | `./data` | Database and logs directory |
| `TRACES_DIR` | `./traces` | Trace files directory |

### Example: Using PowerShell Variables

```powershell
$env:CAPTCHA_SERVICE = "http://192.168.1.100:5000"
$env:BROWSER_HEADLESS = "false"
go run main.go
```

---

## 🧪 Testing

### 1. Test Captcha Service Connection

```powershell
# Replace with your server IP
curl http://192.168.1.100:5000/health
```

**Expected output:**
```json
{
  "status": "ok",
  "service": "captcha-ocr",
  ...
}
```

### 2. Test Main Program

```powershell
# Health check
curl http://localhost:8080/api/health

# Start a collection task
curl -X POST http://localhost:8080/api/collect `
  -H "Content-Type: application/json" `
  -d '{"province":"guangdong","keywords":["软件"]}'
```

### 3. Access Web Interface

Open browser: http://localhost:8080

---

## 🎯 Usage Examples

### Collect Guangdong Province Data

**Via Web UI:**
1. Open http://localhost:8080
2. Select "guangdong" from province dropdown
3. Enter keywords: 软件, 信息化
4. Click "Start Collection"

**Via API:**
```powershell
curl -X POST http://localhost:8080/api/collect `
  -H "Content-Type: application/json" `
  -d '{"province":"guangdong","keywords":["软件","信息化"]}'
```

### Query Results

```powershell
# All results
curl "http://localhost:8080/api/tenders"

# Filter by province
curl "http://localhost:8080/api/tenders?province=guangdong"

# Filter by keyword
curl "http://localhost:8080/api/tenders?province=guangdong&keyword=软件"
```

---

## 🐛 Troubleshooting

### Issue 1: "Go not found"

**Solution:**
1. Download Go from https://go.dev/dl/
2. Install and restart terminal
3. Verify: `go version`

### Issue 2: Cannot Connect to Captcha Service

**Checklist:**
- ✅ Server IP address is correct
- ✅ Server is running: `./start-server.sh status`
- ✅ Port 5000 is accessible
- ✅ Windows Firewall allows outbound connections
- ✅ Server firewall/security group allows port 5000

**Test connection:**
```powershell
# Test from Windows
Test-NetConnection -ComputerName YOUR_SERVER_IP -Port 5000

# Or use telnet
telnet YOUR_SERVER_IP 5000
```

### Issue 3: Browser Opens But No Data

**Possible reasons:**
- Target website may have changed structure
- Need to update trace files
- Anti-bot detection

**Check logs:**
Look for error messages in the console output.

### Issue 4: Permission Denied

**Solution:**
Run as Administrator (right-click → Run as administrator)

### Issue 5: Port 8080 Already in Use

**Solution:**
```powershell
# Find process using port 8080
netstat -ano | findstr :8080

# Kill process (replace PID)
taskkill /PID <PID> /F
```

---

## 📂 Directory Structure

```
tender-monitor-demo/
├── main.go                      # Main program
├── start-windows.bat            # CMD startup script
├── start-windows.ps1            # PowerShell startup script
├── traces/
│   ├── guangdong_list.json     # Guangdong list trace
│   ├── guangdong_detail.json   # Guangdong detail trace
│   ├── shandong_list.json      # Shandong list trace
│   └── shandong_detail.json    # Shandong detail trace
├── data/
│   ├── tenders.db              # SQLite database (auto-created)
│   └── browser-data/           # Browser profile (auto-created)
└── static/
    └── index.html              # Web UI
```

---

## 🔄 Updating

```powershell
# Stop the program (Ctrl+C)

# Pull latest changes
git pull

# Restart
start-windows.bat
```

---

## 🛡️ Security Notes

1. **Data Storage**: All data stored locally in `./data/tenders.db`
2. **Browser Data**: Stored in `./data/browser-data/`
3. **Network**: Only connects to:
   - Target government websites (for scraping)
   - Your captcha server (configured address)
4. **No Cloud**: No data sent to external services

---

## 🆘 Getting Help

### Check Logs

Look at console output for error messages.

### Common Issues

1. **Can't connect to server** → Check `CAPTCHA_SERVICE` address
2. **Browser won't start** → Check disk space, try deleting `./data/browser-data/`
3. **No results found** → Target website may have changed, need to update traces

### Report Bugs

GitHub Issues: https://github.com/youyouhe/tender-monitor-demo/issues

---

## 📚 Related Documentation

- [Server Deployment Guide](./REMOTE_DEPLOY.md)
- [Server Quick Start](./captcha-service/QUICKSTART.md)
- [Complete README](./README.md)

---

**Last Updated:** 2026-02-18
