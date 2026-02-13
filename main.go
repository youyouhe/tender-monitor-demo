package main

import (
	"bytes"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed static/*
var staticFiles embed.FS

// ==================== 数据结构 ====================

// Tender 招标信息结构体
type Tender struct {
	ID          int       `json:"id"`
	Province    string    `json:"province"`
	Title       string    `json:"title"`
	Amount      string    `json:"amount"`
	PublishDate string    `json:"publish_date"`
	Contact     string    `json:"contact"`
	Phone       string    `json:"phone"`
	URL         string    `json:"url"`
	Keywords    string    `json:"keywords"`
	CreatedAt   time.Time `json:"created_at"`
}

// TraceFile 轨迹文件结构体
type TraceFile struct {
	Name  string      `json:"name"`
	Type  string      `json:"type"` // "list" 或 "detail"
	URL   string      `json:"url"`
	Steps []TraceStep `json:"steps"`
}

// TraceStep 轨迹步骤
type TraceStep struct {
	Action         string            `json:"action"` // navigate, click, input, captcha, extract, wait
	URL            string            `json:"url,omitempty"`
	Selector       string            `json:"selector,omitempty"`
	Value          string            `json:"value,omitempty"`
	ImageSelector  string            `json:"image_selector,omitempty"`
	InputSelector  string            `json:"input_selector,omitempty"`
	Type           string            `json:"type,omitempty"`   // 用于 extract
	Fields         map[string]string `json:"fields,omitempty"` // 用于 extract
	WaitTime       int               `json:"wait_time,omitempty"`
	WaitForVisible string            `json:"wait_for_visible,omitempty"`
}

// CaptchaResponse 验证码服务响应
type CaptchaResponse struct {
	Success    bool    `json:"success"`
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
	Error      string  `json:"error,omitempty"`
}

// ==================== 全局变量 ====================

var (
	db              *sql.DB
	captchaService  = "http://localhost:5000"
	dataDir         = "./data"
	tracesDir       = "./traces"
	browserHeadless = false // 改为 false 便于调试
)

// ==================== 验证码识别器 ====================

type CaptchaSolver struct {
	ServiceURL string
	Client     *http.Client
}

func NewCaptchaSolver(serviceURL string) *CaptchaSolver {
	return &CaptchaSolver{
		ServiceURL: serviceURL,
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Solve 识别验证码
func (cs *CaptchaSolver) Solve(imageBytes []byte) (string, error) {
	// 调用验证码识别服务
	req, err := http.NewRequest("POST", cs.ServiceURL+"/ocr", bytes.NewReader(imageBytes))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "image/png")

	resp, err := cs.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求验证码服务失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result CaptchaResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	if !result.Success {
		return "", fmt.Errorf("识别失败: %s", result.Error)
	}

	return result.Text, nil
}

// CheckAvailable 检查服务是否可用
func (cs *CaptchaSolver) CheckAvailable() bool {
	resp, err := cs.Client.Get(cs.ServiceURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

// ==================== 数据库操作 ====================

func initDB() error {
	var err error
	dbPath := filepath.Join(dataDir, "tenders.db")

	// 确保目录存在
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("创建数据目录失败: %v", err)
	}

	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("打开数据库失败: %v", err)
	}

	// 创建表
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS tenders (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		province TEXT,
		title TEXT,
		amount TEXT,
		publish_date TEXT,
		contact TEXT,
		phone TEXT,
		url TEXT UNIQUE,
		keywords TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_province ON tenders(province);
	CREATE INDEX IF NOT EXISTS idx_publish_date ON tenders(publish_date);
	`

	if _, err := db.Exec(createTableSQL); err != nil {
		return fmt.Errorf("创建表失败: %v", err)
	}

	log.Println("✅ 数据库初始化成功")
	return nil
}

func saveTender(tender *Tender) error {
	query := `
	INSERT OR IGNORE INTO tenders
	(province, title, amount, publish_date, contact, phone, url, keywords)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := db.Exec(query,
		tender.Province,
		tender.Title,
		tender.Amount,
		tender.PublishDate,
		tender.Contact,
		tender.Phone,
		tender.URL,
		tender.Keywords,
	)
	return err
}

func queryTenders(province, keyword string, limit int) ([]Tender, error) {
	query := `
	SELECT id, province, title, amount, publish_date, contact, phone, url, keywords, created_at
	FROM tenders WHERE 1=1
	`
	args := []interface{}{}

	if province != "" {
		query += " AND province = ?"
		args = append(args, province)
	}
	if keyword != "" {
		query += " AND (title LIKE ? OR keywords LIKE ?)"
		args = append(args, "%"+keyword+"%", "%"+keyword+"%")
	}

	query += " ORDER BY publish_date DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tenders []Tender
	for rows.Next() {
		var t Tender
		err := rows.Scan(
			&t.ID, &t.Province, &t.Title, &t.Amount,
			&t.PublishDate, &t.Contact, &t.Phone, &t.URL,
			&t.Keywords, &t.CreatedAt,
		)
		if err != nil {
			continue
		}
		tenders = append(tenders, t)
	}

	return tenders, nil
}

// ==================== 浏览器自动化 ====================

func setupBrowser() (*rod.Browser, error) {
	// 启动浏览器
	var l *launcher.Launcher
	if browserHeadless {
		l = launcher.New().Headless(true)
	} else {
		l = launcher.New().Headless(false)
	}

	url := l.MustLaunch()
	browser := rod.New().ControlURL(url).MustConnect()

	log.Println("✅ 浏览器启动成功")
	return browser, nil
}

// executeTrace 执行轨迹文件
func executeTrace(browser *rod.Browser, trace *TraceFile, params map[string]string, solver *CaptchaSolver) (interface{}, error) {
	page := browser.MustPage()
	defer page.Close()

	var extractedData interface{}

	for i, step := range trace.Steps {
		log.Printf("执行步骤 %d/%d: %s", i+1, len(trace.Steps), step.Action)

		switch step.Action {
		case "navigate":
			url := replaceParams(step.URL, params)
			if err := page.Navigate(url); err != nil {
				return nil, fmt.Errorf("导航失败: %v", err)
			}
			page.MustWaitLoad()

		case "click":
			selector := replaceParams(step.Selector, params)
			elem := page.MustElement(selector)
			elem.MustClick()
			time.Sleep(500 * time.Millisecond)

		case "input":
			selector := replaceParams(step.Selector, params)
			value := replaceParams(step.Value, params)
			elem := page.MustElement(selector)
			elem.MustSelectAllText().MustInput(value)

		case "captcha":
			// 验证码识别
			text, err := handleCaptcha(page, step.ImageSelector, solver)
			if err != nil {
				return nil, fmt.Errorf("验证码识别失败: %v", err)
			}
			log.Printf("✅ 验证码识别结果: %s", text)

			// 输入验证码
			inputElem := page.MustElement(step.InputSelector)
			inputElem.MustSelectAllText().MustInput(text)
			time.Sleep(500 * time.Millisecond)

		case "wait":
			if step.WaitTime > 0 {
				time.Sleep(time.Duration(step.WaitTime) * time.Millisecond)
			}
			if step.WaitForVisible != "" {
				page.MustElement(step.WaitForVisible).MustWaitVisible()
			}

		case "extract":
			// 提取数据
			if step.Type == "list" {
				extractedData = extractList(page, step)
			} else if step.Type == "detail" {
				extractedData = extractDetail(page, step)
			}
		}

		time.Sleep(300 * time.Millisecond) // 每步之间暂停
	}

	return extractedData, nil
}

// handleCaptcha 处理验证码
func handleCaptcha(page *rod.Page, imageSelector string, solver *CaptchaSolver) (string, error) {
	// 截取验证码图片
	imgElem := page.MustElement(imageSelector)
	imgBytes, err := imgElem.Screenshot(nil, nil)
	if err != nil {
		return "", fmt.Errorf("截图失败: %v", err)
	}

	// 保存图片用于调试
	timestamp := time.Now().Format("20060102_150405")
	captchaPath := filepath.Join(dataDir, fmt.Sprintf("captcha_%s.png", timestamp))
	os.WriteFile(captchaPath, imgBytes, 0644)
	log.Printf("验证码已保存: %s", captchaPath)

	// 自动识别（智能降级）
	if solver != nil && solver.CheckAvailable() {
		text, err := solver.Solve(imgBytes)
		if err == nil {
			log.Printf("✅ 自动识别成功: %s", text)
			return text, nil
		}
		log.Printf("⚠️ 自动识别失败: %v，降级到手动输入", err)
	} else {
		log.Println("⚠️ 验证码服务不可用，使用手动输入")
	}

	// 手动输入降级
	fmt.Printf("请查看验证码图片: %s\n", captchaPath)
	fmt.Print("请输入验证码: ")
	var manualInput string
	fmt.Scanln(&manualInput)
	return manualInput, nil
}

// extractList 提取列表数据
func extractList(page *rod.Page, step TraceStep) []map[string]string {
	var results []map[string]string

	// 等待列表加载
	time.Sleep(2 * time.Second)

	rows := page.MustElements(step.Selector)
	log.Printf("找到 %d 条记录", len(rows))

	for _, row := range rows {
		item := make(map[string]string)
		for field, selector := range step.Fields {
			elem := row.MustElement(selector)
			if field == "url" {
				item[field], _ = elem.Attribute("href")
			} else {
				item[field] = elem.MustText()
			}
		}
		results = append(results, item)
	}

	return results
}

// extractDetail 提取详情数据
func extractDetail(page *rod.Page, step TraceStep) map[string]string {
	result := make(map[string]string)

	time.Sleep(2 * time.Second)

	for field, selector := range step.Fields {
		elem, err := page.Element(selector)
		if err == nil {
			result[field] = elem.MustText()
		}
	}

	return result
}

// replaceParams 替换参数
func replaceParams(template string, params map[string]string) string {
	result := template
	for key, value := range params {
		placeholder := fmt.Sprintf("{{.%s}}", key)
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}

// ==================== 采集任务 ====================

func runCollectTask(province string, keywords []string) error {
	log.Printf("🚀 开始采集任务：省份=%s, 关键词=%v", province, keywords)

	// 加载轨迹文件
	listTracePath := filepath.Join(tracesDir, province+"_list.json")
	detailTracePath := filepath.Join(tracesDir, province+"_detail.json")

	listTrace, err := loadTrace(listTracePath)
	if err != nil {
		return fmt.Errorf("加载列表轨迹失败: %v", err)
	}

	detailTrace, err := loadTrace(detailTracePath)
	if err != nil {
		return fmt.Errorf("加载详情轨迹失败: %v", err)
	}

	// 初始化浏览器和验证码识别器
	browser, err := setupBrowser()
	if err != nil {
		return err
	}
	defer browser.Close()

	solver := NewCaptchaSolver(captchaService)

	// 阶段1：采集列表
	for _, keyword := range keywords {
		log.Printf("\n--- 关键词: %s ---", keyword)

		params := map[string]string{"Keyword": keyword}
		data, err := executeTrace(browser, listTrace, params, solver)
		if err != nil {
			log.Printf("❌ 列表采集失败: %v", err)
			continue
		}

		listItems := data.([]map[string]string)
		log.Printf("📋 列表采集完成，共 %d 条", len(listItems))

		// 阶段2：采集详情（仅匹配关键词的）
		for i, item := range listItems {
			title := item["title"]

			// 检查是否包含关键词
			if !containsKeyword(title, keywords) {
				log.Printf("跳过（不匹配）: %s", title)
				continue
			}

			log.Printf("\n[%d/%d] 采集详情: %s", i+1, len(listItems), title)

			// 执行详情采集
			detailParams := map[string]string{"URL": item["url"]}
			detailData, err := executeTrace(browser, detailTrace, detailParams, solver)
			if err != nil {
				log.Printf("❌ 详情采集失败: %v", err)
				continue
			}

			detail := detailData.(map[string]string)

			// 保存到数据库
			tender := &Tender{
				Province:    province,
				Title:       title,
				Amount:      detail["amount"],
				PublishDate: item["date"],
				Contact:     detail["contact"],
				Phone:       detail["phone"],
				URL:         item["url"],
				Keywords:    keyword,
			}

			if err := saveTender(tender); err != nil {
				log.Printf("❌ 保存失败: %v", err)
			} else {
				log.Printf("✅ 已保存到数据库")
			}

			time.Sleep(2 * time.Second) // 防止请求过快
		}
	}

	log.Println("\n✅ 采集任务完成")
	return nil
}

func loadTrace(path string) (*TraceFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var trace TraceFile
	if err := json.Unmarshal(data, &trace); err != nil {
		return nil, err
	}

	return &trace, nil
}

func containsKeyword(text string, keywords []string) bool {
	text = strings.ToLower(text)
	for _, kw := range keywords {
		if strings.Contains(text, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// ==================== HTTP API ====================

func startAPIServer() {
	// 静态文件
	http.Handle("/", http.FileServer(http.FS(staticFiles)))

	// API 路由
	http.HandleFunc("/api/tenders", handleGetTenders)
	http.HandleFunc("/api/collect", handleCollect)
	http.HandleFunc("/api/health", handleHealth)

	log.Println("🌐 Web 服务启动: http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("启动服务失败: %v", err)
	}
}

func handleGetTenders(w http.ResponseWriter, r *http.Request) {
	province := r.URL.Query().Get("province")
	keyword := r.URL.Query().Get("keyword")

	tenders, err := queryTenders(province, keyword, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    tenders,
		"count":   len(tenders),
	})
}

func handleCollect(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Province string   `json:"province"`
		Keywords []string `json:"keywords"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 异步执行采集任务
	go runCollectTask(req.Province, req.Keywords)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "采集任务已启动",
	})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"service": "tender-monitor",
		"version": "1.0.0",
	})
}

// ==================== 主函数 ====================

func main() {
	log.Println("="*60)
	log.Println("🚀 招标信息监控系统")
	log.Println("="*60)

	// 初始化数据库
	if err := initDB(); err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	defer db.Close()

	// 检查验证码服务
	solver := NewCaptchaSolver(captchaService)
	if solver.CheckAvailable() {
		log.Println("✅ 验证码服务已连接")
	} else {
		log.Println("⚠️ 验证码服务不可用（将使用手动输入）")
	}

	// 确保目录存在
	os.MkdirAll(dataDir, 0755)
	os.MkdirAll(tracesDir, 0755)

	// 启动 Web 服务
	startAPIServer()
}
