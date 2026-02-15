# AGENTS.md - Coding Guidelines for my-test

## Project Overview

Go-based browser automation project using the [Rod](https://github.com/go-rod/rod) library for web scraping and browser control. The project includes multiple components:
- **test_shandong_debug.go**: Main scraper for Shandong government procurement website
- **tender-monitor-demo/**: Full-featured tender monitoring system with web API, database, and CAPTCHA service

## Build Commands

```bash
# Build the main test script
go build -o test_shandong test_shandong_debug.go

# Build the tender monitor service (from tender-monitor-demo/)
cd tender-monitor-demo
go build -o tender-monitor main.go

# Run the main debug script
go run test_shandong_debug.go

# Run the tender monitor service
cd tender-monitor-demo
go run main.go

# Download dependencies
go mod download

# Tidy dependencies (clean up unused)
go mod tidy

# Verify dependencies
go mod verify

# Update specific dependency
go get -u github.com/go-rod/rod@latest
```

## Test Commands

```bash
# Run all tests
go test ./...

# Run a single test function (example)
go test -run TestFunctionName ./...

# Run tests with verbose output
go test -v ./...

# Run tests with coverage
go test -cover ./...

# Generate coverage report HTML
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run tests with race detector
go test -race ./...

# Run benchmarks
go test -bench=. ./...
```

## Lint Commands

```bash
# Format code with gofmt (REQUIRED before commit)
gofmt -w .

# Format and organize imports
goimports -w .

# Run golangci-lint (if installed)
golangci-lint run

# Vet for common issues
go vet ./...

# Check for suspicious constructs
go vet -all ./...
```

## Code Style Guidelines

### Formatting
- Use `gofmt` for all Go code - no exceptions
- Use `goimports` to manage imports automatically
- Tab indentation (width 8, don't change)
- Line length: aim for 100 chars, hard limit 120
- No trailing whitespace
- End files with a newline

### Imports
- Group imports in three sections with blank lines between:
  1. Standard library (e.g., `fmt`, `os`, `time`)
  2. Third-party packages (e.g., `github.com/go-rod/rod`)
  3. Local packages (if any)
- Use import aliases when package name differs from path
- Remove unused imports (enforced by compiler)
- Prefer explicit imports over dot imports (never use `.`)
- Example:
  ```go
  import (
      "fmt"
      "time"
      
      "github.com/go-rod/rod"
      "github.com/go-rod/rod/lib/launcher"
  )
  ```

### Naming Conventions
- **Packages**: lowercase, single word, no underscores (e.g., `main`, `captcha`)
- **Exported**: PascalCase/starts with capital (e.g., `Notice`, `CaptchaSolver`)
- **Unexported**: camelCase/starts with lowercase (e.g., `logStep`, `setupBrowser`)
- **Constants**: Use const blocks, group related constants together
  - Exported: PascalCase (e.g., `DefaultTimeout`)
  - Unexported: camelCase (e.g., `targetURL`, `slowMotionDelay`)
- **Interfaces**: -er suffix (e.g., `Reader`, `Writer`, `Stringer`)
- **Variables**: 
  - Short names in small scopes (e.g., `i`, `err`, `ctx`)
  - Descriptive names in larger scopes (e.g., `captchaSolver`, `searchKeyword`)
- **Files**: snake_case (e.g., `test_shandong_debug.go`, `captcha_service.py`)

### Types
- Define types at the top of file (after imports, before constants)
- Use explicit types, avoid `interface{}` when possible (prefer `any` in Go 1.18+)
- Return concrete types, accept interfaces
- Use struct tags for JSON/XML marshaling (e.g., `` `json:"id"` ``)
- Embed interfaces to extend behavior
- Example struct with proper tags:
  ```go
  type Tender struct {
      ID          int       `json:"id"`
      Province    string    `json:"province"`
      Title       string    `json:"title"`
      CreatedAt   time.Time `json:"created_at"`
  }
  ```

### Error Handling
- Always check errors, never ignore with `_` unless justified with comment
- Return errors up the call stack (don't log and return)
- Wrap errors with context: `fmt.Errorf("doing X: %w", err)`
- Use `errors.Is()` and `errors.As()` for error inspection
- Panic only for unrecoverable errors or initialization failures
- Use `rod.Try()` for graceful error handling with Rod operations
- Example:
  ```go
  err := rod.Try(func() {
      page.MustElement(selector).MustClick()
  })
  if err != nil {
      return fmt.Errorf("failed to click element: %w", err)
  }
  ```

### Rod Browser Automation
- Always use `defer browser.MustClose()` or `defer page.Close()` for cleanup
- Set appropriate timeouts: `page.Timeout(15 * time.Second)`
- Use `Must*` methods for quick scripts, check errors for robust production code
- Set viewport with `MustSetViewport(1920, 1080, 1, false)` for consistent rendering
- Use `MustWindowMaximize()` for better visibility during debugging
- Use `MustWaitStable()` after interactions that change page state
- Handle CAPTCHA with graceful degradation (auto → manual fallback)
- Take screenshots with timestamps for debugging: `page.MustScreenshot("debug_" + time.Now().Format("150405") + ".png")`
- Prefer CSS selectors over XPath when possible (faster and more readable)
- Use `ElementX()` for XPath when CSS selectors are insufficient
- Add sleeps between operations: `time.Sleep(500 * time.Millisecond)`
- Use launcher options to avoid detection:
  ```go
  launcher.New().
      Headless(false).
      Set("disable-blink-features", "AutomationControlled")
  ```

### Functions
- Keep functions small and focused (ideally under 50 lines)
- Use early returns to reduce nesting depth
- Accept interfaces, return concrete types
- Document exported functions with comments starting with function name
- Group related functions together (e.g., all setup functions, all extraction functions)
- Example function signature:
  ```go
  // extractNotices extracts notice data from the search results page.
  // Returns a slice of Notice structs and any error encountered.
  func extractNotices(page *rod.Page) ([]Notice, error) {
      // implementation
  }
  ```

### Comments
- Exported items MUST have doc comments
- Comments start with the name being declared
- Use complete sentences with proper punctuation
- Explain why, not what (code shows what)
- Add inline comments for complex logic or non-obvious behavior
- Example:
  ```go
  // Notice represents a government procurement notice with all metadata.
  type Notice struct {
      // No is the sequence number in the search results (1-based).
      No string
      Title string
  }
  ```

### Constants
- Define constants at package level (after imports and types)
- Group related constants in const blocks
- Use typed constants when possible
- Use `time.Duration` for all time-related constants
- Example:
  ```go
  const (
      targetURL       = "http://www.ccgp-shandong.gov.cn/home"
      viewportWidth   = 1157
      viewportHeight  = 865
      slowMotionDelay = 800 * time.Millisecond
      pageLoadTimeout = 15 * time.Second
  )
  ```

### Project-Specific Guidelines
- Use `logStep()` helper for all user-facing console output with timestamps
- All sleep durations must use `time.Duration` constants (never raw numbers)
- Screenshots must include timestamps in filenames: `time.Now().Format("150405")`
- Handle browser windows with `MustWindowMaximize()` during development
- CSV files should include timestamps: `time.Now().Format("20060102_150405")`
- Use Chinese text for user-facing messages, English for code/logs
- Validate input data (e.g., `isValidSequenceNo()`) before processing
- Close resources properly with defer statements
- Check service availability before use: `solver.CheckAvailable()`

### HTTP/API (for tender-monitor-demo)
- Use standard library `net/http` for APIs
- Return JSON with proper Content-Type header
- Use HTTP status codes appropriately (200, 400, 500, etc.)
- Validate request input before processing
- Run long tasks asynchronously with `go` keyword
- Log important events with timestamp

### Database (SQLite)
- Use `modernc.org/sqlite` for pure Go SQLite (no CGO)
- Use parameterized queries to prevent SQL injection
- Use `INSERT OR IGNORE` for idempotent inserts
- Create indexes on frequently queried columns
- Use transactions for batch operations
- Handle database connection errors gracefully
