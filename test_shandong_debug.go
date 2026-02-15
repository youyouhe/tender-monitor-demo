package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

const (
	targetURL       = "http://www.ccgp-shandong.gov.cn/home"
	viewportWidth   = 1157
	viewportHeight  = 865
	slowMotionDelay = 800 * time.Millisecond
	clickRetryDelay = 1 * time.Second
	pageLoadTimeout = 15 * time.Second
	resultWaitDelay = 3 * time.Second
	finalWaitDelay  = 10 * time.Second
	searchKeyword   = "软件"

	// CSS Selectors based on recording
	selectorProcurementTab   = "div.row-2 li.is_active"
	selectorProcurementXPath = "//*[@id=\"app\"]/div[1]/div/div[2]/div[2]/div[1]/div/div[2]/ul/li[2]"
	selectorMoreButton       = "div.row-2 div.h-right"
	selectorMoreXPath        = "//*[@id=\"app\"]/div[1]/div/div[2]/div[2]/div[1]/div/div[1]/div[2]"
	selectorTitleInput       = "input[placeholder=\"请输入公告标题\"]"
	selectorVerifyInput      = "input[placeholder=\"请输入验证码\"]"
	selectorSearchButton     = "button.el-button--primary"

	// Chinese text labels
	textProcurementTab = "采购公告"
	textSearchButton   = "查询"
)

func logStep(msg string) {
	fmt.Printf("[%s] %s\n", time.Now().Format("15:04:05"), msg)
}

func setupBrowser() (*rod.Browser, error) {
	logStep("正在启动浏览器...")

	l := launcher.New().
		Headless(false).
		Devtools(false).
		Set("disable-blink-features", "AutomationControlled").
		Set("window-size", "1920,1080")

	url, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("failed to launch browser: %w", err)
	}

	browser := rod.New().ControlURL(url).MustConnect()
	browser.SlowMotion(slowMotionDelay)

	return browser, nil
}

func navigateToHomePage(page *rod.Page) error {
	logStep("访问首页: 山东省政府采购网...")

	page.MustNavigate(targetURL)
	page.MustWaitLoad()

	// 设置大视口并最大化窗口
	page.MustSetViewport(1920, 1080, 1, false)
	page.MustWindowMaximize()

	return nil
}

func clickProcurementTab(page *rod.Page) error {
	logStep("步骤 1: 点击【采购公告】标签...")

	err := rod.Try(func() {
		page.MustElement(selectorProcurementTab).MustClick()
	})

	if err != nil {
		logStep("CSS选择器失败，尝试XPath...")
		page.MustElementX(selectorProcurementXPath).MustClick()
	}

	return nil
}

func clickMoreButton(page *rod.Page) error {
	logStep("步骤 2: 点击【更多】按钮...")
	time.Sleep(clickRetryDelay)

	// 记录点击前的URL
	infoBefore := page.MustInfo()
	logStep("点击前 URL: " + infoBefore.URL)

	err := rod.Try(func() {
		page.MustElement(selectorMoreButton).MustClick()
	})

	if err != nil {
		logStep("CSS选择器点击失败，尝试XPath...")
		page.MustElementX(selectorMoreXPath).MustClick()
	}

	// 等待可能的页面跳转或新标签页
	time.Sleep(3 * time.Second)

	// 检查点击后的URL
	infoAfter := page.MustInfo()
	logStep("点击后 URL: " + infoAfter.URL)

	return nil
}

func waitForSearchPage(page *rod.Page) (*rod.Element, error) {
	logStep("步骤 3: 等待查询列表页加载...")

	// 检查是否有新标签页打开
	time.Sleep(2 * time.Second)
	pages, err := page.Browser().Pages()
	if err == nil && len(pages) > 1 {
		logStep(fmt.Sprintf("检测到 %d 个标签页，切换到最新标签页", len(pages)))
		page = pages[len(pages)-1]
	}

	info := page.MustInfo()
	logStep("当前页面 URL: " + info.URL)

	page.Timeout(pageLoadTimeout).MustWaitStable()

	logStep("步骤 4: 检查查询页元素...")

	// 等待页面完全渲染
	time.Sleep(2 * time.Second)

	// Try to find title input using placeholder
	logStep("尝试查找标题输入框...")
	titleInput, err := page.Timeout(30 * time.Second).Element(selectorTitleInput)
	if err != nil {
		logStep("【错误】未能找到标题输入框: " + err.Error())
		page.MustScreenshot("error_at_list_page.png")
		return nil, fmt.Errorf("search page not loaded correctly: %w", err)
	}

	logStep("成功找到标题输入框元素")
	return titleInput, nil
}

func inputSearchKeyword(page *rod.Page, inputEl *rod.Element) error {
	logStep("正在输入关键词：" + searchKeyword)

	// Click to focus the input
	inputEl.MustClick()
	time.Sleep(300 * time.Millisecond)

	// Clear existing text and input new keyword
	inputEl.MustSelectAllText()
	time.Sleep(200 * time.Millisecond)
	inputEl.MustInput(searchKeyword)

	return nil
}

func handleCaptchaAndSubmit(page *rod.Page) error {
	logStep("步骤 5: 定位验证码输入框...")

	// Find verify code input using placeholder
	verifyInput, err := page.Timeout(10 * time.Second).Element(selectorVerifyInput)
	if err != nil {
		return fmt.Errorf("failed to find verify code input: %w", err)
	}

	fmt.Println("")
	fmt.Println("====================================================")
	fmt.Println("【验证码输入】")
	fmt.Println("请在浏览器中查看验证码图片，然后在这里输入验证码：")
	fmt.Println("====================================================")
	fmt.Println("")

	var captchaCode string
	fmt.Print("请输入验证码: ")
	fmt.Scanln(&captchaCode)

	logStep("正在输入验证码...")

	// 重新查找验证码输入框（避免元素过期）
	verifyInput, err = page.Timeout(10 * time.Second).Element(selectorVerifyInput)
	if err != nil {
		return fmt.Errorf("重新定位验证码输入框失败: %w", err)
	}

	// 使用非panic方式输入
	err = verifyInput.Input(captchaCode)
	if err != nil {
		// 备用方案：通过JavaScript设置值
		logStep("直接输入失败，尝试JS方式...")
		page.MustEval(`(val) => {
			const input = document.querySelector('input[placeholder="请输入验证码"]');
			if (input) {
				input.value = val;
				input.dispatchEvent(new Event('input', { bubbles: true }));
				return true;
			}
			return false;
		}`, captchaCode)
	}

	time.Sleep(500 * time.Millisecond)

	// Step 6: Click search button
	logStep("步骤 6: 点击查询按钮...")
	searchBtn, err := page.ElementR(selectorSearchButton, textSearchButton)
	if err != nil {
		// Try to find by aria label
		searchBtn, err = page.Element("[aria-label=\"查询\"]")
		if err != nil {
			// Fallback to XPath
			searchBtn, err = page.ElementX("//button[contains(@class, 'el-button--primary')]")
			if err != nil {
				return fmt.Errorf("failed to find search button: %w", err)
			}
		}
	}
	searchBtn.MustClick()

	return nil
}

func captureResult(page *rod.Page) (string, error) {
	logStep("步骤 7: 等待结果刷新并截图...")
	time.Sleep(resultWaitDelay)
	page.MustWaitStable()

	savePath := "search_result_" + time.Now().Format("150405") + ".png"
	page.MustScreenshot(savePath)

	return savePath, nil
}

// Notice 表示一条公告信息
type Notice struct {
	No          string
	Region      string
	Title       string
	Method      string
	Type        string
	PublishTime string
	DetailURL   string
	Content     string
}

// NoticeDetail 表示公告详情
type NoticeDetail struct {
	Title       string
	ProjectNo   string
	PublishTime string
	Content     string
	Attachments []string
}

func extractNotices(page *rod.Page) ([]Notice, error) {
	logStep("步骤 8: 提取公告列表数据...")

	// 等待表格加载
	time.Sleep(3 * time.Second)
	page.MustWaitStable()

	// 调试：打印页面HTML的一部分来查看表格结构
	html, _ := page.HTML()
	if len(html) > 0 {
		// 查找包含表格的部分
		if idx := strings.Index(html, "el-table"); idx != -1 && idx+500 < len(html) {
			logStep("页面HTML中包含 el-table 类")
		}
	}

	// 使用record中的XPath定位表格行
	// XPath: //*[@id="app"]/div[1]/div/div/div[2]/div/div[2]/div[2]/table/tbody/tr[n]
	logStep("使用record中的XPath提取数据...")

	var notices []Notice

	// 尝试读取最多10行数据
	for i := 1; i <= 10; i++ {
		// 构建XPath：第i行的所有单元格
		xpath := fmt.Sprintf("//*[@id='app']/div[1]/div/div/div[2]/div/div[2]/div[2]/table/tbody/tr[%d]/td", i)

		cells, err := page.ElementsX(xpath)
		if err != nil || len(cells) == 0 {
			// 没有更多行了
			break
		}

		if len(cells) < 6 {
			// 列数不够，跳过
			continue
		}

		notice := Notice{
			No:          getCellText(cells, 0),
			Region:      getCellText(cells, 1),
			Title:       getCellText(cells, 2),
			Method:      getCellText(cells, 3),
			Type:        getCellText(cells, 4),
			PublishTime: getCellText(cells, 5),
		}

		// 简单过滤：序号是数字，标题不为空
		if !isValidSequenceNo(notice.No) || len(notice.Title) < 5 {
			continue
		}

		notices = append(notices, notice)
		logStep(fmt.Sprintf("提取第 %d 条: %s", len(notices), notice.Title))
	}

	logStep(fmt.Sprintf("共提取 %d 条有效公告", len(notices)))
	return notices, nil
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

func isValidSequenceNo(s string) bool {
	if !isNumeric(s) {
		return false
	}
	// 序号应该在1-1000范围内
	no := 0
	for _, c := range s {
		no = no*10 + int(c-'0')
	}
	return no >= 1 && no <= 1000
}

func getCellText(cells rod.Elements, index int) string {
	if index >= len(cells) {
		return ""
	}
	text := cells[index].MustText()
	return strings.TrimSpace(text)
}

func saveToCSV(notices []Notice, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// 写入表头
	header := []string{"序号", "地区", "标题", "采购方式", "项目类型", "发布时间", "详情链接", "详细内容"}
	if err := writer.Write(header); err != nil {
		return err
	}

	// 写入数据
	for _, notice := range notices {
		record := []string{
			notice.No,
			notice.Region,
			notice.Title,
			notice.Method,
			notice.Type,
			notice.PublishTime,
			notice.DetailURL,
			notice.Content,
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}

func fetchNoticeDetails(page *rod.Page, notices []Notice) ([]Notice, error) {
	logStep("步骤 9: 获取公告详情...")
	logStep(fmt.Sprintf("共有 %d 条公告需要获取详情", len(notices)))

	for i := range notices {
		logStep(fmt.Sprintf("\n[%d/%d] 正在获取: %s", i+1, len(notices), notices[i].Title))

		err := rod.Try(func() {
			// 记录当前URL
			infoBefore := page.MustInfo()
			beforeURL := infoBefore.URL

			// 使用CSS选择器点击：第(i+1)行第3列的span元素
			// 注意：录制显示数据从第1行开始（tr:nth-of-type(1)）
			selector := fmt.Sprintf("table tbody tr:nth-of-type(%d) td:nth-of-type(3) span", i+1)

			logStep(fmt.Sprintf("  尝试点击选择器: %s", selector))

			// 先滚动到元素
			el, err := page.Timeout(10 * time.Second).Element(selector)
			if err != nil {
				// 尝试XPath方式
				xpath := fmt.Sprintf("//*[@id=\"app\"]/div[1]/div/div/div[2]/div/div[2]/div[2]/table/tbody/tr[%d]/td[3]/span", i+1)
				logStep(fmt.Sprintf("  CSS失败，尝试XPath: %s", xpath))
				el, err = page.Timeout(10 * time.Second).ElementX(xpath)
				if err != nil {
					logStep(fmt.Sprintf("  未找到元素: %v", err))
					return
				}
			}

			// 滚动到可见区域
			el.MustScrollIntoView()
			time.Sleep(500 * time.Millisecond)

			// 点击元素
			logStep("  正在点击...")
			el.MustClick()

			logStep("  已触发点击，等待页面跳转...")
			time.Sleep(5 * time.Second)

			// 检查是否打开新标签页
			logStep("  检查新标签页...")

			// 等待一段时间让新标签页打开
			time.Sleep(3 * time.Second)

			pages, err := page.Browser().Pages()
			if err != nil {
				logStep(fmt.Sprintf("  获取页面列表失败: %v", err))
				return
			}

			logStep(fmt.Sprintf("  当前共有 %d 个标签页", len(pages)))

			currentPage := page
			if len(pages) > 1 {
				// 找到包含 "detail" 的URL（详情页）
				for _, p := range pages {
					info, err := p.Info()
					if err != nil {
						continue
					}
					// 如果URL包含detail，那就是详情页
					if strings.Contains(info.URL, "detail") {
						currentPage = p
						logStep(fmt.Sprintf("  找到详情页标签: %s", info.URL))
						break
					}
				}

				// 激活新标签页
				if currentPage != page {
					currentPage.MustActivate()
					logStep("  已切换到详情页并激活")
				}
			}

			// 等待详情页加载并检查URL变化
			logStep("  等待页面加载...")

			// 使用非阻塞方式等待，带超时
			done := make(chan bool, 1)
			go func() {
				currentPage.MustWaitStable()
				done <- true
			}()

			select {
			case <-done:
				logStep("  页面已稳定")
			case <-time.After(20 * time.Second):
				logStep("  等待页面稳定超时，继续执行")
			}

			// 获取当前URL
			infoAfter := currentPage.MustInfo()
			currentURL := infoAfter.URL
			logStep(fmt.Sprintf("  当前URL: %s", currentURL))

			// 检查URL是否真的变化了
			if currentURL == beforeURL {
				logStep("  警告：URL未变化，可能点击未生效或页面未跳转")
				// 截图查看当前状态
				debugScreenshot := fmt.Sprintf("debug_no_navigate_%s.png", notices[i].No)
				currentPage.MustScreenshot(debugScreenshot)
				logStep(fmt.Sprintf("  已保存调试图: %s", debugScreenshot))
				return
			}

			notices[i].DetailURL = currentURL

			// 截图保存详情页
			detailScreenshot := fmt.Sprintf("detail_%s_%s.png", notices[i].No, time.Now().Format("150405"))
			currentPage.MustScreenshot(detailScreenshot)
			logStep(fmt.Sprintf("  详情页截图: %s", detailScreenshot))

			// 提取详情内容
			content, err := extractDetailContent(currentPage)
			if err == nil && len(content) > 0 {
				notices[i].Content = content
				logStep(fmt.Sprintf("  成功提取详情，内容长度: %d 字符", len(content)))
			}

			// 返回列表页
			if len(pages) > 1 && currentPage != page {
				// 关闭详情页标签
				currentPage.MustClose()
				logStep("  已关闭详情页")
			} else {
				// 返回上一页
				page.MustNavigate(beforeURL)
				logStep("  已返回列表")
			}

			page.MustWaitStable()
			time.Sleep(1 * time.Second)
		})

		if err != nil {
			logStep(fmt.Sprintf("  处理第 %d 条时出错: %v", i+1, err))
		}
	}

	return notices, nil
}

func extractDetailContent(page *rod.Page) (string, error) {
	// 使用录制中的选择器
	selectors := []string{
		"div.site-content > table > tbody > tr > td",
		"//*[@id=\"app\"]/div[1]/div/div/div[2]/table/tbody/tr/td",
		".content",
		".detail-content",
	}

	for _, selector := range selectors {
		var el *rod.Element
		var err error

		if strings.HasPrefix(selector, "//") {
			// XPath选择器
			el, err = page.ElementX(selector)
		} else {
			// CSS选择器
			el, err = page.Element(selector)
		}

		if err == nil {
			text, err := el.Text()
			if err == nil && len(text) > 50 {
				return strings.TrimSpace(text), nil
			}
		}
	}

	// 如果没找到特定内容区域，获取整个页面的文本
	body, err := page.Element("body")
	if err != nil {
		return "", err
	}

	text, err := body.Text()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(text), nil
}

func run() error {
	browser, err := setupBrowser()
	if err != nil {
		return err
	}
	defer browser.MustClose()

	page := browser.MustPage()

	if err := navigateToHomePage(page); err != nil {
		return err
	}

	if err := clickProcurementTab(page); err != nil {
		return err
	}

	if err := clickMoreButton(page); err != nil {
		return err
	}

	titleInput, err := waitForSearchPage(page)
	if err != nil {
		return err
	}

	if err := inputSearchKeyword(page, titleInput); err != nil {
		return err
	}

	if err := handleCaptchaAndSubmit(page); err != nil {
		return err
	}

	savePath, err := captureResult(page)
	if err != nil {
		return err
	}

	logStep("截图已保存至: " + savePath)

	// 提取公告列表
	notices, err := extractNotices(page)
	if err != nil {
		logStep("提取数据时出错: " + err.Error())
	} else {
		logStep(fmt.Sprintf("成功提取 %d 条公告", len(notices)))

		// 获取详情
		if len(notices) > 0 {
			notices, err = fetchNoticeDetails(page, notices)
			if err != nil {
				logStep("获取详情时出错: " + err.Error())
			}
		}

		// 保存到CSV
		csvPath := "notices_" + time.Now().Format("150405") + ".csv"
		if err := saveToCSV(notices, csvPath); err != nil {
			logStep("保存CSV失败: " + err.Error())
		} else {
			logStep("数据已保存至: " + csvPath)
		}
	}

	logStep("任务完成！")

	time.Sleep(finalWaitDelay)
	return nil
}

func main() {
	if err := run(); err != nil {
		logStep("【致命错误】" + err.Error())
		os.Exit(1)
	}
}
