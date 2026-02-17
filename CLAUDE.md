# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Government procurement tender monitoring system (招标信息监控系统). A Go web server crawls provincial procurement websites using browser automation, extracts tender information, and stores it in SQLite. A companion Python FastAPI microservice handles CAPTCHA recognition via ddddocr OCR, with an optional Qwen2-VL intelligent recognition service.

## Build & Run Commands

```bash
# Build the main server (pure Go, no CGO required)
go build -o tender-monitor main.go

# Run directly
go run main.go

# Build standalone tools
go build -o collect ./cmd/collect/              # Shandong collector CLI
go build -o convert-trace ./cmd/convert-trace/  # Chrome trace converter

# Full deployment (installs deps, builds, starts services)
./deploy.sh install

# Service management
./deploy.sh start | stop | restart | status | logs

# Convert Chrome DevTools Recorder trace to simplified format
go run ./cmd/convert-trace/ recording.json list traces/province_list.json

# Docker
docker build -t tender-monitor .
docker-compose up -d
```

### Captcha Service (Python)

```bash
cd captcha-service
python3 -m venv venv && source venv/bin/activate
pip install -r requirements.txt              # ddddocr engine (FastAPI, uvicorn, ddddocr)
# For Qwen2-VL engine (optional, needs GPU), install additional dependencies:
# pip install transformers torch pillow qwen-vl-utils
uvicorn app:app --host 0.0.0.0 --port 5000

# Test
python3 test_captcha.py                      # ddddocr engine
python3 test_qwen_captcha.py                 # Qwen2-VL engine (if installed)
```

### Verify Services

```bash
curl http://localhost:8080/api/health    # Main server
curl http://localhost:5000/health        # Captcha service
```

## Architecture

The system has three components that work together:

**Go Server (`main.go`)** - Web server, crawler engine, database layer, and API handlers. Uses `//go:embed static/*` to bundle the frontend into the binary. Uses `modernc.org/sqlite` (pure Go, no CGO). Supports both CSS selectors and XPath for element extraction. Serves on port 8080.

**Standalone Tools (`cmd/`)** - Two independent CLI programs:
- `cmd/collect/` - Shandong province dedicated collector with CSV export
- `cmd/convert-trace/` - Chrome DevTools Recorder JSON → simplified trace format converter

**Python Captcha Service (`captcha-service/app.py`)** - Unified FastAPI service with two switchable engines in a single app:
- `ddddocr` (default) - lightweight OCR, no GPU needed, always available
- `qwen` - Qwen2-VL intelligent recognition (90%+ accuracy), needs GPU and additional dependencies

Switch engine per request via `?engine=qwen` query param or `{"engine": "qwen"}` in JSON body. Accepts images as raw binary (`Content-Type: image/*`), multipart form, or base64 JSON. The Go server calls `POST /ocr` on port 5000; if unavailable, falls back to manual input. The Qwen engine requires additional packages (transformers, torch, qwen-vl-utils) and model download (~4GB).

**Trace-Driven Crawler** - Province-specific automation is defined in JSON trace files under `traces/`. Each trace describes a sequence of browser actions (navigate, click, input, captcha, wait, extract). New provinces are added by recording a Chrome DevTools trace and converting it with `cmd/convert-trace/`.

### Collection Flow

Two-stage process triggered by `POST /api/collect`:
1. **List stage**: Load province's list trace → navigate to procurement site → search keywords → handle CAPTCHA → extract table rows (title, date, URL)
2. **Detail stage**: For each matched item, load detail trace → extract enrichment fields (amount, contact, phone) → save to SQLite with URL-based deduplication

### API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/health` | Health check |
| GET | `/api/tenders?province=&keyword=` | Query stored tenders (max 100) |
| POST | `/api/collect` | Start async collection `{province, keywords[]}` |
| GET | `/` | Serves embedded static frontend |

### Database

SQLite at `./data/tenders.db`. Single `tenders` table with `url UNIQUE` constraint for deduplication. Indexes on `province` and `publish_date`.

### Key Dependencies

- **go-rod** (v0.114.5) - Chromium browser automation (requires Chromium installed)
- **modernc.org/sqlite** (v1.28.0) - Pure Go SQLite driver (no CGO required)
- **ddddocr** (Python) - CAPTCHA OCR engine using ONNX Runtime
- **Qwen2-VL** (Python, optional) - Intelligent CAPTCHA recognition with higher accuracy

## Testing

```bash
# Quick environment and trace file check
./test_quick.sh

# Test captcha service (ddddocr)
cd captcha-service && python3 test_captcha.py

# Test captcha service (Qwen2-VL, if installed)
cd captcha-service && python3 test_qwen_captcha.py

# Manual API testing
curl http://localhost:8080/api/health
curl "http://localhost:8080/api/tenders?province=shandong&keyword=软件"
```

Note: There are no Go unit tests (*_test.go files) in this project. Testing is primarily done through the test scripts and manual API calls.

## Configuration

Global variables in `main.go` (lines 74-79), overridable via environment:

| Variable | Default | Env Var |
|----------|---------|---------|
| `captchaService` | `http://localhost:5000` | `CAPTCHA_SERVICE` |
| `dataDir` | `./data` | `DATA_DIR` |
| `tracesDir` | `./traces` | `TRACES_DIR` |
| `browserHeadless` | `false` | `BROWSER_HEADLESS` |

## Adding a New Province

1. Record browser interactions using Chrome DevTools Recorder on the target procurement site
2. Export the recording as JSON
3. Convert: `go run ./cmd/convert-trace/ recording.json list traces/<province>_list.json`
4. Create a detail trace similarly with `detail` type
5. The system auto-discovers traces by province name prefix in `traces/`

## Code Conventions

- All source comments and log messages are in Chinese
- The frontend (`static/index.html`) is vanilla HTML/CSS/JS with no build step
- Main server is in `main.go`; standalone tools live in `cmd/` subdirectories (each with their own `main()`)
- Trace template variables use Go template syntax: `{{.Keyword}}`, `{{.URL}}`
- SQLite driver is pure Go (`modernc.org/sqlite`) — database driver name is `"sqlite"`, not `"sqlite3"`
