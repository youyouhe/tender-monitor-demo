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
	_ "modernc.org/sqlite"
)

//go:embed static/*
var staticFiles embed.FS

// ==================== 数据结构 ====================

// Source 采集源
type Source struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Code        string `json:"code"`
	Category    string `json:"category"`
	BaseURL     string `json:"base_url"`
	Description string `json:"description"`
	IsActive    int    `json:"is_active"`
	CreatedAt   string `json:"created_at"`
}

// TraceRecord 轨迹记录
type TraceRecord struct {
	ID         int    `json:"id"`
	SourceID   int    `json:"source_id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	RawContent string `json:"raw_content"`
	ParsedURL  string `json:"parsed_url"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
}

// TagDefinition 标签定义
type TagDefinition struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	SortOrder int    `json:"sort_order"`
}

// Tender 招标信息
type Tender struct {
	ID          int       `json:"id"`
	SourceID    int       `json:"source_id"`
	Title       string    `json:"title"`
	Amount      string    `json:"amount"`
	PublishDate string    `json:"publish_date"`
	Deadline    string    `json:"deadline"`
	Contact     string    `json:"contact"`
	Phone       string    `json:"phone"`
	URL         string    `json:"url"`
	Keywords    string    `json:"keywords"`
	Content     string    `json:"content"`
	Attachments string    `json:"attachments"`
	Status      string    `json:"status"`
	Tags        string    `json:"tags"`
	Note        string    `json:"note"`
	ReviewedAt  string    `json:"reviewed_at"`
	ReviewedBy  string    `json:"reviewed_by"`
	CreatedAt   time.Time `json:"created_at"`
}

// TenderQueryParams 查询参数
type TenderQueryParams struct {
	SourceID int
	Category string
	Status   string
	Keyword  string
	DateFrom string
	DateTo   string
	Tags     string
	Limit    int
}

// TraceFile 标准轨迹格式
type TraceFile struct {
	Name  string      `json:"name"`
	Type  string      `json:"type"`
	URL   string      `json:"url"`
	Steps []TraceStep `json:"steps"`
}

// TraceStep 轨迹步骤
type TraceStep struct {
	Action         string            `json:"action"`
	URL            string            `json:"url,omitempty"`
	Selector       string            `json:"selector,omitempty"`
	XPath          string            `json:"xpath,omitempty"`
	Value          string            `json:"value,omitempty"`
	ImageSelector  string            `json:"image_selector,omitempty"`
	InputSelector  string            `json:"input_selector,omitempty"`
	Type           string            `json:"type,omitempty"`
	Fields         map[string]string `json:"fields,omitempty"`
	MultiFields    map[string]string `json:"multi_fields,omitempty"`
	WaitTime       int               `json:"wait_time,omitempty"`
	WaitForVisible string            `json:"wait_for_visible,omitempty"`
}

// ChromeDevToolsStep Chrome DevTools 录制格式
type ChromeDevToolsStep struct {
	Type      string     `json:"type"`
	URL       string     `json:"url"`
	Selectors [][]string `json:"selectors"`
}

// ChromeDevToolsRecording Chrome DevTools 录制
type ChromeDevToolsRecording struct {
	Title string               `json:"title"`
	URL   string               `json:"url"`
	Steps []ChromeDevToolsStep `json:"steps"`
}

// CaptchaResponse 验证码服务响应
type CaptchaResponse struct {
	Success    bool    `json:"success"`
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
	Error      string  `json:"error,omitempty"`
}

// Tag 标签结构体
type Tag struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	CreatedAt string `json:"created_at"`
}

// ==================== 全局变量 ====================

var (
	captchaService  = getEnv("CAPTCHA_SERVICE", "http://localhost:5000")
	dataDir         = getEnv("DATA_DIR", "./data")
	tracesDir       = getEnv("TRACES_DIR", "./traces")
	browserHeadless = getEnv("BROWSER_HEADLESS", "false") == "true"
	db              *sql.DB
)

var supportedProvinces = []string{"guangdong", "shandong"}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

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

func (cs *CaptchaSolver) Solve(imageBytes []byte) (string, error) {
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
	bodyStr := strings.TrimSpace(string(body))

	var result CaptchaResponse
	if err := json.Unmarshal(body, &result); err == nil {
		if !result.Success {
			return "", fmt.Errorf("识别失败: %s", result.Error)
		}
		return result.Text, nil
	}

	if len(bodyStr) > 0 {
		log.Printf("验证码服务返回纯文本: %s", bodyStr)
		return bodyStr, nil
	}

	return "", fmt.Errorf("验证码服务返回空响应")
}

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

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("创建数据目录失败: %v", err)
	}

	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("打开数据库失败: %v", err)
	}

	db.Exec(`CREATE TABLE IF NOT EXISTS sources (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		code TEXT UNIQUE NOT NULL,
		category TEXT NOT NULL,
		base_url TEXT,
		description TEXT,
		is_active INTEGER DEFAULT 1,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)

	db.Exec(`CREATE TABLE IF NOT EXISTS traces (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source_id INTEGER,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		raw_content TEXT,
		parsed_url TEXT,
		status TEXT DEFAULT 'draft',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (source_id) REFERENCES sources(id)
	)`)

	db.Exec(`CREATE TABLE IF NOT EXISTS tag_definitions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		color TEXT,
		sort_order INTEGER DEFAULT 0
	)`)

	db.Exec(`CREATE TABLE IF NOT EXISTS tenders (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source_id INTEGER,
		title TEXT,
		amount TEXT,
		publish_date TEXT,
		deadline TEXT,
		contact TEXT,
		phone TEXT,
		url TEXT UNIQUE,
		keywords TEXT,
		content TEXT,
		attachments TEXT,
		status TEXT DEFAULT 'active',
		tags TEXT,
		note TEXT,
		reviewed_at TEXT,
		reviewed_by TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)

	db.Exec(`CREATE INDEX IF NOT EXISTS idx_source_id ON tenders(source_id)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_publish_date ON tenders(publish_date)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_status ON tenders(status)`)

	migrateTendersTable()
	initDefaultSources()
	initDefaultTags()

	log.Println("✅ 数据库初始化成功")
	return nil
}

func migrateTendersTable() {
	migrations := []struct {
		colName string
		colType string
	}{
		{"source_id", "INTEGER"}, {"deadline", "TEXT"}, {"status", "TEXT DEFAULT 'active'"},
		{"tags", "TEXT"}, {"note", "TEXT"}, {"reviewed_at", "TEXT"}, {"reviewed_by", "TEXT"}, {"attachments", "TEXT"},
	}
	for _, m := range migrations {
		var count int
		row := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('tenders') WHERE name=?", m.colName)
		row.Scan(&count)
		if count == 0 {
			db.Exec(fmt.Sprintf("ALTER TABLE tenders ADD COLUMN %s %s", m.colName, m.colType))
		}
	}
}

func initDefaultSources() {
	sources := []struct {
		name, code, category, baseURL, desc string
	}{
		{"广东省政府采购网", "guangdong", "province", "https://gdgpo.czt.gd.gov.cn", "广东省政府采购官方网站"},
		{"山东省政府采购网", "shandong", "province", "https://www.ccgp.gov.cn", "山东省政府采购官方网站"},
		{"中国政府采购网", "govcn", "province", "http://www.ccgp.gov.cn", "中国政府采购网"},
		{"中国招标投标网", "bidcenter", "industry", "https://www.cec.gov.cn", "中国招标投标公共服务平台"},
		{"央国企采购平台", "soe", "soe", "", "央企国企采购信息汇总"},
	}
	for _, s := range sources {
		db.Exec(`INSERT OR IGNORE INTO sources (name, code, category, base_url, description) VALUES (?, ?, ?, ?, ?)`,
			s.name, s.code, s.category, s.baseURL, s.desc)
	}
}

func initDefaultTags() {
	tags := []struct {
		name, color string
		order       int
	}{
		{"重点关注", "#f56565", 1}, {"已跟进", "#48bb78", 2}, {"待评估", "#ecc94b", 3},
		{"放弃", "#a0aec0", 4}, {"中标", "#4299e1", 5},
	}
	for _, t := range tags {
		db.Exec(`INSERT OR IGNORE INTO tag_definitions (name, color, sort_order) VALUES (?, ?, ?)`,
			t.name, t.color, t.order)
	}
}

func saveTender(tender *Tender) error {
	query := `INSERT OR IGNORE INTO tenders (source_id, title, amount, publish_date, deadline, contact, phone, url, keywords, content, attachments, status, tags, note) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := db.Exec(query, tender.SourceID, tender.Title, tender.Amount, tender.PublishDate, tender.Deadline, tender.Contact, tender.Phone, tender.URL, tender.Keywords, tender.Content, tender.Attachments, tender.Status, tender.Tags, tender.Note)
	return err
}

func queryTenders(params TenderQueryParams) ([]Tender, error) {
	query := `SELECT id, source_id, title, amount, publish_date, deadline, contact, phone, url, keywords, content, attachments, status, tags, note, reviewed_at, reviewed_by, created_at FROM tenders WHERE 1=1`
	args := []interface{}{}

	if params.SourceID > 0 {
		query += " AND source_id = ?"
		args = append(args, params.SourceID)
	}
	if params.Category != "" {
		query += " AND source_id IN (SELECT id FROM sources WHERE category = ?)"
		args = append(args, params.Category)
	}
	if params.Status != "" {
		query += " AND status = ?"
		args = append(args, params.Status)
	}
	if params.Keyword != "" {
		query += " AND (title LIKE ? OR keywords LIKE ? OR content LIKE ?)"
		args = append(args, "%"+params.Keyword+"%", "%"+params.Keyword+"%", "%"+params.Keyword+"%")
	}
	if params.DateFrom != "" {
		query += " AND publish_date >= ?"
		args = append(args, params.DateFrom)
	}
	if params.DateTo != "" {
		query += " AND publish_date <= ?"
		args = append(args, params.DateTo)
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 100
	}
	query += " ORDER BY publish_date DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return []Tender{}, err
	}
	defer rows.Close()

	tenders := []Tender{}
	for rows.Next() {
		var t Tender
		var attachments, deadline, status, tags, note, reviewedAt, reviewedBy sql.NullString
		var sourceID sql.NullInt64
		rows.Scan(&t.ID, &sourceID, &t.Title, &t.Amount, &t.PublishDate, &deadline, &t.Contact, &t.Phone, &t.URL, &t.Keywords, &t.Content, &attachments, &status, &tags, &note, &reviewedAt, &reviewedBy, &t.CreatedAt)
		if sourceID.Valid {
			t.SourceID = int(sourceID.Int64)
		}
		if deadline.Valid {
			t.Deadline = deadline.String
		}
		if status.Valid {
			t.Status = status.String
		} else {
			t.Status = "active"
		}
		if attachments.Valid {
			t.Attachments = attachments.String
		}
		if tags.Valid {
			t.Tags = tags.String
		}
		if note.Valid {
			t.Note = note.String
		}
		if reviewedAt.Valid {
			t.ReviewedAt = reviewedAt.String
		}
		if reviewedBy.Valid {
			t.ReviewedBy = reviewedBy.String
		}
		tenders = append(tenders, t)
	}
	return tenders, nil
}

func getSourceIDByCode(code string) int {
	var id int
	err := db.QueryRow("SELECT id FROM sources WHERE code = ?", code).Scan(&id)
	if err != nil {
		return 0
	}
	return id
}

func getSourcesMap() map[int]Source {
	sources := make(map[int]Source)
	rows, _ := db.Query("SELECT id, name, code, category, base_url, description, is_active FROM sources")
	defer rows.Close()
	for rows.Next() {
		var s Source
		if err := rows.Scan(&s.ID, &s.Name, &s.Code, &s.Category, &s.BaseURL, &s.Description, &s.IsActive); err == nil {
			sources[s.ID] = s
		}
	}
	return sources
}

func getAllSources() ([]Source, error) {
	rows, err := db.Query("SELECT id, name, code, category, base_url, description, is_active, created_at FROM sources ORDER BY category, name")
	if err != nil {
		return []Source{}, err
	}
	defer rows.Close()
	sources := []Source{}
	for rows.Next() {
		var s Source
		if err := rows.Scan(&s.ID, &s.Name, &s.Code, &s.Category, &s.BaseURL, &s.Description, &s.IsActive, &s.CreatedAt); err == nil {
			sources = append(sources, s)
		}
	}
	return sources, nil
}

func saveSource(s *Source) error {
	if s.ID > 0 {
		_, err := db.Exec(`UPDATE sources SET name=?, code=?, category=?, base_url=?, description=?, is_active=? WHERE id=?`,
			s.Name, s.Code, s.Category, s.BaseURL, s.Description, s.IsActive, s.ID)
		return err
	}
	result, err := db.Exec(`INSERT INTO sources (name, code, category, base_url, description, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		s.Name, s.Code, s.Category, s.BaseURL, s.Description, s.IsActive)
	if err != nil {
		return err
	}
	id, _ := result.LastInsertId()
	s.ID = int(id)
	return nil
}

func deleteSource(id int) error {
	_, err := db.Exec("DELETE FROM sources WHERE id = ?", id)
	return err
}

func getAllTags() ([]TagDefinition, error) {
	rows, err := db.Query("SELECT id, name, color, sort_order FROM tag_definitions ORDER BY sort_order")
	if err != nil {
		return []TagDefinition{}, err
	}
	defer rows.Close()
	tags := []TagDefinition{}
	for rows.Next() {
		var t TagDefinition
		if err := rows.Scan(&t.ID, &t.Name, &t.Color, &t.SortOrder); err == nil {
			tags = append(tags, t)
		}
	}
	return tags, nil
}

func saveTag(t *TagDefinition) error {
	if t.ID > 0 {
		_, err := db.Exec(`UPDATE tag_definitions SET name=?, color=?, sort_order=? WHERE id=?`, t.Name, t.Color, t.SortOrder, t.ID)
		return err
	}
	result, err := db.Exec(`INSERT INTO tag_definitions (name, color, sort_order) VALUES (?, ?, ?)`, t.Name, t.Color, t.SortOrder)
	if err != nil {
		return err
	}
	id, _ := result.LastInsertId()
	t.ID = int(id)
	return nil
}

func updateTenderTags(id int, tags string) error {
	_, err := db.Exec("UPDATE tenders SET tags = ? WHERE id = ?", tags, id)
	return err
}

func updateTenderNote(id int, note string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err := db.Exec("UPDATE tenders SET note = ?, reviewed_at = ? WHERE id = ?", note, now, id)
	return err
}

func updateTenderStatus(id int, status string) error {
	_, err := db.Exec("UPDATE tenders SET status = ? WHERE id = ?", status, id)
	return err
}

// ==================== 轨迹解析 ====================

func parseTraceFile(content string) (*TraceFile, error) {
	var trace TraceFile
	if err := json.Unmarshal([]byte(content), &trace); err == nil {
		if len(trace.Steps) > 0 && trace.Steps[0].Action != "" {
			return &trace, nil
		}
	}

	var chrome ChromeDevToolsRecording
	if err := json.Unmarshal([]byte(content), &chrome); err != nil {
		return nil, fmt.Errorf("无法解析JSON: %v", err)
	}

	trace.Name = chrome.Title
	trace.URL = chrome.URL

	if strings.Contains(chrome.URL, "noticeGd") || strings.Contains(chrome.URL, "detail") {
		trace.Type = "detail"
	} else {
		trace.Type = "list"
	}

	for _, step := range chrome.Steps {
		if step.Type == "setViewport" {
			continue
		}
		newStep := TraceStep{
			Action: step.Type,
			URL:    step.URL,
		}
		if len(step.Selectors) > 0 && len(step.Selectors[0]) > 0 {
			sel := step.Selectors[0][0]
			if strings.HasPrefix(sel, "xpath") {
				newStep.XPath = sel
			} else if strings.HasPrefix(sel, "pierce") {
				newStep.Selector = strings.TrimPrefix(sel, "pierce/")
			} else if strings.HasPrefix(sel, "aria") {
				newStep.Selector = sel
			} else {
				newStep.Selector = sel
			}
		}
		trace.Steps = append(trace.Steps, newStep)
	}

	log.Printf("📝 Chrome DevTools 格式已转换: %d 步骤", len(trace.Steps))
	return &trace, nil
}

// ==================== 浏览器自动化 ====================

func setupBrowser() (*rod.Browser, error) {
	var l *launcher.Launcher
	userDataDir := filepath.Join(dataDir, "browser-data")
	os.MkdirAll(userDataDir, 0755)

	if browserHeadless {
		l = launcher.New().Headless(true).UserDataDir(userDataDir)
	} else {
		l = launcher.New().Headless(false).UserDataDir(userDataDir)
	}

	url := l.MustLaunch()
	browser := rod.New().ControlURL(url).MustConnect()

	log.Println("✅ 浏览器启动成功")
	return browser, nil
}

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
			page.MustElement(selector).MustClick()
			time.Sleep(500 * time.Millisecond)
		case "input":
			selector := replaceParams(step.Selector, params)
			value := replaceParams(step.Value, params)
			page.MustElement(selector).MustSelectAllText().MustInput(value)
		case "wait":
			if step.WaitTime > 0 {
				time.Sleep(time.Duration(step.WaitTime) * time.Millisecond)
			}
			if step.WaitForVisible != "" {
				page.MustElement(step.WaitForVisible).MustWaitVisible()
			}
		case "extract":
			if step.Type == "list" {
				extractedData = extractList(page, step)
			} else if step.Type == "detail" {
				extractedData = extractDetail(page, step)
			}
		}
		time.Sleep(300 * time.Millisecond)
	}

	return extractedData, nil
}

func handleCaptcha(page *rod.Page, imageSelector string, solver *CaptchaSolver) (string, error) {
	imgElem := page.MustElement(imageSelector)
	imgBytes := imgElem.MustScreenshot()

	timestamp := time.Now().Format("20060102_150405")
	captchaPath := filepath.Join(dataDir, fmt.Sprintf("captcha_%s.png", timestamp))
	os.WriteFile(captchaPath, imgBytes, 0644)
	log.Printf("验证码已保存: %s", captchaPath)

	if solver != nil && solver.CheckAvailable() {
		text, err := solver.Solve(imgBytes)
		if err == nil {
			log.Printf("✅ 自动识别成功: %s", text)
			return text, nil
		}
		log.Printf("⚠️ 自动识别失败: %v", err)
	}

	fmt.Printf("请查看验证码图片: %s\n", captchaPath)
	fmt.Print("请输入验证码: ")
	var manualInput string
	fmt.Scanln(&manualInput)
	return manualInput, nil
}

func extractList(page *rod.Page, step TraceStep) []map[string]string {
	var results []map[string]string
	time.Sleep(2 * time.Second)

	var rows []*rod.Element
	var err error

	if step.XPath != "" {
		rows, err = page.ElementsX(step.XPath)
	} else {
		rows, err = page.Elements(step.Selector)
	}

	if err != nil {
		log.Printf("提取失败: %v", err)
		return results
	}

	log.Printf("找到 %d 条记录", len(rows))
	listURL := page.MustInfo().URL

	for _, row := range rows {
		item := make(map[string]string)
		hasValidData := false

		var clickSelector string
		for field, selector := range step.Fields {
			if field == "url" && strings.HasPrefix(selector, "@click") {
				clickSelector = strings.TrimPrefix(selector, "@click:")
				if clickSelector == "" {
					clickSelector = "span"
				}
				continue
			}
			if elem, err := row.Element(selector); err == nil {
				text := elem.MustText()
				item[field] = text
				if text != "" {
					hasValidData = true
				}
			}
		}

		if clickSelector != "" && hasValidData {
			if clickElem, err := row.Element(clickSelector); err == nil {
				url := extractURLByClick(page, clickElem, listURL)
				if url != "" {
					item["url"] = url
				}
			}
		}

		if hasValidData && item["url"] != "" {
			results = append(results, item)
		}

		if len(results) >= 10 {
			log.Printf("已达到采集上限 10 条")
			break
		}
	}

	return results
}

func extractURLByClick(page *rod.Page, elem *rod.Element, returnURL string) string {
	initialURL := page.MustInfo().URL
	elem.MustClick()

	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		currentURL := page.MustInfo().URL
		if currentURL != initialURL {
			page.MustNavigate(returnURL)
			page.MustWaitLoad()
			time.Sleep(2 * time.Second)
			return currentURL
		}
	}

	browser := elem.Page().Browser()
	pages, _ := browser.Pages()
	if len(pages) > 1 {
		for _, p := range pages {
			if p.MustInfo().URL != initialURL {
				detailURL := p.MustInfo().URL
				p.Close()
				return detailURL
			}
		}
	}

	return initialURL
}

func extractDetail(page *rod.Page, step TraceStep) map[string]string {
	result := make(map[string]string)
	time.Sleep(2 * time.Second)

	for field, selector := range step.Fields {
		if elem, err := page.Element(selector); err == nil {
			result[field] = elem.MustText()
		}
	}

	for field, selector := range step.MultiFields {
		if elems, err := page.Elements(selector); err == nil {
			var links []map[string]string
			for _, elem := range elems {
				if href, _ := elem.Attribute("href"); href != nil && *href != "" {
					links = append(links, map[string]string{"url": *href, "name": elem.MustText()})
				}
			}
			if len(links) > 0 {
				jsonData, _ := json.Marshal(links)
				result[field] = string(jsonData)
			}
		}
	}

	return result
}

func replaceParams(template string, params map[string]string) string {
	result := template
	for key, value := range params {
		result = strings.ReplaceAll(result, fmt.Sprintf("{{.%s}}", key), value)
	}
	return result
}

// ==================== 采集任务 ====================

func runCollectTask(sourceID int, keywords []string) error {
	if sourceID > 0 {
		if err := collectBySource(sourceID, keywords); err != nil {
			log.Printf("❌ 采集源 %d 采集失败: %v", sourceID, err)
		}
		return nil
	}

	for _, p := range supportedProvinces {
		if err := collectSingleProvince(p, keywords); err != nil {
			log.Printf("❌ 省份 %s 采集失败: %v", p, err)
		}
	}
	return nil
}

func collectBySource(sourceID int, keywords []string) error {
	var source Source
	err := db.QueryRow("SELECT id, name, code, category, base_url FROM sources WHERE id = ?", sourceID).Scan(
		&source.ID, &source.Name, &source.Code, &source.Category, &source.BaseURL,
	)
	if err != nil {
		return fmt.Errorf("获取采集源失败: %v", err)
	}

	log.Printf("🚀 开始采集任务：采集源=%s, 关键词=%v", source.Name, keywords)

	listTrace := getTraceBySourceAndType(sourceID, "list")
	if listTrace == nil {
		return fmt.Errorf("未找到列表轨迹，请先上传轨迹文件")
	}

	detailTrace := getTraceBySourceAndType(sourceID, "detail")
	if detailTrace == nil {
		log.Printf("⚠️ 未找到详情轨迹，将使用统一轨迹模式（仅采集列表页）")
	}

	browser, err := setupBrowser()
	if err != nil {
		return err
	}
	defer browser.Close()

	solver := NewCaptchaSolver(captchaService)

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

		for i, item := range listItems {
			title := item["title"]
			if !containsKeyword(title, keywords) {
				continue
			}

			log.Printf("\n[%d/%d] 采集详情: %s", i+1, len(listItems), title)

			var detail map[string]string
			if detailTrace != nil {
				detailParams := map[string]string{"URL": item["url"]}
				detailData, err := executeTrace(browser, detailTrace, detailParams, solver)
				if err != nil {
					log.Printf("❌ 详情采集失败: %v", err)
					continue
				}
				detail = detailData.(map[string]string)
			}

			tender := &Tender{
				SourceID:    sourceID,
				Title:       title,
				PublishDate: item["date"],
				URL:         item["url"],
				Keywords:    keyword,
				Status:      "active",
			}

			if detail != nil {
				tender.Amount = detail["amount"]
				tender.Deadline = detail["deadline"]
				tender.Contact = detail["contact"]
				tender.Phone = detail["phone"]
				tender.Content = detail["content"]
				tender.Attachments = detail["attachments"]
			}

			if err := saveTender(tender); err != nil {
				log.Printf("❌ 保存失败: %v", err)
			} else {
				log.Printf("✅ 已保存到数据库")
			}

			time.Sleep(2 * time.Second)
		}
	}

	log.Println("\n✅ 采集任务完成")
	return nil
}

func collectSingleProvince(province string, keywords []string) error {
	log.Printf("🚀 开始采集任务：省份=%s, 关键词=%v", province, keywords)

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

	browser, err := setupBrowser()
	if err != nil {
		return err
	}
	defer browser.Close()

	solver := NewCaptchaSolver(captchaService)

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

		for i, item := range listItems {
			title := item["title"]
			if !containsKeyword(title, keywords) {
				continue
			}

			log.Printf("\n[%d/%d] 采集详情: %s", i+1, len(listItems), title)

			detailParams := map[string]string{"URL": item["url"]}
			detailData, err := executeTrace(browser, detailTrace, detailParams, solver)
			if err != nil {
				log.Printf("❌ 详情采集失败: %v", err)
				continue
			}

			detail := detailData.(map[string]string)

			sourceID := getSourceIDByCode(province)
			tender := &Tender{
				SourceID:    sourceID,
				Title:       title,
				Amount:      detail["amount"],
				PublishDate: item["date"],
				Deadline:    detail["deadline"],
				Contact:     detail["contact"],
				Phone:       detail["phone"],
				URL:         item["url"],
				Keywords:    keyword,
				Content:     detail["content"],
				Attachments: detail["attachments"],
				Status:      "active",
			}

			if err := saveTender(tender); err != nil {
				log.Printf("❌ 保存失败: %v", err)
			}

			time.Sleep(2 * time.Second)
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

func getTraceBySourceAndType(sourceID int, traceType string) *TraceFile {
	var rawContent string
	err := db.QueryRow("SELECT raw_content FROM traces WHERE source_id = ? AND type = ? AND status = 'active' LIMIT 1", sourceID, traceType).Scan(&rawContent)
	if err != nil {
		return nil
	}

	trace, err := parseTraceFile(rawContent)
	if err != nil {
		log.Printf("解析轨迹失败: %v", err)
		return nil
	}
	return trace
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
	http.Handle("/", http.FileServer(http.FS(staticFiles)))

	http.HandleFunc("/api/tenders", handleGetTenders)
	http.HandleFunc("/api/collect", handleCollect)
	http.HandleFunc("/api/health", handleHealth)
	http.HandleFunc("/api/sources", handleSources)
	http.HandleFunc("/api/traces", handleTraces)
	http.HandleFunc("/api/tags", handleTags)
	http.HandleFunc("/api/tender/update", handleTenderUpdate)

	log.Println("🌐 Web 服务启动: http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("启动服务失败: %v", err)
	}
}

func handleGetTenders(w http.ResponseWriter, r *http.Request) {
	params := TenderQueryParams{
		Category: r.URL.Query().Get("category"),
		Status:   r.URL.Query().Get("status"),
		Keyword:  r.URL.Query().Get("keyword"),
		DateFrom: r.URL.Query().Get("date_from"),
		DateTo:   r.URL.Query().Get("date_to"),
		Tags:     r.URL.Query().Get("tags"),
		Limit:    100,
	}
	if sourceIDStr := r.URL.Query().Get("source_id"); sourceIDStr != "" {
		if sourceID, err := parseInt(sourceIDStr); err == nil {
			params.SourceID = sourceID
		}
	}

	tenders, err := queryTenders(params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sources := getSourcesMap()

	type TenderResponse struct {
		Tender
		SourceName string `json:"source_name"`
		SourceType string `json:"source_type"`
	}
	var response []TenderResponse
	for _, t := range tenders {
		tr := TenderResponse{Tender: t}
		if src, ok := sources[t.SourceID]; ok {
			tr.SourceName = src.Name
			tr.SourceType = src.Category
		}
		response = append(response, tr)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": response, "count": len(response)})
}

func parseInt(s string) (int, error) {
	var i int
	_, err := fmt.Sscanf(s, "%d", &i)
	return i, err
}

func handleCollect(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SourceID int      `json:"source_id"`
		Keywords []string `json:"keywords"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	go runCollectTask(req.SourceID, req.Keywords)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "采集任务已启动"})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "service": "tender-monitor", "version": "1.0.0"})
}

func handleSources(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		sources, err := getAllSources()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": sources})
	case "POST":
		var s Source
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := saveSource(&s); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": s})
	case "DELETE":
		if id, err := parseInt(r.URL.Query().Get("id")); err == nil {
			deleteSource(id)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleTraces(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		rows, err := db.Query("SELECT id, source_id, name, type, parsed_url, status, created_at FROM traces ORDER BY id DESC")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		var traces []map[string]interface{}
		for rows.Next() {
			var t TraceRecord
			rows.Scan(&t.ID, &t.SourceID, &t.Name, &t.Type, &t.ParsedURL, &t.Status, &t.CreatedAt)
			traces = append(traces, map[string]interface{}{"id": t.ID, "source_id": t.SourceID, "name": t.Name, "type": t.Type, "parsed_url": t.ParsedURL, "status": t.Status, "created_at": t.CreatedAt})
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": traces})
	case "POST":
		var req struct {
			RawContent string `json:"raw_content"`
			SourceID   int    `json:"source_id"`
			Name       string `json:"name"`
			Type       string `json:"type"`
			Analyze    bool   `json:"analyze"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.Analyze {
			traceData, err := parseTraceFile(req.RawContent)
			if err != nil {
				http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
				return
			}
			var parsedURL string
			for _, step := range traceData.Steps {
				if step.Action == "navigate" && step.URL != "" {
					parsedURL = step.URL
					break
				}
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": map[string]interface{}{"parsed_url": parsedURL, "type": traceData.Type, "name": traceData.Name, "step_count": len(traceData.Steps)}})
			return
		}

		traceData, err := parseTraceFile(req.RawContent)
		if err != nil {
			log.Printf("解析轨迹失败: %v", err)
			http.Error(w, "解析轨迹失败: "+err.Error(), http.StatusBadRequest)
			return
		}

		var parsedURL string
		for _, step := range traceData.Steps {
			if step.Action == "navigate" && step.URL != "" {
				parsedURL = step.URL
				break
			}
		}

		sourceID := req.SourceID
		if sourceID < 0 {
			sourceID = 0
		}

		var existingID int
		checkErr := db.QueryRow("SELECT id FROM traces WHERE source_id = ? AND type = ?", sourceID, req.Type).Scan(&existingID)
		if checkErr == nil {
			_, err = db.Exec(`UPDATE traces SET name=?, raw_content=?, parsed_url=?, status='active' WHERE id=?`,
				req.Name, req.RawContent, parsedURL, existingID)
			if err != nil {
				log.Printf("更新轨迹失败: %v", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			log.Printf("轨迹已更新: source_id=%d, type=%s", sourceID, req.Type)
		} else {
			_, err = db.Exec(`INSERT INTO traces (source_id, name, type, raw_content, parsed_url, status) VALUES (?, ?, ?, ?, ?, ?)`,
				sourceID, req.Name, req.Type, req.RawContent, parsedURL, "active")
			if err != nil {
				log.Printf("保存轨迹失败: %v", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	case "DELETE":
		if delID, delErr := parseInt(r.URL.Query().Get("id")); delErr == nil {
			_, delExecErr := db.Exec("DELETE FROM traces WHERE id = ?", delID)
			if delExecErr != nil {
				log.Printf("删除轨迹失败: %v", delExecErr)
			}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleTags(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		tags, err := getAllTags()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": tags})
	case "POST":
		var t TagDefinition
		if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := saveTag(&t); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": t})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleTenderUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "PUT" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID     int    `json:"id"`
		Tags   string `json:"tags"`
		Note   string `json:"note"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Tags != "" {
		updateTenderTags(req.ID, req.Tags)
	}
	if req.Note != "" {
		updateTenderNote(req.ID, req.Note)
	}
	if req.Status != "" {
		updateTenderStatus(req.ID, req.Status)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

// ==================== 主函数 ====================

func main() {
	log.Println(strings.Repeat("=", 60))
	log.Println("🚀 招标信息监控系统")
	log.Println(strings.Repeat("=", 60))

	if err := initDB(); err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	defer db.Close()

	solver := NewCaptchaSolver(captchaService)
	if solver.CheckAvailable() {
		log.Println("✅ 验证码服务已连接")
	} else {
		log.Println("⚠️ 验证码服务不可用（将使用手动输入）")
	}

	os.MkdirAll(dataDir, 0755)
	os.MkdirAll(tracesDir, 0755)

	startAPIServer()
}
