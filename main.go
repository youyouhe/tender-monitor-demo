package main

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
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

// CollectTask 采集任务
type CollectTask struct {
	ID         string    `json:"id"`
	SourceID   int       `json:"source_id"`
	SourceName string    `json:"source_name"`
	Keywords   string    `json:"keywords"` // JSON数组字符串
	Status     string    `json:"status"`   // pending/running/completed/failed/cancelled
	Progress   int       `json:"progress"` // 0-100
	Found      int       `json:"found"`    // 发现的条数
	Saved      int       `json:"saved"`    // 保存的条数
	Message    string    `json:"message"`  // 状态消息或错误信息
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	CompletedAt string   `json:"completed_at,omitempty"`
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
	SourceID  int
	Category  string
	Status    string
	Keyword   string
	MatchMode string // 关键词匹配模式: any/all/exact
	DateFrom  string
	DateTo    string
	Tags      string
	Limit     int // 每页记录数
	Offset    int // 偏移量（跳过前N条）
	Page      int // 页码（从1开始，用于计算Offset）
}

// TenderQueryResult 查询结果
type TenderQueryResult struct {
	Data       []Tender `json:"data"`
	Total      int      `json:"total"`       // 总记录数
	Page       int      `json:"page"`        // 当前页码
	PageSize   int      `json:"page_size"`   // 每页记录数
	TotalPages int      `json:"total_pages"` // 总页数
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
	Value     string     `json:"value"` // change事件的输入值
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

	// 任务取消管理器
	taskCancelers = make(map[string]context.CancelFunc)
	taskMutex     sync.RWMutex
)

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// registerTaskCanceler 注册任务取消函数
func registerTaskCanceler(taskID string, cancel context.CancelFunc) {
	taskMutex.Lock()
	defer taskMutex.Unlock()
	taskCancelers[taskID] = cancel
}

// unregisterTaskCanceler 注销任务取消函数
func unregisterTaskCanceler(taskID string) {
	taskMutex.Lock()
	defer taskMutex.Unlock()
	delete(taskCancelers, taskID)
}

// cancelTask 取消指定任务
func cancelTask(taskID string) error {
	taskMutex.RLock()
	cancel, exists := taskCancelers[taskID]
	taskMutex.RUnlock()

	if !exists {
		return fmt.Errorf("任务不存在或已完成")
	}

	cancel()
	updateCollectTask(taskID, map[string]interface{}{
		"status":       "cancelled",
		"message":      "用户手动取消",
		"completed_at": time.Now().Format("2006-01-02 15:04:05"),
	})
	return nil
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

	db.Exec(`CREATE TABLE IF NOT EXISTS collect_tasks (
		id TEXT PRIMARY KEY,
		source_id INTEGER,
		source_name TEXT,
		keywords TEXT,
		status TEXT DEFAULT 'pending',
		progress INTEGER DEFAULT 0,
		found INTEGER DEFAULT 0,
		saved INTEGER DEFAULT 0,
		message TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		completed_at TIMESTAMP,
		FOREIGN KEY (source_id) REFERENCES sources(id)
	)`)

	db.Exec(`CREATE INDEX IF NOT EXISTS idx_task_status ON collect_tasks(status)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_task_created ON collect_tasks(created_at)`)

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

// SaveTenderResult 保存招标信息的结果
type SaveTenderResult struct {
	IsNew   bool   // 是否是新记录
	Updated bool   // 是否更新了已有记录
	Action  string // "created" / "updated" / "skipped"
}

func saveTender(tender *Tender) (*SaveTenderResult, error) {
	// 查询是否已存在
	var existingID int
	var existingAmount, existingDeadline, existingContact, existingPhone, existingContent, existingAttachments sql.NullString

	err := db.QueryRow(`
		SELECT id, amount, deadline, contact, phone, content, attachments
		FROM tenders WHERE url = ?
	`, tender.URL).Scan(&existingID, &existingAmount, &existingDeadline, &existingContact, &existingPhone, &existingContent, &existingAttachments)

	if err == sql.ErrNoRows {
		// 不存在，插入新记录
		_, err = db.Exec(`
			INSERT INTO tenders (source_id, title, amount, publish_date, deadline, contact, phone, url, keywords, content, attachments, status, tags, note)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, tender.SourceID, tender.Title, tender.Amount, tender.PublishDate, tender.Deadline, tender.Contact, tender.Phone, tender.URL, tender.Keywords, tender.Content, tender.Attachments, tender.Status, tender.Tags, tender.Note)

		if err != nil {
			return nil, fmt.Errorf("插入失败: %v", err)
		}

		return &SaveTenderResult{IsNew: true, Updated: false, Action: "created"}, nil
	}

	if err != nil {
		return nil, fmt.Errorf("查询失败: %v", err)
	}

	// 记录已存在，检查是否需要更新
	needsUpdate := false

	// 比较关键字段，如果新数据有值且与旧数据不同，则需要更新
	if tender.Amount != "" && (!existingAmount.Valid || existingAmount.String != tender.Amount) {
		needsUpdate = true
	}
	if tender.Deadline != "" && (!existingDeadline.Valid || existingDeadline.String != tender.Deadline) {
		needsUpdate = true
	}
	if tender.Contact != "" && (!existingContact.Valid || existingContact.String != tender.Contact) {
		needsUpdate = true
	}
	if tender.Phone != "" && (!existingPhone.Valid || existingPhone.String != tender.Phone) {
		needsUpdate = true
	}
	if tender.Content != "" && (!existingContent.Valid || existingContent.String != tender.Content) {
		needsUpdate = true
	}
	if tender.Attachments != "" && (!existingAttachments.Valid || existingAttachments.String != tender.Attachments) {
		needsUpdate = true
	}

	if !needsUpdate {
		// 数据没有变化，跳过
		return &SaveTenderResult{IsNew: false, Updated: false, Action: "skipped"}, nil
	}

	// 更新记录（只更新有值的字段）
	setClauses := []string{}
	args := []interface{}{}

	if tender.Amount != "" {
		setClauses = append(setClauses, "amount = ?")
		args = append(args, tender.Amount)
	}
	if tender.Deadline != "" {
		setClauses = append(setClauses, "deadline = ?")
		args = append(args, tender.Deadline)
	}
	if tender.Contact != "" {
		setClauses = append(setClauses, "contact = ?")
		args = append(args, tender.Contact)
	}
	if tender.Phone != "" {
		setClauses = append(setClauses, "phone = ?")
		args = append(args, tender.Phone)
	}
	if tender.Content != "" {
		setClauses = append(setClauses, "content = ?")
		args = append(args, tender.Content)
	}
	if tender.Attachments != "" {
		setClauses = append(setClauses, "attachments = ?")
		args = append(args, tender.Attachments)
	}

	// 始终更新关键词（追加模式）
	if tender.Keywords != "" {
		setClauses = append(setClauses, "keywords = ?")
		// 如果已有关键词，追加新关键词（避免重复）
		existingKeywords := ""
		db.QueryRow("SELECT keywords FROM tenders WHERE id = ?", existingID).Scan(&existingKeywords)
		if existingKeywords != "" && !strings.Contains(existingKeywords, tender.Keywords) {
			args = append(args, existingKeywords+","+tender.Keywords)
		} else {
			args = append(args, tender.Keywords)
		}
	}

	if len(setClauses) == 0 {
		return &SaveTenderResult{IsNew: false, Updated: false, Action: "skipped"}, nil
	}

	args = append(args, existingID)
	query := fmt.Sprintf("UPDATE tenders SET %s WHERE id = ?", strings.Join(setClauses, ", "))
	_, err = db.Exec(query, args...)

	if err != nil {
		return nil, fmt.Errorf("更新失败: %v", err)
	}

	return &SaveTenderResult{IsNew: false, Updated: true, Action: "updated"}, nil
}

func queryTenders(params TenderQueryParams) (*TenderQueryResult, error) {
	// 构建WHERE子句
	whereClause := "WHERE 1=1"
	args := []interface{}{}

	if params.SourceID > 0 {
		whereClause += " AND source_id = ?"
		args = append(args, params.SourceID)
	}
	if params.Category != "" {
		whereClause += " AND source_id IN (SELECT id FROM sources WHERE category = ?)"
		args = append(args, params.Category)
	}
	if params.Status != "" {
		whereClause += " AND status = ?"
		args = append(args, params.Status)
	}
	if params.Keyword != "" {
		// 解析关键词（支持空格、逗号、分号分隔）
		keywords := strings.FieldsFunc(params.Keyword, func(r rune) bool {
			return r == ',' || r == '，' || r == ';' || r == '；' || r == ' '
		})

		if len(keywords) > 0 {
			matchMode := KeywordMatchMode(params.MatchMode)
			if matchMode == "" {
				matchMode = MatchModeAny
			}

			switch matchMode {
			case MatchModeAll:
				// AND逻辑：所有关键词都必须匹配
				for _, kw := range keywords {
					whereClause += " AND (title LIKE ? OR keywords LIKE ? OR content LIKE ?)"
					args = append(args, "%"+kw+"%", "%"+kw+"%", "%"+kw+"%")
				}
			case MatchModeExact:
				// 精确匹配：标题完全等于关键词
				placeholders := make([]string, len(keywords))
				for i, kw := range keywords {
					placeholders[i] = "?"
					args = append(args, kw)
				}
				whereClause += fmt.Sprintf(" AND title IN (%s)", strings.Join(placeholders, ","))
			default: // MatchModeAny
				// OR逻辑：匹配任意一个关键词
				conditions := []string{}
				for _, kw := range keywords {
					conditions = append(conditions, "(title LIKE ? OR keywords LIKE ? OR content LIKE ?)")
					args = append(args, "%"+kw+"%", "%"+kw+"%", "%"+kw+"%")
				}
				whereClause += " AND (" + strings.Join(conditions, " OR ") + ")"
			}
		}
	}
	if params.DateFrom != "" {
		whereClause += " AND publish_date >= ?"
		args = append(args, params.DateFrom)
	}
	if params.DateTo != "" {
		whereClause += " AND publish_date <= ?"
		args = append(args, params.DateTo)
	}

	// 查询总记录数
	countQuery := "SELECT COUNT(*) FROM tenders " + whereClause
	var total int
	err := db.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("查询总数失败: %v", err)
	}

	// 处理分页参数
	limit := params.Limit
	if limit <= 0 {
		limit = 20 // 默认每页20条
	}
	if limit > 100 {
		limit = 100 // 最大100条
	}

	// 如果提供了Page，则计算Offset
	offset := params.Offset
	page := params.Page
	if page > 0 {
		offset = (page - 1) * limit
	} else if offset < 0 {
		offset = 0
	}

	// 查询数据
	dataQuery := `SELECT id, source_id, title, amount, publish_date, deadline, contact, phone, url, keywords, content, attachments, status, tags, note, reviewed_at, reviewed_by, created_at FROM tenders ` + whereClause + " ORDER BY publish_date DESC LIMIT ? OFFSET ?"
	dataArgs := append(args, limit, offset)

	rows, err := db.Query(dataQuery, dataArgs...)
	if err != nil {
		return nil, fmt.Errorf("查询数据失败: %v", err)
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

	// 计算总页数
	totalPages := (total + limit - 1) / limit
	if page <= 0 {
		page = 1
	}

	return &TenderQueryResult{
		Data:       tenders,
		Total:      total,
		Page:       page,
		PageSize:   limit,
		TotalPages: totalPages,
	}, nil
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

// exportTendersToCSV 导出招标数据为CSV格式
func exportTendersToCSV(w http.ResponseWriter, params TenderQueryParams) error {
	// 限制导出数量，防止内存溢出
	maxExportLimit := 10000
	if params.Limit <= 0 || params.Limit > maxExportLimit {
		params.Limit = maxExportLimit
	}
	params.Offset = 0 // 导出时不使用分页偏移

	result, err := queryTenders(params)
	if err != nil {
		return err
	}

	sources := getSourcesMap()
	filename := fmt.Sprintf("tenders_export_%s.csv", time.Now().Format("20060102_150405"))

	// 设置响应头
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))

	// 写入UTF-8 BOM，确保Excel正确识别中文
	w.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(w)
	defer writer.Flush()

	// 写入表头
	headers := []string{
		"ID", "采集源", "标题", "金额", "发布日期", "截止日期",
		"联系人", "联系电话", "URL", "关键词", "状态", "标签", "备注",
	}
	if err := writer.Write(headers); err != nil {
		return err
	}

	// 写入数据行
	for _, t := range result.Data {
		sourceName := "未知源"
		if src, ok := sources[t.SourceID]; ok {
			sourceName = src.Name
		}

		row := []string{
			fmt.Sprintf("%d", t.ID),
			sourceName,
			t.Title,
			t.Amount,
			t.PublishDate,
			t.Deadline,
			t.Contact,
			t.Phone,
			t.URL,
			t.Keywords,
			t.Status,
			t.Tags,
			t.Note,
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
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

// ==================== 采集任务管理 ====================

func createCollectTask(sourceID int, keywords []string) (*CollectTask, error) {
	// 生成任务ID
	taskID := fmt.Sprintf("task_%d_%d", sourceID, time.Now().Unix())

	// 获取source名称
	var sourceName string
	db.QueryRow("SELECT name FROM sources WHERE id = ?", sourceID).Scan(&sourceName)

	// 将关键词数组转为JSON字符串
	keywordsJSON, _ := json.Marshal(keywords)

	task := &CollectTask{
		ID:         taskID,
		SourceID:   sourceID,
		SourceName: sourceName,
		Keywords:   string(keywordsJSON),
		Status:     "pending",
		Progress:   0,
		Found:      0,
		Saved:      0,
		Message:    "任务已创建，等待执行",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	_, err := db.Exec(`
		INSERT INTO collect_tasks (id, source_id, source_name, keywords, status, progress, found, saved, message, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, task.ID, task.SourceID, task.SourceName, task.Keywords, task.Status, task.Progress, task.Found, task.Saved, task.Message, task.CreatedAt, task.UpdatedAt)

	if err != nil {
		return nil, err
	}

	return task, nil
}

func updateCollectTask(taskID string, updates map[string]interface{}) error {
	setClauses := []string{"updated_at = ?"}
	args := []interface{}{time.Now()}

	allowedFields := map[string]bool{
		"status": true, "progress": true, "found": true, "saved": true,
		"message": true, "completed_at": true,
	}

	for key, value := range updates {
		if allowedFields[key] {
			setClauses = append(setClauses, fmt.Sprintf("%s = ?", key))
			args = append(args, value)
		}
	}

	args = append(args, taskID)
	query := fmt.Sprintf("UPDATE collect_tasks SET %s WHERE id = ?", strings.Join(setClauses, ", "))
	_, err := db.Exec(query, args...)
	return err
}

func getCollectTask(taskID string) (*CollectTask, error) {
	var task CollectTask
	var completedAt sql.NullString

	err := db.QueryRow(`
		SELECT id, source_id, source_name, keywords, status, progress, found, saved, message, created_at, updated_at, completed_at
		FROM collect_tasks WHERE id = ?
	`, taskID).Scan(&task.ID, &task.SourceID, &task.SourceName, &task.Keywords, &task.Status,
		&task.Progress, &task.Found, &task.Saved, &task.Message, &task.CreatedAt, &task.UpdatedAt, &completedAt)

	if err != nil {
		return nil, err
	}

	if completedAt.Valid {
		task.CompletedAt = completedAt.String
	}

	return &task, nil
}

func getAllCollectTasks(limit int) ([]CollectTask, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := db.Query(`
		SELECT id, source_id, source_name, keywords, status, progress, found, saved, message, created_at, updated_at, completed_at
		FROM collect_tasks ORDER BY created_at DESC LIMIT ?
	`, limit)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := []CollectTask{}
	for rows.Next() {
		var task CollectTask
		var completedAt sql.NullString

		if err := rows.Scan(&task.ID, &task.SourceID, &task.SourceName, &task.Keywords, &task.Status,
			&task.Progress, &task.Found, &task.Saved, &task.Message, &task.CreatedAt, &task.UpdatedAt, &completedAt); err == nil {

			if completedAt.Valid {
				task.CompletedAt = completedAt.String
			}
			tasks = append(tasks, task)
		}
	}

	return tasks, nil
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

	// 智能推断轨迹类型
	if strings.Contains(chrome.URL, "detail") || strings.Contains(chrome.Title, "详情") {
		trace.Type = "detail"
	} else {
		trace.Type = "list"
	}

	// 使用高级转换逻辑（与convert-trace工具一致）
	trace.Steps = convertChromeStepsAdvanced(chrome.Steps, trace.Type, chrome.URL)

	log.Printf("📝 Chrome DevTools 格式已转换: %d 步骤 → %d 步骤", len(chrome.Steps), len(trace.Steps))
	return &trace, nil
}

// convertChromeStepsAdvanced 高级轨迹转换（整合convert-trace工具逻辑）
func convertChromeStepsAdvanced(chromeSteps []ChromeDevToolsStep, traceType string, baseURL string) []TraceStep {
	// 中间步骤结构
	type intermediateStep struct {
		Type     string
		Selector string
		Value    string
		URL      string
	}

	var intermediate []intermediateStep
	pendingChanges := make(map[string]string) // 合并同一输入框的多次change事件
	var listSelector string
	var listFieldInfo struct {
		titleSelector string
		dateSelector  string
		urlSelector   string
	}

	// 刷新待处理的输入事件
	flushPendingChanges := func() {
		for selector, value := range pendingChanges {
			if value != "" {
				intermediate = append(intermediate, intermediateStep{
					Type:     "input",
					Selector: selector,
					Value:    value,
				})
			}
		}
		pendingChanges = make(map[string]string)
	}

	// 第一遍：分析步骤，检测列表结构
	for i, step := range chromeSteps {
		if step.Type == "click" && i < len(chromeSteps)-1 {
			nextStep := chromeSteps[i+1]
			// 检测列表行点击（导致页面跳转）
			if nextStep.Type == "navigate" {
				selector := extractBestSelector(step.Selectors)
				if isListRowClick(selector) {
					listSelector = inferListSelector(selector)
					listFieldInfo = inferListFields(step.Selectors)
				}
			}
		}
	}

	// 第二遍：转换步骤（保守策略：保留为主，删除为辅）
	for i, step := range chromeSteps {
		// 只跳过明确无用的步骤
		if shouldSkipStep(step.Type) {
			continue
		}

		switch step.Type {
		case "navigate":
			flushPendingChanges()
			intermediate = append(intermediate, intermediateStep{
				Type: "navigate",
				URL:  step.URL,
			})

		case "click":
			selector := extractBestSelector(step.Selectors)
			if selector == "" {
				continue
			}

			// 只跳过会导致页面跳转的列表行点击（后面紧跟navigate）
			if i < len(chromeSteps)-1 && chromeSteps[i+1].Type == "navigate" {
				if isListRowClick(selector) {
					continue
				}
			}

			// 检查后续是否有同一元素的change事件
			// 如果有，则跳过click（change会被转为input）
			// 需要跳过中间被过滤的步骤（keyDown/keyUp等）
			skipClick := false
			for j := i + 1; j < len(chromeSteps); j++ {
				futureStep := chromeSteps[j]
				// 跳过会被过滤的步骤
				if shouldSkipStep(futureStep.Type) {
					continue
				}
				// 找到下一个有效步骤
				if futureStep.Type == "change" {
					nextSelector := extractBestSelector(futureStep.Selectors)
					if nextSelector == selector {
						skipClick = true
					}
				}
				// 只检查紧接着的有效步骤
				break
			}

			if !skipClick {
				flushPendingChanges()
				intermediate = append(intermediate, intermediateStep{
					Type:     "click",
					Selector: selector,
				})
			}

		case "change":
			selector := extractBestSelector(step.Selectors)
			if selector != "" {
				pendingChanges[selector] = step.Value
			}
		}
	}

	flushPendingChanges()

	// 第三遍：构建最终步骤并优化
	var result []TraceStep

	for i, step := range intermediate {
		switch step.Type {
		case "navigate":
			result = append(result, TraceStep{
				Action: "navigate",
				URL:    step.URL,
			})
			result = append(result, TraceStep{
				Action:   "wait",
				WaitTime: 2000,
			})

		case "click":
			result = append(result, TraceStep{
				Action:   "click",
				Selector: step.Selector,
			})
			// 点击后的等待时间
			waitTime := 2000 // 默认2秒，足够动画和元素加载
			if isSearchButton(step.Selector) {
				waitTime = 3000 // 查询按钮等待3秒
			}
			result = append(result, TraceStep{
				Action:   "wait",
				WaitTime: waitTime,
			})

		case "input":
			value := step.Value

			// 智能识别关键词输入框
			if isKeywordInputField(step.Selector) {
				value = "{{.Keyword}}"
			}

			// 智能识别验证码输入
			if isCaptchaInput(step.Selector, step.Value) {
				// 查找前一个点击（可能是验证码图片）
				imgSelector := "img[src*='captcha']"
				for j := i - 1; j >= 0; j-- {
					if intermediate[j].Type == "click" {
						imgSelector = intermediate[j].Selector
						// 移除这个点击步骤（因为会被captcha步骤替代）
						if len(result) > 0 && result[len(result)-1].Action == "click" {
							result = result[:len(result)-1]
							if len(result) > 0 && result[len(result)-1].Action == "wait" {
								result = result[:len(result)-1]
							}
						}
						break
					}
				}

				result = append(result, TraceStep{
					Action:        "captcha",
					ImageSelector: imgSelector,
					InputSelector: step.Selector,
				})
			} else {
				result = append(result, TraceStep{
					Action:   "input",
					Selector: step.Selector,
					Value:    value,
				})
			}
		}
	}

	// 自动添加数据提取步骤
	if traceType == "list" {
		// 使用分析出的列表结构
		if listSelector == "" {
			listSelector = "tbody tr" // 默认值
		}
		fields := map[string]string{
			"title": listFieldInfo.titleSelector,
			"date":  listFieldInfo.dateSelector,
			"url":   listFieldInfo.urlSelector,
		}
		if fields["title"] == "" {
			fields = map[string]string{
				"title": "td:nth-child(1) span",
				"date":  "td:nth-child(3)",
				"url":   "td:nth-child(1) span",
			}
		}

		result = append(result, TraceStep{
			Action:   "extract",
			Type:     "list",
			Selector: listSelector,
			Fields:   fields,
		})
	} else if traceType == "detail" {
		result = append(result, TraceStep{
			Action: "extract",
			Type:   "detail",
			Fields: map[string]string{
				"amount":  "span:contains('预算金额')",
				"contact": "span:contains('联系人')",
				"phone":   "span:contains('联系电话')",
			},
		})
	}

	return result
}

// shouldSkipStep 判断是否应跳过该步骤
func shouldSkipStep(stepType string) bool {
	skipTypes := []string{"setViewport", "keyDown", "keyUp", "scroll"}
	for _, t := range skipTypes {
		if stepType == t {
			return true
		}
	}
	return false
}

// extractBestSelector 智能选择最佳选择器
func extractBestSelector(selectors [][]string) string {
	if len(selectors) == 0 {
		return ""
	}

	var selectedSelector string
	var fallbackSelector string // 降级选择器（即使是动态的）
	var ariaPlaceholder string   // 从 aria 选择器提取的 placeholder
	var priority int             // 优先级：3=ID, 2=CSS, 1=XPath, 0=其他

	for _, selectorGroup := range selectors {
		if len(selectorGroup) == 0 {
			continue
		}
		sel := selectorGroup[0]

		// 提取 aria 选择器中的 placeholder 信息
		if strings.HasPrefix(sel, "aria/") {
			ariaText := strings.TrimPrefix(sel, "aria/")
			if strings.Contains(ariaText, "请输入") {
				ariaPlaceholder = ariaText
			}
			continue
		}

		// 跳过 text 选择器
		if strings.HasPrefix(sel, "text/") {
			continue
		}

		// 处理 Pierce 选择器
		if strings.HasPrefix(sel, "pierce/") {
			sel = strings.TrimPrefix(sel, "pierce/")
		}

		// 保存第一个可用选择器作为降级选项
		if fallbackSelector == "" {
			fallbackSelector = sel
		}

		// XPath 选择器（需排除基于动态 ID 的 XPath）
		if strings.HasPrefix(sel, "xpath") {
			// 检查 XPath 中是否包含动态 ID
			isDynamic := false
			dynamicPrefixes := []string{"el-id-", "mui-", "rc-", "headlessui-"}
			for _, prefix := range dynamicPrefixes {
				if strings.Contains(sel, prefix) {
					isDynamic = true
					break
				}
			}
			// 只使用稳定的 XPath
			if !isDynamic && priority < 1 {
				selectedSelector = sel
				priority = 1
			}
			continue
		}

		// ID 选择器（需排除动态生成的 ID）
		if strings.Contains(sel, "#") && !strings.Contains(sel, "xpath") {
			// 检查是否是动态生成的 ID（Element UI、Material UI、React 等）
			isDynamic := false
			dynamicPrefixes := []string{"el-id-", "mui-", "rc-", "headlessui-"}
			for _, prefix := range dynamicPrefixes {
				if strings.Contains(sel, prefix) {
					isDynamic = true
					break
				}
			}
			// 只有稳定的 ID 才使用最高优先级
			if !isDynamic {
				selectedSelector = sel
				priority = 3
				break
			}
		}

		// 标准 CSS 选择器
		if priority < 2 {
			selectedSelector = sel
			priority = 2
		}
	}

	// 如果没有找到稳定的选择器，尝试从 aria 生成
	if selectedSelector == "" && ariaPlaceholder != "" {
		// 从 aria 文本生成基于 placeholder 的选择器
		placeholderText := strings.TrimPrefix(ariaPlaceholder, "请输入")
		if placeholderText != "" {
			generatedSelector := fmt.Sprintf("input[placeholder*=\"%s\"]", placeholderText)
			log.Printf("✨ 从 aria 生成稳定选择器: %s", generatedSelector)
			return generatedSelector
		}
	}

	// 如果没有找到稳定的选择器，使用降级选择器
	if selectedSelector == "" && fallbackSelector != "" {
		log.Printf("⚠️ 未找到稳定选择器，使用降级选择器: %s (可能包含动态ID，需手动验证)", fallbackSelector)
		return fallbackSelector
	}

	return selectedSelector
}

// isListRowClick 判断是否是列表行点击
func isListRowClick(selector string) bool {
	patterns := []string{
		"tr:nth-of-type", "tbody tr", "td:nth-of-type",
		"li:nth-of-type", ".list-item", ".item",
	}
	for _, p := range patterns {
		if strings.Contains(selector, p) {
			return true
		}
	}
	return false
}

// inferListSelector 从行选择器推断列表容器选择器
func inferListSelector(rowSelector string) string {
	if strings.Contains(rowSelector, "tr:nth-of-type") || strings.Contains(rowSelector, "tbody") {
		return "tbody tr"
	}
	if strings.Contains(rowSelector, "li:nth-of-type") {
		return "ul li"
	}
	return "tbody tr"
}

// inferListFields 从选择器推断列表字段映射
func inferListFields(selectors [][]string) struct {
	titleSelector string
	dateSelector  string
	urlSelector   string
} {
	result := struct {
		titleSelector string
		dateSelector  string
		urlSelector   string
	}{
		titleSelector: "td:nth-child(1) span",
		dateSelector:  "td:nth-child(3)",
		urlSelector:   "td:nth-child(1) span",
	}

	// 从 xpath 中解析列索引
	for _, selectorGroup := range selectors {
		for _, sel := range selectorGroup {
			if strings.HasPrefix(sel, "xpath") && strings.Contains(sel, "/td[") {
				// 解析: xpath///.../td[3]/...
				if idx := strings.Index(sel, "/td["); idx != -1 {
					rest := sel[idx+4:]
					if end := strings.Index(rest, "]"); end != -1 {
						var colNum int
						if _, err := fmt.Sscanf(rest[:end], "%d", &colNum); err == nil {
							result.titleSelector = fmt.Sprintf("td:nth-child(%d) span", colNum)
							result.urlSelector = fmt.Sprintf("td:nth-child(%d) span", colNum)
						}
					}
				}
			}
		}
	}

	return result
}

// isInputClick 判断是否是输入框点击
func isInputClick(selector string) bool {
	return strings.Contains(selector, "input") ||
		strings.Contains(selector, "[role=\"textbox\"]")
}

// isSearchButton 判断是否是查询按钮
func isSearchButton(selector string) bool {
	return strings.Contains(selector, "button") &&
		(strings.Contains(selector, "primary") ||
			strings.Contains(selector, "search") ||
			strings.Contains(selector, "查询"))
}

// isKeywordInputField 判断是否是关键词输入框
func isKeywordInputField(selector string) bool {
	keywords := []string{"标题", "关键词", "keyword", "title", "搜索", "search"}
	selectorLower := strings.ToLower(selector)
	for _, kw := range keywords {
		if strings.Contains(selectorLower, kw) {
			return true
		}
	}
	return false
}

// isCaptchaInput 判断是否是验证码输入
func isCaptchaInput(selector string, value string) bool {
	if strings.Contains(selector, "验证码") || strings.Contains(selector, "captcha") {
		return true
	}
	// 4位数字/字母组合通常是验证码
	if len(value) == 4 && !strings.Contains(value, " ") {
		return true
	}
	return false
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

	// 设置全局超时时间为30秒
	page = page.Timeout(30 * time.Second)

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
			log.Printf("🔍 查找元素: %s", selector)
			elem, err := page.Element(selector)
			if err != nil {
				return nil, fmt.Errorf("找不到点击元素 '%s': %v", selector, err)
			}
			if err := elem.Click(proto.InputMouseButtonLeft, 1); err != nil {
				return nil, fmt.Errorf("点击失败: %v", err)
			}
			time.Sleep(500 * time.Millisecond)
		case "input":
			selector := replaceParams(step.Selector, params)
			value := replaceParams(step.Value, params)
			log.Printf("🔍 查找输入框: %s", selector)
			elem, err := page.Element(selector)
			if err != nil {
				return nil, fmt.Errorf("找不到输入元素 '%s': %v", selector, err)
			}
			if err := elem.SelectAllText(); err != nil {
				log.Printf("⚠️ SelectAllText 失败（可能是空输入框）: %v", err)
			}
			if err := elem.Input(value); err != nil {
				return nil, fmt.Errorf("输入失败: %v", err)
			}
		case "wait":
			if step.WaitTime > 0 {
				time.Sleep(time.Duration(step.WaitTime) * time.Millisecond)
			}
			if step.WaitForVisible != "" {
				log.Printf("🔍 等待元素可见: %s", step.WaitForVisible)
				elem, err := page.Element(step.WaitForVisible)
				if err != nil {
					return nil, fmt.Errorf("等待元素失败 '%s': %v", step.WaitForVisible, err)
				}
				if err := elem.WaitVisible(); err != nil {
					return nil, fmt.Errorf("元素未变为可见: %v", err)
				}
			}
		case "captcha":
			if step.ImageSelector == "" || step.InputSelector == "" {
				return nil, fmt.Errorf("captcha action 缺少必要参数: image_selector 或 input_selector")
			}
			captchaText, err := handleCaptcha(page, step.ImageSelector, solver)
			if err != nil {
				return nil, fmt.Errorf("验证码处理失败: %v", err)
			}
			// 输入验证码
			elem, err := page.Element(step.InputSelector)
			if err != nil {
				return nil, fmt.Errorf("找不到验证码输入框 '%s': %v", step.InputSelector, err)
			}
			if err := elem.SelectAllText(); err != nil {
				log.Printf("⚠️ SelectAllText 失败: %v", err)
			}
			if err := elem.Input(captchaText); err != nil {
				return nil, fmt.Errorf("输入验证码失败: %v", err)
			}
			log.Printf("✅ 验证码已输入")
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
	log.Printf("🔍 查找验证码图片: %s", imageSelector)
	imgElem, err := page.Element(imageSelector)
	if err != nil {
		return "", fmt.Errorf("找不到验证码图片元素 '%s': %v", imageSelector, err)
	}
	imgBytes, err := imgElem.Screenshot(proto.PageCaptureScreenshotFormatPng, 0)
	if err != nil {
		return "", fmt.Errorf("截图失败: %v", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	captchaPath := filepath.Join(dataDir, fmt.Sprintf("captcha_%s.png", timestamp))
	os.WriteFile(captchaPath, imgBytes, 0600) // 修复安全问题：文件权限改为0600
	log.Printf("验证码已保存: %s", captchaPath)

	if solver != nil && solver.CheckAvailable() {
		text, err := solver.Solve(imgBytes)
		if err == nil {
			log.Printf("✅ 自动识别成功: %s", text)
			return text, nil
		}
		log.Printf("⚠️ 自动识别失败: %v", err)
		return "", fmt.Errorf("验证码自动识别失败: %v (已保存至 %s)", err, captchaPath)
	}

	// 验证码服务不可用
	log.Printf("❌ 验证码服务不可用，已保存验证码图片: %s", captchaPath)
	return "", fmt.Errorf("验证码服务不可用，无法继续采集 (验证码已保存至 %s)", captchaPath)
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

// runCollectTaskWithTracking 带任务状态跟踪的采集任务执行器
func runCollectTaskWithTracking(taskID string, sourceID int, keywords []string) {
	// 创建可取消的context
	ctx, cancel := context.WithCancel(context.Background())
	registerTaskCanceler(taskID, cancel)
	defer unregisterTaskCanceler(taskID)

	// 更新状态为运行中
	updateCollectTask(taskID, map[string]interface{}{
		"status":  "running",
		"message": "采集任务执行中",
	})

	// 执行采集
	err := runCollectTask(ctx, taskID, sourceID, keywords)

	// 更新完成状态
	if err != nil {
		// 如果是context取消，任务已在cancelTask中更新状态
		if ctx.Err() == context.Canceled {
			log.Printf("🚫 任务 %s 已取消", taskID)
		} else {
			updateCollectTask(taskID, map[string]interface{}{
				"status":       "failed",
				"message":      fmt.Sprintf("采集失败: %v", err),
				"completed_at": time.Now().Format("2006-01-02 15:04:05"),
			})
			log.Printf("❌ 任务 %s 失败: %v", taskID, err)
		}
	} else {
		updateCollectTask(taskID, map[string]interface{}{
			"status":       "completed",
			"progress":     100,
			"message":      "采集完成",
			"completed_at": time.Now().Format("2006-01-02 15:04:05"),
		})
		log.Printf("✅ 任务 %s 完成", taskID)
	}
}

func runCollectTask(ctx context.Context, taskID string, sourceID int, keywords []string) error {
	if sourceID > 0 {
		// 采集指定的源
		if err := collectBySourceWithProgress(ctx, taskID, sourceID, keywords); err != nil {
			log.Printf("❌ 采集源 %d 采集失败: %v", sourceID, err)
			return err
		}
		return nil
	}

	// sourceID=0时，采集所有活跃的源
	log.Printf("🚀 开始批量采集所有活跃源...")
	rows, err := db.Query("SELECT id, name, code FROM sources WHERE is_active = 1 ORDER BY id")
	if err != nil {
		return fmt.Errorf("查询采集源失败: %v", err)
	}
	defer rows.Close()

	activeSources := []struct {
		ID   int
		Name string
		Code string
	}{}

	for rows.Next() {
		var s struct {
			ID   int
			Name string
			Code string
		}
		if err := rows.Scan(&s.ID, &s.Name, &s.Code); err == nil {
			activeSources = append(activeSources, s)
		}
	}

	log.Printf("📋 找到 %d 个活跃采集源", len(activeSources))

	// 遍历所有活跃源进行采集
	successCount := 0
	failCount := 0

	for _, source := range activeSources {
		// 检查是否被取消
		if ctx.Err() != nil {
			return ctx.Err()
		}

		log.Printf("\n========== 采集源: %s (%s) ==========", source.Name, source.Code)

		// 检查是否有对应的轨迹
		listTrace := getTraceBySourceAndType(source.ID, "list")
		if listTrace == nil {
			log.Printf("⚠️ 跳过 %s：未找到列表轨迹", source.Name)
			continue
		}

		// 使用collectBySource（不带进度跟踪，因为是批量模式）
		if err := collectBySource(source.ID, keywords); err != nil {
			log.Printf("❌ 采集源 %s 采集失败: %v", source.Name, err)
			failCount++
		} else {
			log.Printf("✅ 采集源 %s 完成", source.Name)
			successCount++
		}
	}

	log.Printf("\n📊 批量采集完成：成功 %d 个，失败 %d 个", successCount, failCount)
	return nil
}

// collectBySourceWithProgress 带进度跟踪的采集函数
func collectBySourceWithProgress(ctx context.Context, taskID string, sourceID int, keywords []string) error {
	var source Source
	err := db.QueryRow("SELECT id, name, code, category, base_url FROM sources WHERE id = ?", sourceID).Scan(
		&source.ID, &source.Name, &source.Code, &source.Category, &source.BaseURL,
	)
	if err != nil {
		return fmt.Errorf("获取采集源失败: %v", err)
	}

	log.Printf("🚀 开始采集任务：采集源=%s, 关键词=%v", source.Name, keywords)
	updateCollectTask(taskID, map[string]interface{}{
		"progress": 10,
		"message":  fmt.Sprintf("正在准备采集 %s", source.Name),
	})

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

	updateCollectTask(taskID, map[string]interface{}{
		"progress": 20,
		"message":  "浏览器已启动，开始采集列表",
	})

	solver := NewCaptchaSolver(captchaService)

	// 创建关键词匹配器（性能优化：在循环外创建一次，循环内重用）
	keywordMatcher := NewKeywordMatcher(keywords, MatchModeAny)

	totalFound := 0
	totalSaved := 0

	for kwIdx, keyword := range keywords {
		// 检查是否被取消
		if ctx.Err() != nil {
			return ctx.Err()
		}

		log.Printf("\n--- 关键词 [%d/%d]: %s ---", kwIdx+1, len(keywords), keyword)

		// 更新进度：20 + (kwIdx / len(keywords)) * 70
		progress := 20 + (kwIdx*70)/len(keywords)
		updateCollectTask(taskID, map[string]interface{}{
			"progress": progress,
			"message":  fmt.Sprintf("正在采集关键词: %s", keyword),
		})

		params := map[string]string{"Keyword": keyword}
		data, err := executeTrace(browser, listTrace, params, solver)
		if err != nil {
			log.Printf("❌ 列表采集失败: %v", err)
			updateCollectTask(taskID, map[string]interface{}{
				"message": fmt.Sprintf("关键词 %s 采集失败: %v", keyword, err),
			})
			continue
		}

		listItems := data.([]map[string]string)
		log.Printf("📋 列表采集完成，共 %d 条", len(listItems))
		totalFound += len(listItems)

		for i, item := range listItems {
			// 检查是否被取消
			if ctx.Err() != nil {
				return ctx.Err()
			}

			title := item["title"]
			if !keywordMatcher.Match(title) {
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

			result, err := saveTender(tender)
			if err != nil {
				log.Printf("❌ 保存失败: %v", err)
			} else {
				switch result.Action {
				case "created":
					log.Printf("✅ 新增到数据库")
					totalSaved++
				case "updated":
					log.Printf("🔄 更新已有记录")
					totalSaved++
				case "skipped":
					log.Printf("⏭️  已存在且无变化，跳过")
				}
				updateCollectTask(taskID, map[string]interface{}{
					"found": totalFound,
					"saved": totalSaved,
				})
			}
		}
	}

	updateCollectTask(taskID, map[string]interface{}{
		"progress": 90,
		"message":  fmt.Sprintf("采集完成，共发现 %d 条，保存 %d 条", totalFound, totalSaved),
		"found":    totalFound,
		"saved":    totalSaved,
	})

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

	// 创建关键词匹配器（性能优化）
	keywordMatcher := NewKeywordMatcher(keywords, MatchModeAny)

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
			if !keywordMatcher.Match(title) {
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

			result, err := saveTender(tender)
			if err != nil {
				log.Printf("❌ 保存失败: %v", err)
			} else {
				switch result.Action {
				case "created":
					log.Printf("✅ 新增到数据库")
				case "updated":
					log.Printf("🔄 更新已有记录")
				case "skipped":
					log.Printf("⏭️  已存在且无变化，跳过")
				}
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

	// 创建关键词匹配器（性能优化）
	keywordMatcher := NewKeywordMatcher(keywords, MatchModeAny)

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
			if !keywordMatcher.Match(title) {
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

			result, err := saveTender(tender)
			if err != nil {
				log.Printf("❌ 保存失败: %v", err)
			} else {
				switch result.Action {
				case "created":
					log.Printf("✅ 新增到数据库")
				case "updated":
					log.Printf("🔄 更新已有记录")
				case "skipped":
					log.Printf("⏭️  已存在且无变化，跳过")
				}
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
		log.Printf("❌ 查询轨迹失败: source_id=%d, type=%s, error=%v", sourceID, traceType, err)

		// 调试：列出所有轨迹
		rows, debugErr := db.Query("SELECT id, source_id, name, type, status FROM traces")
		if debugErr == nil {
			log.Printf("📋 数据库中的所有轨迹：")
			for rows.Next() {
				var id, sid int
				var name, ttype, status string
				rows.Scan(&id, &sid, &name, &ttype, &status)
				log.Printf("   - ID=%d, source_id=%d, name=%s, type=%s, status=%s", id, sid, name, ttype, status)
			}
			rows.Close()
		}
		return nil
	}

	log.Printf("✅ 找到轨迹: source_id=%d, type=%s, raw_content长度=%d", sourceID, traceType, len(rawContent))

	trace, err := parseTraceFile(rawContent)
	if err != nil {
		log.Printf("❌ 解析轨迹失败: %v", err)
		return nil
	}

	log.Printf("✅ 轨迹解析成功: %d 个步骤", len(trace.Steps))
	return trace
}

// KeywordMatchMode 关键词匹配模式
type KeywordMatchMode string

const (
	MatchModeAny   KeywordMatchMode = "any"   // OR逻辑：匹配任意一个关键词即可
	MatchModeAll   KeywordMatchMode = "all"   // AND逻辑：必须匹配所有关键词
	MatchModeExact KeywordMatchMode = "exact" // 精确匹配：文本完全等于关键词
)

// KeywordMatcher 关键词匹配器
type KeywordMatcher struct {
	keywords      []string         // 原始关键词列表
	lowercaseKeys []string         // 预处理的小写关键词（性能优化）
	mode          KeywordMatchMode // 匹配模式
}

// NewKeywordMatcher 创建关键词匹配器
func NewKeywordMatcher(keywords []string, mode KeywordMatchMode) *KeywordMatcher {
	if mode == "" {
		mode = MatchModeAny // 默认OR逻辑
	}

	// 预处理：转小写并去重
	lowercaseKeys := make([]string, 0, len(keywords))
	seen := make(map[string]bool)

	for _, kw := range keywords {
		lower := strings.ToLower(strings.TrimSpace(kw))
		if lower != "" && !seen[lower] {
			lowercaseKeys = append(lowercaseKeys, lower)
			seen[lower] = true
		}
	}

	// 按长度降序排序（长的在前，避免"软件开发"被"软件"先匹配）
	for i := 0; i < len(lowercaseKeys); i++ {
		for j := i + 1; j < len(lowercaseKeys); j++ {
			if len(lowercaseKeys[i]) < len(lowercaseKeys[j]) {
				lowercaseKeys[i], lowercaseKeys[j] = lowercaseKeys[j], lowercaseKeys[i]
			}
		}
	}

	return &KeywordMatcher{
		keywords:      keywords,
		lowercaseKeys: lowercaseKeys,
		mode:          mode,
	}
}

// Match 判断文本是否匹配关键词
func (km *KeywordMatcher) Match(text string) bool {
	if len(km.lowercaseKeys) == 0 {
		return true // 没有关键词限制，全部匹配
	}

	text = strings.ToLower(text)

	switch km.mode {
	case MatchModeAll:
		// AND逻辑：必须匹配所有关键词
		for _, kw := range km.lowercaseKeys {
			if !strings.Contains(text, kw) {
				return false
			}
		}
		return true

	case MatchModeExact:
		// 精确匹配：文本完全等于任意一个关键词
		for _, kw := range km.lowercaseKeys {
			if text == kw {
				return true
			}
		}
		return false

	default: // MatchModeAny
		// OR逻辑：匹配任意一个关键词即可
		for _, kw := range km.lowercaseKeys {
			if strings.Contains(text, kw) {
				return true
			}
		}
		return false
	}
}

// MatchedKeywords 返回匹配到的关键词列表
func (km *KeywordMatcher) MatchedKeywords(text string) []string {
	matched := []string{}
	text = strings.ToLower(text)

	for i, kw := range km.lowercaseKeys {
		if strings.Contains(text, kw) {
			matched = append(matched, km.keywords[i])
		}
	}

	return matched
}

// containsKeyword 保留旧函数以兼容（内部使用KeywordMatcher）
func containsKeyword(text string, keywords []string) bool {
	matcher := NewKeywordMatcher(keywords, MatchModeAny)
	return matcher.Match(text)
}

// ==================== HTTP API ====================

func startAPIServer() {
	http.Handle("/", http.FileServer(http.FS(staticFiles)))

	http.HandleFunc("/api/tenders", handleGetTenders)
	http.HandleFunc("/api/tenders/export/csv", handleExportCSV)
	http.HandleFunc("/api/collect", handleCollect)
	http.HandleFunc("/api/collect/tasks", handleCollectTasks)
	http.HandleFunc("/api/collect/task", handleCollectTask)
	http.HandleFunc("/api/collect/task/cancel", handleCancelTask)
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
		Category:  r.URL.Query().Get("category"),
		Status:    r.URL.Query().Get("status"),
		Keyword:   r.URL.Query().Get("keyword"),
		MatchMode: r.URL.Query().Get("match_mode"),
		DateFrom:  r.URL.Query().Get("date_from"),
		DateTo:    r.URL.Query().Get("date_to"),
		Tags:      r.URL.Query().Get("tags"),
		Limit:     20, // 默认每页20条
		Page:      1,  // 默认第1页
	}

	// 解析source_id
	if sourceIDStr := r.URL.Query().Get("source_id"); sourceIDStr != "" {
		if sourceID, err := parseInt(sourceIDStr); err == nil {
			params.SourceID = sourceID
		}
	}

	// 解析page
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if page, err := parseInt(pageStr); err == nil && page > 0 {
			params.Page = page
		}
	}

	// 解析limit（每页记录数）
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := parseInt(limitStr); err == nil && limit > 0 {
			params.Limit = limit
		}
	}

	result, err := queryTenders(params)
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
	var responseData []TenderResponse
	for _, t := range result.Data {
		tr := TenderResponse{Tender: t}
		if src, ok := sources[t.SourceID]; ok {
			tr.SourceName = src.Name
			tr.SourceType = src.Category
		}
		responseData = append(responseData, tr)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"data":        responseData,
		"total":       result.Total,
		"page":        result.Page,
		"page_size":   result.PageSize,
		"total_pages": result.TotalPages,
	})
}

// handleExportCSV 处理CSV导出请求
func handleExportCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	params := TenderQueryParams{
		Category:  r.URL.Query().Get("category"),
		Status:    r.URL.Query().Get("status"),
		Keyword:   r.URL.Query().Get("keyword"),
		MatchMode: r.URL.Query().Get("match_mode"),
		DateFrom:  r.URL.Query().Get("date_from"),
		DateTo:    r.URL.Query().Get("date_to"),
		Limit:     10000, // 导出最多10000条
	}

	// 解析source_id
	if sourceIDStr := r.URL.Query().Get("source_id"); sourceIDStr != "" {
		if sourceID, err := parseInt(sourceIDStr); err == nil {
			params.SourceID = sourceID
		}
	}

	// 执行CSV导出
	if err := exportTendersToCSV(w, params); err != nil {
		log.Printf("导出CSV失败: %v", err)
		http.Error(w, fmt.Sprintf("导出失败: %v", err), http.StatusInternalServerError)
	}
}

// handleCancelTask 处理任务取消请求
func handleCancelTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	taskID := r.URL.Query().Get("id")
	if taskID == "" {
		http.Error(w, "Missing task id", http.StatusBadRequest)
		return
	}

	// 查询任务状态
	task, err := getCollectTask(taskID)
	if err != nil {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	// 只能取消运行中的任务
	if task.Status != "running" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("任务状态为 %s，无法取消", task.Status),
		})
		return
	}

	// 执行取消
	if err := cancelTask(taskID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "任务已取消",
	})
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

	// 创建任务记录
	task, err := createCollectTask(req.SourceID, req.Keywords)
	if err != nil {
		http.Error(w, fmt.Sprintf("创建任务失败: %v", err), http.StatusInternalServerError)
		return
	}

	// 异步执行采集任务
	go runCollectTaskWithTracking(task.ID, req.SourceID, req.Keywords)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "采集任务已启动",
		"task_id": task.ID,
		"task":    task,
	})
}

func handleCollectTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := parseInt(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	tasks, err := getAllCollectTasks(limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    tasks,
		"count":   len(tasks),
	})
}

func handleCollectTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	taskID := r.URL.Query().Get("id")
	if taskID == "" {
		http.Error(w, "Missing task id", http.StatusBadRequest)
		return
	}

	task, err := getCollectTask(taskID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Task not found: %v", err), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    task,
	})
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
