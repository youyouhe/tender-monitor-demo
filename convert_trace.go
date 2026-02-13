package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Chrome Recorder 格式
type ChromeRecording struct {
	Title string       `json:"title"`
	Steps []ChromeStep `json:"steps"`
}

type ChromeStep struct {
	Type      string     `json:"type"`
	URL       string     `json:"url,omitempty"`
	Selectors [][]string `json:"selectors,omitempty"`
	Value     string     `json:"value,omitempty"`
	Target    string     `json:"target,omitempty"`
}

// 简化格式
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
}

func convertChromeRecording(input string, traceType string) (*SimplifiedTrace, error) {
	// 读取 Chrome Recording 文件
	data, err := os.ReadFile(input)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %v", err)
	}

	var recording ChromeRecording
	if err := json.Unmarshal(data, &recording); err != nil {
		return nil, fmt.Errorf("解析JSON失败: %v", err)
	}

	// 转换为简化格式
	trace := &SimplifiedTrace{
		Name:  recording.Title,
		Type:  traceType, // "list" 或 "detail"
		Steps: []TraceStep{},
	}

	for i, step := range recording.Steps {
		switch step.Type {
		case "setViewport":
			// 忽略视口设置
			continue

		case "navigate":
			trace.URL = step.URL
			trace.Steps = append(trace.Steps, TraceStep{
				Action: "navigate",
				URL:    step.URL,
			})

		case "click":
			selector := extractSelector(step.Selectors)
			trace.Steps = append(trace.Steps, TraceStep{
				Action:   "click",
				Selector: selector,
			})

		case "change":
			// 判断是否是验证码输入
			if strings.Contains(step.Value, "验证码") || len(step.Value) == 4 {
				// 这是验证码输入，需要找到前一步的图片元素
				if i > 0 {
					trace.Steps = append(trace.Steps, TraceStep{
						Action:        "captcha",
						ImageSelector: "img[src*='captcha']", // 需要手动调整
						InputSelector: extractSelector(step.Selectors),
					})
				}
			} else {
				// 普通输入
				selector := extractSelector(step.Selectors)

				// 参数化关键词输入
				value := step.Value
				if isKeywordInput(selector) {
					value = "{{.Keyword}}"
				}

				trace.Steps = append(trace.Steps, TraceStep{
					Action:   "input",
					Selector: selector,
					Value:    value,
				})
			}

		case "waitForElement":
			selector := extractSelector(step.Selectors)
			trace.Steps = append(trace.Steps, TraceStep{
				Action:         "wait",
				WaitForVisible: selector,
			})
		}
	}

	// 根据类型添加提取步骤
	if traceType == "list" {
		trace.Steps = append(trace.Steps, TraceStep{
			Action:   "extract",
			Type:     "list",
			Selector: "tbody tr", // 需要手动调整
			Fields: map[string]string{
				"title": "td:nth-child(3) span",
				"date":  "td:nth-child(4)",
				"url":   "td:nth-child(3) span",
			},
		})
	} else if traceType == "detail" {
		trace.Steps = append(trace.Steps, TraceStep{
			Action: "extract",
			Type:   "detail",
			Fields: map[string]string{
				"amount":  "span:contains('预算金额')",
				"contact": "span:contains('联系人')",
				"phone":   "span:contains('联系电话')",
			},
		})
	}

	return trace, nil
}

func extractSelector(selectors [][]string) string {
	if len(selectors) == 0 {
		return ""
	}

	// 优先使用文本选择器或 CSS 选择器
	for _, selectorGroup := range selectors {
		if len(selectorGroup) > 0 {
			return selectorGroup[0]
		}
	}

	return ""
}

func isKeywordInput(selector string) bool {
	keywords := []string{"标题", "关键词", "公告", "keyword", "title"}
	selector = strings.ToLower(selector)
	for _, kw := range keywords {
		if strings.Contains(selector, kw) {
			return true
		}
	}
	return false
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

	// 转换
	trace, err := convertChromeRecording(inputFile, traceType)
	if err != nil {
		fmt.Printf("❌ 转换失败: %v\n", err)
		os.Exit(1)
	}

	// 保存
	output, err := json.MarshalIndent(trace, "", "  ")
	if err != nil {
		fmt.Printf("❌ 生成JSON失败: %v\n", err)
		os.Exit(1)
	}

	// 确保输出目录存在
	os.MkdirAll(filepath.Dir(outputFile), 0755)

	if err := os.WriteFile(outputFile, output, 0644); err != nil {
		fmt.Printf("❌ 保存文件失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ 转换成功")
	fmt.Printf("   输入: %s\n", inputFile)
	fmt.Printf("   输出: %s\n", outputFile)
	fmt.Printf("   类型: %s\n", traceType)
	fmt.Println("\n⚠️  请手动检查并调整以下内容:")
	fmt.Println("   1. 验证码图片选择器 (image_selector)")
	fmt.Println("   2. 列表行选择器 (selector)")
	fmt.Println("   3. 字段提取选择器 (fields)")
	fmt.Println("   4. 等待时间和条件")
}
