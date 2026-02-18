package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type ChromeRecording struct {
	Title string       `json:"title"`
	URL   string       `json:"url"`
	Steps []ChromeStep `json:"steps"`
}

type ChromeStep struct {
	Type           string          `json:"type"`
	URL            string          `json:"url,omitempty"`
	Selectors      [][]string      `json:"selectors,omitempty"`
	Value          string          `json:"value,omitempty"`
	Target         string          `json:"target,omitempty"`
	Key            string          `json:"key,omitempty"`
	AssertedEvents []AssertedEvent `json:"assertedEvents,omitempty"`
}

type AssertedEvent struct {
	Type  string `json:"type"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

type SimplifiedTrace struct {
	Name  string      `json:"name"`
	Type  string      `json:"type"`
	URL   string      `json:"url"`
	Steps []TraceStep `json:"steps"`
}

type TraceStep struct {
	Action         string            `json:"action"`
	URL            string            `json:"url,omitempty"`
	Selector       string            `json:"selector,omitempty"`
	Value          string            `json:"value,omitempty"`
	ImageSelector  string            `json:"image_selector,omitempty"`
	InputSelector  string            `json:"input_selector,omitempty"`
	Type           string            `json:"type,omitempty"`
	Fields         map[string]string `json:"fields,omitempty"`
	WaitTime       int               `json:"wait_time,omitempty"`
	WaitForVisible string            `json:"wait_for_visible,omitempty"`
	Pagination     *PaginationConfig `json:"pagination,omitempty"`
}

type PaginationConfig struct {
	Selector   string `json:"selector"`
	NextButton string `json:"next_button"`
	MaxPages   int    `json:"max_pages"`
	MaxItems   int    `json:"max_items"`
}

type IntermediateStep struct {
	Type     string
	Selector string
	Value    string
	URL      string
	Target   string
}

type ListFieldInfo struct {
	TitleSelector string
	URLSelector   string
	DateSelector  string
	HasLink       bool
}

func convertChromeRecording(input string, traceType string) (*SimplifiedTrace, error) {
	data, err := os.ReadFile(input)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %v", err)
	}

	var recording ChromeRecording
	if err := json.Unmarshal(data, &recording); err != nil {
		return nil, fmt.Errorf("解析JSON失败: %v", err)
	}

	trace := &SimplifiedTrace{
		Name:  recording.Title,
		Type:  traceType,
		URL:   recording.URL,
		Steps: []TraceStep{},
	}

	var intermediate []IntermediateStep
	pendingChanges := make(map[string]string)
	var pageNavigated bool
	var listFieldInfo ListFieldInfo
	var listFieldCaptured bool
	var paginationSelector string

	flushAllPendingChanges := func() {
		for selector, value := range pendingChanges {
			if value != "" {
				intermediate = append(intermediate, IntermediateStep{
					Type:     "change",
					Selector: selector,
					Value:    value,
				})
			}
		}
		pendingChanges = make(map[string]string)
	}

	// 分析步骤，检测翻页和列表点击
	analyzeSteps := recording.Steps

	for i, step := range analyzeSteps {
		// 检测翻页控件
		if step.Type == "click" {
			selector := extractSelector(step.Selectors)
			if isPaginationClick(selector) {
				paginationSelector = selector
				continue
			}
		}

		// 检测列表行点击（可能导致页面跳转）
		if step.Type == "click" && i < len(analyzeSteps)-1 {
			nextStep := analyzeSteps[i+1]
			// 如果下一步是 navigate，说明当前点击导致了页面跳转
			if nextStep.Type == "navigate" {
				selector := extractSelector(step.Selectors)
				if isListItemClick(selector) {
					if !listFieldCaptured {
						listFieldInfo = parseListClickSelectors(step.Selectors)
						listFieldCaptured = true
					}
					// 记录这个是列表项点击，会导致跳转
					intermediate = append(intermediate, IntermediateStep{
						Type:     "listItemClick",
						Selector: selector,
						URL:      nextStep.URL,
					})
					continue
				}
			}
		}
	}

	// 重新处理所有步骤
	for _, step := range recording.Steps {
		switch step.Type {
		case "setViewport", "keyUp", "keyDown", "scroll":
			continue

		case "navigate":
			trace.URL = step.URL
			intermediate = append(intermediate, IntermediateStep{
				Type: "navigate",
				URL:  step.URL,
			})

		case "click":
			if pageNavigated {
				continue
			}
			selector := extractSelector(step.Selectors)
			if selector == "" {
				continue
			}

			// 跳过翻页按钮，不加入轨迹
			if isPaginationClick(selector) {
				continue
			}

			flushAllPendingChanges()

			if len(intermediate) > 0 {
				last := intermediate[len(intermediate)-1]
				if last.Type == "click" && last.Selector == selector {
					continue
				}
			}

			// 检测点击列表行的情况
			if traceType == "list" && isListNavigationClick(selector) {
				if !listFieldCaptured {
					listFieldInfo = parseListClickSelectors(step.Selectors)
					listFieldCaptured = true
				}
				pageNavigated = true
				continue
			}

			// 跳过列表项点击（已在上文处理）
			if isListItemClick(selector) {
				pageNavigated = true
				continue
			}

			if isInputFieldClick(selector, pendingChanges) {
				continue
			}

			intermediate = append(intermediate, IntermediateStep{
				Type:     "click",
				Selector: selector,
				Target:   step.Target,
			})

		case "change":
			selector := extractSelector(step.Selectors)
			if selector == "" {
				continue
			}
			pendingChanges[selector] = step.Value

		case "waitForElement":
			flushAllPendingChanges()
			selector := extractSelector(step.Selectors)
			if selector != "" {
				intermediate = append(intermediate, IntermediateStep{
					Type:     "waitForElement",
					Selector: selector,
				})
			}
		}
	}

	flushAllPendingChanges()

	trace.Steps = buildFinalSteps(intermediate, traceType, listFieldInfo, listFieldCaptured, paginationSelector)

	return trace, nil
}

func buildFinalSteps(intermediate []IntermediateStep, traceType string, listFieldInfo ListFieldInfo, hasListFieldInfo bool, paginationSelector string) []TraceStep {
	var result []TraceStep

	for i, step := range intermediate {
		switch step.Type {
		case "navigate", "listItemClick":
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
			waitTime := 500
			if isSearchButton(step.Selector) {
				waitTime = 3000
			}
			result = append(result, TraceStep{
				Action:   "wait",
				WaitTime: waitTime,
			})

		case "change":
			value := step.Value
			if isKeywordInput(step.Selector) {
				value = "{{.Keyword}}"
			}

			if isCaptchaInput(step.Selector, step.Value) {
				prevClick := findPrevClick(intermediate, i)
				result = append(result, TraceStep{
					Action:        "captcha",
					ImageSelector: prevClick,
					InputSelector: step.Selector,
				})
			} else {
				result = append(result, TraceStep{
					Action:   "input",
					Selector: step.Selector,
					Value:    value,
				})
			}

		case "waitForElement":
			result = append(result, TraceStep{
				Action:         "wait",
				WaitForVisible: step.Selector,
			})
		}
	}

	if traceType == "list" {
		fields := map[string]string{
			"title": "td:nth-child(1) span",
			"date":  "td:nth-child(3)",
			"url":   "td:nth-child(1) span",
		}
		if hasListFieldInfo {
			fields = map[string]string{
				"title": listFieldInfo.TitleSelector,
				"date":  listFieldInfo.DateSelector,
				"url":   listFieldInfo.URLSelector,
			}
		}

		// 检测列表容器选择器
		listSelector := detectListContainer(intermediate)

		extractStep := TraceStep{
			Action:   "extract",
			Type:     "list",
			Selector: listSelector,
			Fields:   fields,
		}

		// 添加翻页配置
		if paginationSelector != "" {
			extractStep.Pagination = &PaginationConfig{
				Selector:   paginationSelector,
				NextButton: paginationSelector,
				MaxPages:   10,
				MaxItems:   100,
			}
		}

		result = append(result, extractStep)
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

func findPrevClick(steps []IntermediateStep, currentIndex int) string {
	for i := currentIndex - 1; i >= 0; i-- {
		if steps[i].Type == "click" {
			return steps[i].Selector
		}
	}
	return "img[src*='captcha']"
}

func extractSelector(selectors [][]string) string {
	if len(selectors) == 0 {
		return ""
	}

	for _, selectorGroup := range selectors {
		if len(selectorGroup) > 0 {
			s := selectorGroup[0]
			if !strings.HasPrefix(s, "aria/") && !strings.HasPrefix(s, "text/") && !strings.HasPrefix(s, "pierce/") {
				return s
			}
		}
	}

	for _, selectorGroup := range selectors {
		if len(selectorGroup) > 0 {
			s := selectorGroup[0]
			if strings.HasPrefix(s, "pierce/") {
				return strings.TrimPrefix(s, "pierce/")
			}
		}
	}

	for _, selectorGroup := range selectors {
		if len(selectorGroup) > 0 {
			return selectorGroup[0]
		}
	}

	return ""
}

func isKeywordInput(selector string) bool {
	keywords := []string{"标题", "关键词", "keyword", "title"}
	selectorLower := strings.ToLower(selector)
	for _, kw := range keywords {
		if strings.Contains(selectorLower, kw) {
			return true
		}
	}
	return false
}

func isInputFieldClick(selector string, pendingChanges map[string]string) bool {
	if strings.Contains(selector, "input") || strings.Contains(selector, "[role=\"textbox\"]") {
		return true
	}
	return false
}

func detectListContainer(intermediate []IntermediateStep) string {
	for _, step := range intermediate {
		if step.Type == "click" && step.Selector != "" {
			selector := step.Selector
			if strings.Contains(selector, "li:nth-of-type") {
				return "li"
			}
			if strings.Contains(selector, "tr:nth-of-type") {
				return "tr"
			}
			if strings.Contains(selector, "div:nth-of-type") {
				return "div"
			}
		}
	}
	return "tbody tr"
}

func isListNavigationClick(selector string) bool {
	return strings.Contains(selector, "tr:nth-of-type") ||
		strings.Contains(selector, "tbody tr") ||
		strings.Contains(selector, "td.el-table")
}

func isListItemClick(selector string) bool {
	if selector == "" {
		return false
	}
	// 检测常见的列表项选择器模式
	listPatterns := []string{
		"li:nth-of-type", "li: nth-of-type",
		"tr:nth-of-type", "tbody tr",
		"div:nth-of-type", ".item", ".list-item",
		"a[href", "span",
	}
	for _, p := range listPatterns {
		if strings.Contains(selector, p) {
			return true
		}
	}
	return false
}

func isPaginationClick(selector string) bool {
	if selector == "" {
		return false
	}
	pagePatterns := []string{
		"page", "pager", "pagination",
		"next", "prev", "previous",
		"下页", "上页", "下一页", "上一页",
		"第", "页",
		"a:nth-of-type", "li:nth-of-type",
	}
	selectorLower := strings.ToLower(selector)
	for _, p := range pagePatterns {
		if strings.Contains(selectorLower, p) {
			return true
		}
	}
	// 检测数字结尾的选择器，如 "li:nth-of-type(2)"
	matched, _ := regexp.MatchString(`.*\((\d+|last)\)$`, selector)
	return matched
}

func isSearchButton(selector string) bool {
	return strings.Contains(selector, "button") &&
		(strings.Contains(selector, "primary") ||
			strings.Contains(selector, "search") ||
			strings.Contains(selector, "查询"))
}

func isCaptchaInput(selector string, value string) bool {
	return strings.Contains(selector, "验证码") || (len(value) == 4 && !strings.Contains(value, " "))
}

// parseListClickSelectors 从列表行点击的选择器中解析字段选择器
func parseListClickSelectors(selectors [][]string) ListFieldInfo {
	info := ListFieldInfo{
		TitleSelector: "td:nth-child(1) span",
		URLSelector:   "td:nth-child(1) a",
		DateSelector:  "td:nth-child(3)",
		HasLink:       false,
	}

	for _, selectorGroup := range selectors {
		for _, s := range selectorGroup {
			// 从 xpath 中解析结构
			if strings.HasPrefix(s, "xpath//") {
				info = parseXPathForFields(s)
				break
			}
			// 从 CSS 选择器中解析
			if strings.Contains(s, "td.el-table") || strings.Contains(s, "tr:nth-of-type") {
				info = parseCSSForFields(s)
			}
		}
	}

	return info
}

// parseXPathForFields 从 xpath 解析字段选择器
func parseXPathForFields(xpath string) ListFieldInfo {
	info := ListFieldInfo{
		TitleSelector: "td:nth-child(1) span",
		URLSelector:   "@click:td:nth-child(1) span",
		DateSelector:  "td:nth-child(3)",
		HasLink:       false,
	}

	// 示例: xpath///*[@id="app"]/.../tbody/tr[2]/td[1]/div/span
	// 解析 td[n] 获取列索引
	tdIdx := 1
	if idx := strings.Index(xpath, "/td["); idx != -1 {
		rest := xpath[idx+4:]
		if end := strings.Index(rest, "]"); end != -1 {
			fmt.Sscanf(rest[:end], "%d", &tdIdx)
		}
	}

	// 解析 td 后的子元素结构
	if idx := strings.Index(xpath, "/td["); idx != -1 {
		rest := xpath[idx:]
		if end := strings.Index(rest, "/a"); end != -1 {
			info.HasLink = true
		}
	}

	// 生成选择器
	info.TitleSelector = fmt.Sprintf("td:nth-child(%d) span", tdIdx)
	if info.HasLink {
		info.URLSelector = fmt.Sprintf("td:nth-child(%d) a", tdIdx)
	} else {
		// Vue SPA 没有 a 标签，通过点击元素获取跳转URL
		info.URLSelector = fmt.Sprintf("@click:td:nth-child(%d) span", tdIdx)
	}

	return info
}

// parseCSSForFields 从 CSS 选择器解析字段选择器
func parseCSSForFields(css string) ListFieldInfo {
	info := ListFieldInfo{
		TitleSelector: "td:nth-child(1) span",
		URLSelector:   "@click:td:nth-child(1) span",
		DateSelector:  "td:nth-child(3)",
		HasLink:       false,
	}

	colNum := 1
	// 示例: tr:nth-of-type(2) > td.el-table_1_column_1 span
	// 解析 td.el-table_n_column_m
	if strings.Contains(css, "td.el-table") {
		if idx := strings.Index(css, "el-table_"); idx != -1 {
			rest := css[idx+9:]
			if n, err := fmt.Sscanf(rest, "%d", &colNum); n == 1 && err == nil {
				info.TitleSelector = fmt.Sprintf("td:nth-child(%d) span", colNum)
				info.URLSelector = fmt.Sprintf("@click:td:nth-child(%d) span", colNum)
			}
		}
	}

	// 检查是否包含 a 标签
	if strings.Contains(css, " a") || strings.Contains(css, ">a") {
		info.HasLink = true
		info.URLSelector = fmt.Sprintf("td:nth-child(%d) a", colNum)
	}

	return info
}

func backupFile(filePath string) error {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil
	}

	timestamp := time.Now().Format("20060102_150405")
	ext := filepath.Ext(filePath)
	base := filePath[:len(filePath)-len(ext)]
	backupPath := fmt.Sprintf("%s_%s%s", base, timestamp, ext)

	if err := os.Rename(filePath, backupPath); err != nil {
		return err
	}

	fmt.Printf("📦 备份: %s\n", backupPath)
	return nil
}

func main() {
	if len(os.Args) < 4 {
		fmt.Println("用法: go run convert_trace.go <输入文件> <类型:list|detail> <输出文件>")
		fmt.Println("\n示例:")
		fmt.Println("  go run convert_trace.go recording.json list traces/shandong_list.json")
		os.Exit(1)
	}

	inputFile := os.Args[1]
	traceType := os.Args[2]
	outputFile := os.Args[3]

	if traceType != "list" && traceType != "detail" {
		fmt.Println("❌ 类型必须是 'list' 或 'detail'")
		os.Exit(1)
	}

	trace, err := convertChromeRecording(inputFile, traceType)
	if err != nil {
		fmt.Printf("❌ 转换失败: %v\n", err)
		os.Exit(1)
	}

	output, err := json.MarshalIndent(trace, "", "  ")
	if err != nil {
		fmt.Printf("❌ 生成JSON失败: %v\n", err)
		os.Exit(1)
	}

	os.MkdirAll(filepath.Dir(outputFile), 0755)

	if err := backupFile(outputFile); err != nil {
		fmt.Printf("❌ 备份失败: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outputFile, output, 0644); err != nil {
		fmt.Printf("❌ 保存文件失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ 转换成功")
	fmt.Printf("   输入: %s\n", inputFile)
	fmt.Printf("   输出: %s\n", outputFile)
	fmt.Printf("   类型: %s\n", traceType)
	fmt.Printf("   步骤数: %d\n", len(trace.Steps))
	fmt.Println("\n⚠️  请手动检查并调整以下内容:")
	fmt.Println("   1. 验证码图片选择器 (image_selector)")
	fmt.Println("   2. 列表行选择器 (selector)")
	fmt.Println("   3. 字段提取选择器 (fields)")
	fmt.Println("   4. 等待时间和条件")
}
