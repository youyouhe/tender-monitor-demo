package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

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

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err == nil {
		if success, ok := result["success"].(bool); ok && success {
			if text, ok := result["text"].(string); ok {
				return text, nil
			}
		}
	}

	if len(bodyStr) > 0 {
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

const (
	targetURL         = "http://www.ccgp-shandong.gov.cn/home"
	slowMotionDelay   = 800 * time.Millisecond
	pageLoadTimeout  = 15 * time.Second
	resultWaitDelay  = 3 * time.Second
	selectorTitleInput  = `input[placeholder="请输入公告标题"]`
	selectorVerifyInput = `input[placeholder="请输入验证码"]`
	selectorSearchButton = `button.el-button--primary`
)

type Notice struct {
	No         string
	Region     string
	Title      string
	Method     string
	Type       string
	PublishTime string
	DetailURL  string
}

func logStep(msg string) {
	fmt.Printf("[%s] %s\n", time.Now().Format("15:04:05"), msg)
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("用法: collect <搜索关键词>")
	}

	searchKeyword := os.Args[1]
	log.Printf("=== 山东省政府采购网信息采集 ===")
	log.Printf("搜索关键词: %s", searchKeyword)

	solver := NewCaptchaSolver("http://localhost:5000")
	if !solver.CheckAvailable() {
		log.Println("⚠️  验证码服务不可用，将使用手动输入")
	} else {
		log.Println("✅ 验证码服务已连接")
	}

	l := launcher.New().
		Headless(true).
		Devtools(false).
		Set("disable-blink-features", "AutomationControlled").
		Set("window-size", "1920,1080")

	url, err := l.Launch()
	if err != nil {
		log.Fatalf("启动浏览器失败: %v", err)
	}

	browser := rod.New().ControlURL(url).MustConnect()
	browser.SlowMotion(slowMotionDelay)
	defer browser.MustClose()

	page := browser.MustPage()
	defer page.Close()

	logStep("访问首页...")
	page.MustNavigate(targetURL)
	page.MustWaitLoad()
	time.Sleep(2 * time.Second)

	logStep("点击【采购公告】...")
	rod.Try(func() {
		page.MustElement("div.row-2 li.is_active").MustClick()
	})
	time.Sleep(2 * time.Second)

	logStep("点击【更多】按钮...")
	rod.Try(func() {
		page.MustElement("div.row-2 div.h-right").MustClick()
	})
	time.Sleep(3 * time.Second)

	pages, _ := browser.Pages()
	if len(pages) > 1 {
		page = pages[len(pages)-1]
	}
	time.Sleep(2 * time.Second)

	logStep("输入搜索关键词...")
	titleInput, err := page.Timeout(30 * time.Second).Element(selectorTitleInput)
	if err != nil {
		log.Fatalf("未找到标题输入框: %v", err)
	}

	titleInput.MustClick()
	time.Sleep(300 * time.Millisecond)
	titleInput.MustSelectAllText()
	time.Sleep(200 * time.Millisecond)
	titleInput.MustInput(searchKeyword)
	time.Sleep(500 * time.Millisecond)

	logStep("处理验证码...")
	verifyInput, err := page.Timeout(10 * time.Second).Element(selectorVerifyInput)
	if err == nil {
		captchaText := ""

		if solver.CheckAvailable() {
			logStep("尝试自动识别验证码...")
			captchaImgs, _ := page.Elements(`img[src*='captcha'], img[src*='code'], img[src*='valid']`)
			if len(captchaImgs) > 0 {
				imgBytes := captchaImgs[0].MustScreenshot()
				if text, err := solver.Solve(imgBytes); err == nil && len(text) > 0 {
					captchaText = text
					logStep(fmt.Sprintf("✅ 验证码自动识别成功: %s", text))
				} else {
					logStep(fmt.Sprintf("⚠️  自动识别失败: %v", err))
				}
			}
		}

		if captchaText == "" {
			fmt.Println("\n====================================================")
			fmt.Println("【验证码输入】")
			fmt.Println("自动识别失败，请手动查看浏览器中的验证码图片")
			fmt.Print("请输入验证码: ")
			fmt.Scanln(&captchaText)
		}

		logStep("输入验证码...")
		err = verifyInput.Input(captchaText)
		if err != nil {
			page.MustEval(`(val) => {
				const input = document.querySelector('input[placeholder="请输入验证码"]');
				if (input) {
					input.value = val;
					input.dispatchEvent(new Event('input', { bubbles: true }));
					return true;
				}
				return false;
			}`, captchaText)
		}
		time.Sleep(500 * time.Millisecond)
	} else {
		logStep("未检测到验证码，继续...")
	}

	logStep("点击查询按钮...")
	searchBtn, err := page.ElementR(selectorSearchButton, "查询")
	if err != nil {
		searchBtn, err = page.Element(`[aria-label="查询"]`)
		if err != nil {
			log.Fatalf("未找到查询按钮: %v", err)
		}
	}
	searchBtn.MustClick()

	logStep("等待结果加载...")
	time.Sleep(resultWaitDelay)
	page.MustWaitStable()

	logStep("提取公告列表...")
	time.Sleep(3 * time.Second)

	var notices []Notice

	for i := 1; i <= 30; i++ {
		xpath := fmt.Sprintf("//*[@id='app']/div[1]/div/div/div[2]/div/div[2]/div[2]/table/tbody/tr[%d]/td", i)
		cells, err := page.ElementsX(xpath)
		if err != nil || len(cells) == 0 {
			break
		}

		if len(cells) < 6 {
			continue
		}

		notice := Notice{
			No:         getCellText(cells, 0),
			Region:     getCellText(cells, 1),
			Title:      getCellText(cells, 2),
			Method:     getCellText(cells, 3),
			Type:       getCellText(cells, 4),
			PublishTime: getCellText(cells, 5),
		}

		if !isValidSequenceNo(notice.No) || len(notice.Title) < 5 {
			continue
		}

		if len(cells) > 2 {
			linkElem, err := cells[2].Element("a")
			if err == nil {
				href, _ := linkElem.Attribute("href")
				if href != nil {
					notice.DetailURL = *href
				}
			}
		}

		notices = append(notices, notice)
		logStep(fmt.Sprintf("提取第 %d 条: %s", len(notices), notice.Title))
	}

	logStep(fmt.Sprintf("共提取 %d 条有效公告", len(notices)))

	if len(notices) > 0 {
		csvPath := fmt.Sprintf("notices_%s_%s.csv", searchKeyword, time.Now().Format("20060102_150405"))
		if err := saveToCSV(notices, csvPath); err != nil {
			logStep("保存CSV失败: " + err.Error())
		} else {
			logStep("数据已保存至: " + csvPath)

			for _, n := range notices {
				fmt.Printf("\n【%s】%s\n", n.No, n.Title)
				fmt.Printf("  地区: %s | 方式: %s | 类型: %s\n", n.Region, n.Method, n.Type)
				fmt.Printf("  时间: %s\n", n.PublishTime)
			}
		}
	}

	screenshotPath := fmt.Sprintf("search_result_%s_%s.png", searchKeyword, time.Now().Format("20060102_150405"))
	page.MustScreenshot(screenshotPath)
	logStep("截图已保存: " + screenshotPath)

	logStep("任务完成！")
	time.Sleep(2 * time.Second)
}

func getCellText(cells []*rod.Element, index int) string {
	if index >= len(cells) {
		return ""
	}
	text, _ := cells[index].Text()
	return strings.TrimSpace(text)
}

func isValidSequenceNo(s string) bool {
	if !isNumeric(s) {
		return false
	}
	no := 0
	for _, c := range s {
		no = no*10 + int(c-'0')
	}
	return no >= 1 && no <= 1000
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func saveToCSV(notices []Notice, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"序号", "地区", "标题", "采购方式", "项目类型", "发布时间", "详情链接"}
	if err := writer.Write(header); err != nil {
		return err
	}

	for _, notice := range notices {
		record := []string{
			notice.No,
			notice.Region,
			notice.Title,
			notice.Method,
			notice.Type,
			notice.PublishTime,
			notice.DetailURL,
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}
