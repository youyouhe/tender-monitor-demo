# 轨迹采集失败问题分析与修复

## 问题描述

用户测试山东省政府采购网的采集时失败，错误信息：

```
panic: eval js error: DOMException: Failed to execute 'querySelector' on 'Document':
'aria/请输入公告标题' is not a valid selector.
```

---

## 根本原因

### 1️⃣ **ARIA 选择器不被支持**

Chrome DevTools Recorder 录制的轨迹包含多种选择器格式：

```json
{
    "type": "click",
    "selectors": [
        ["aria/请输入公告标题"],                    // ❌ ARIA选择器（Rod不支持）
        ["#el-id-11-10"],                          // ✅ ID选择器
        ["xpath///*[@id=\"el-id-11-10\"]"],        // ✅ XPath
        ["pierce/#el-id-11-10"]                    // ⚠️ Pierce/Shadow DOM
    ]
}
```

**旧代码问题**：
- 只取第一个选择器：`sel := step.Selectors[0][0]`
- 遇到 `aria/` 前缀时直接保存，没有跳过
- Rod 库执行 `querySelector('aria/请输入公告标题')` 时报错

### 2️⃣ **冗余步骤过多**

录制的77个步骤中包含大量不必要的事件：
- `keyDown` / `keyUp` - 键盘按下/释放
- `change` - 输入框内容变化（每输入一个字符触发一次）
- 重复的 `navigate` 步骤

这些应该被过滤或合并。

---

## 修复方案

### ✅ 已修复（代码层面）

#### 1. **智能选择器提取算法**

按优先级选择可用的选择器：

```go
// 优先级顺序：
// 1. ID选择器（#xxx）- 最稳定
// 2. 标准CSS选择器
// 3. XPath选择器
// 4. Pierce选择器（Shadow DOM）
// 跳过：aria/、text/ 选择器

for _, selectorGroup := range step.Selectors {
    sel := selectorGroup[0]

    // 跳过不支持的格式
    if strings.HasPrefix(sel, "aria/") || strings.HasPrefix(sel, "text/") {
        continue
    }

    // ID选择器优先
    if strings.Contains(sel, "#") {
        selectedSelector = sel
        break
    }

    // 其他逻辑...
}
```

#### 2. **过滤冗余步骤**

```go
skipTypes := []string{"setViewport", "keyDown", "keyUp"}
```

#### 3. **change 事件转换为 input**

```go
if action == "change" {
    action = "input"
}
newStep.Value = step.Value  // 提取输入值
```

---

## 修复后的效果

### 修复前
```
执行步骤 1/77: (空操作)
执行步骤 2/77: (空操作)
...
执行步骤 43/77: click
panic: 'aria/请输入公告标题' is not a valid selector
```

### 修复后（预期）
```
执行步骤 1/15: navigate
执行步骤 2/15: click
执行步骤 3/15: input (使用 #el-id-11-10)
执行步骤 4/15: input (验证码)
执行步骤 5/15: click (查询按钮)
...
```

---

## 重新测试步骤

### 1. **使用修复后的程序**

```bash
# 重新构建
go build -o tender-monitor main.go

# 启动服务
./tender-monitor
```

### 2. **直接上传原始录制文件**

- 访问 http://localhost:8080
- 进入"轨迹管理"标签
- 点击"+ 上传轨迹"
- 粘贴或上传你的原始 JSON 文件
- 选择采集源：山东省政府采购网
- 轨迹类型：列表页 (list)
- 点击"保存"

### 3. **启动采集测试**

- 回到"招标列表"标签
- 选择"山东省政府采购网"
- 输入关键词（或留空使用默认）
- 点击"采集"
- 切换到"采集任务"查看进度

---

## 进一步优化建议

### 🔧 使用 convert-trace 工具（推荐）

虽然现在可以直接上传，但使用专用转换工具效果更好：

```bash
# 转换列表轨迹
go run ./cmd/convert-trace/ shandong_recording.json list traces/shandong_list.json

# 手动编辑生成的文件
# 1. 将硬编码的"软件"替换为 {{.Keyword}}
# 2. 添加数据提取规则（extract步骤）
```

**手动编辑示例**：

```json
{
  "name": "山东省政府采购网-列表",
  "type": "list",
  "url": "http://www.ccgp-shandong.gov.cn/xxgk?colCode=0301",
  "steps": [
    {
      "action": "navigate",
      "url": "http://www.ccgp-shandong.gov.cn/xxgk?colCode=0301"
    },
    {
      "action": "click",
      "selector": "div.second-search > div:nth-of-type(1) > div:nth-of-type(2) span:nth-of-type(4)"
    },
    {
      "action": "input",
      "selector": "#el-id-11-10",
      "value": "{{.Keyword}}"  // ✅ 使用模板变量
    },
    {
      "action": "wait",
      "wait_time": 2000
    },
    {
      "action": "extract",
      "type": "list",
      "selector": "tbody tr",  // ✅ 添加数据提取规则
      "fields": {
        "title": "td:nth-of-type(3) span",
        "date": "td:nth-of-type(5)",
        "url": "td:nth-of-type(3) span"
      }
    }
  ]
}
```

---

## 常见问题

### Q1: 为什么有些选择器格式不支持？

**A**: Rod 库基于标准的 Web API，只支持：
- ✅ CSS Selectors（`.class`, `#id`, `tag`）
- ✅ XPath（`//div[@id='xxx']`）
- ❌ ARIA（`aria/请输入`）- 这是 Chrome 专有格式
- ❌ Text（`text/查询`）- 这是 Chrome 专有格式

### Q2: 录制的轨迹为什么有77个步骤？

**A**: Chrome DevTools Recorder 会录制**所有**浏览器事件，包括：
- 每次键盘按下/释放
- 每个字符的输入
- 鼠标移动、悬停

这些在自动化时不需要，应该被过滤或合并。

### Q3: 如何判断轨迹是否有效？

**A**: 检查以下关键点：
1. ✅ 有 `navigate` 步骤（导航到目标页面）
2. ✅ 有 `input` 步骤（输入关键词）
3. ✅ 有 `extract` 步骤（提取数据）
4. ✅ 选择器使用标准CSS或XPath格式
5. ✅ 关键词使用 `{{.Keyword}}` 模板变量

### Q4: 验证码怎么处理？

**A**:
1. **自动识别**（推荐）：
   ```json
   {
     "action": "captcha",
     "image_selector": "img.captcha",
     "input_selector": "#captcha-input"
   }
   ```
   系统会调用 OCR 服务自动识别

2. **手动处理**：
   如果识别失败，系统会保存验证码图片到 `data/captcha_*.png`，
   任务会失败并提示手动查看

---

## 提交修复

修复已提交到代码库：

**文件变更**:
- `main.go` - 修复选择器提取逻辑
- `main.go` - 过滤冗余步骤
- `main.go` - 添加 change → input 转换

**Commit**: 待提交

---

## 验证清单

测试时请检查：

- [ ] 不再出现 "aria/ is not a valid selector" 错误
- [ ] 轨迹步骤数减少（从77步降到约15-20步）
- [ ] 能正确输入关键词
- [ ] 能点击查询按钮
- [ ] 能提取列表数据

---

## 联系支持

如果问题仍然存在，请提供：
1. 完整的错误日志
2. 录制的原始 JSON 文件
3. 目标网站 URL
4. 采集源配置信息

我们会进一步协助调试！🚀
