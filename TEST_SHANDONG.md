# 山东省采集测试指南

## 📋 测试准备清单

### 1. 环境检查

```bash
# 检查依赖
go version        # 需要 1.21+
python3 --version # 需要 3.10+

# 检查浏览器
which google-chrome || which chromium
```

---

## 🚀 快速开始测试

### 步骤 1：部署系统

```bash
cd /workspace/group/tender-monitor

# 一键部署
./deploy.sh install

# 或者分步部署
./deploy.sh
# 选择 1 (完整部署)
```

---

### 步骤 2：录制山东省轨迹文件

#### 2.1 打开山东省政府采购网

**网址：** http://www.ccgp-shandong.gov.cn/

#### 2.2 录制列表页轨迹

**步骤：**

1. **打开 Chrome DevTools**
   - 按 `F12` 或 `右键 → 检查`

2. **切换到 Recorder**
   - 顶部菜单找到 "Recorder" 标签
   - 如果没有，点击 `>>` 更多工具，选择 "Recorder"

3. **开始录制**
   - 点击 "Start new recording"
   - 录制名称：`shandong_list`

4. **执行操作**（一步步来，不要太快）：
   ```
   ① 导航到首页
   ② 点击 "采购公告" 或 "招标公告"
   ③ 在搜索框输入："软件"
   ④ 输入验证码（随便输入，比如 "1234"）
   ⑤ 点击 "查询" 或 "搜索" 按钮
   ⑥ 等待列表加载完成（看到结果）
   ```

5. **停止录制**
   - 点击 "Stop recording"

6. **导出 JSON**
   - 点击 "Export" 按钮
   - 选择 "Export as a JSON file"
   - 保存为：`shandong_list_recording.json`

#### 2.3 录制详情页轨迹

**步骤：**

1. **开始新录制**
   - 录制名称：`shandong_detail`

2. **执行操作**：
   ```
   ① 在列表页
   ② 点击第一条记录的标题（进入详情页）
   ③ 等待详情页加载完成
   ```

3. **停止并导出**
   - 保存为：`shandong_detail_recording.json`

---

### 步骤 3：转换轨迹文件

把录制的 JSON 文件放到项目目录，然后执行：

```bash
cd /workspace/group/tender-monitor

# 转换列表页轨迹
go run convert_trace.go shandong_list_recording.json list traces/shandong_list.json

# 转换详情页轨迹
go run convert_trace.go shandong_detail_recording.json detail traces/shandong_detail.json
```

---

### 步骤 4：手动调整轨迹文件

转换后需要手动检查和调整：

#### 4.1 检查列表页轨迹

打开 `traces/shandong_list.json`，重点检查：

**① 验证码图片选择器**
```json
{
  "action": "captcha",
  "image_selector": "img[src*='captcha']",  // 👈 可能需要调整
  "input_selector": "input[placeholder*='验证码']"
}
```

**如何找到正确的选择器：**
- 在网页上右键验证码图片 → 检查
- 查看 HTML 代码，找到图片的 `id`、`class` 或 `src` 特征
- 常见选择器：
  - `#captchaImg`
  - `.captcha-image`
  - `img[src*='validateCode']`

**② 列表提取选择器**
```json
{
  "action": "extract",
  "type": "list",
  "selector": "tbody tr",  // 👈 检查是否正确
  "fields": {
    "title": "td:nth-child(3) span",  // 👈 检查列号
    "date": "td:nth-child(4)",
    "url": "td:nth-child(3) a"
  }
}
```

**如何验证：**
- 在列表页右键第一行数据 → 检查
- 数一下标题在第几列（从 1 开始）
- 检查标题是在 `span` 还是 `a` 标签里

#### 4.2 检查详情页轨迹

打开 `traces/shandong_detail.json`：

```json
{
  "action": "extract",
  "type": "detail",
  "fields": {
    "amount": "td:contains('预算金额') + td",  // 👈 检查字段名
    "contact": "td:contains('联系人') + td",
    "phone": "td:contains('联系电话') + td"
  }
}
```

**如何验证：**
- 在详情页右键 "预算金额" → 检查
- 看它的 HTML 结构
- 常见结构：
  ```html
  <tr>
    <td>预算金额：</td>
    <td>100万元</td>
  </tr>
  ```

---

### 步骤 5：启动服务

```bash
# 启动所有服务
./deploy.sh start

# 查看状态
./deploy.sh status

# 查看日志
./deploy.sh logs
```

**检查：**
- ✅ 验证码服务：http://localhost:5000/health
- ✅ 主程序：http://localhost:8080/api/health
- ✅ Web 界面：http://localhost:8080

---

### 步骤 6：执行采集测试

#### 方式 1：通过 Web 界面

1. 打开：http://localhost:8080
2. 选择省份：山东省
3. 点击 "🚀 启动采集"
4. 观察终端日志输出
5. 等待采集完成
6. 点击 "🔍 查询" 查看结果

#### 方式 2：通过 API

```bash
curl -X POST http://localhost:8080/api/collect \
  -H "Content-Type: application/json" \
  -d '{
    "province": "shandong",
    "keywords": ["软件", "软件开发", "信息化"]
  }'
```

#### 方式 3：直接运行 Go 程序

```bash
cd /workspace/group/tender-monitor
go run main.go
```

---

## 🐛 常见问题排查

### 问题 1：浏览器无法启动

**现象：**
```
Error: browser not found
```

**解决：**
```bash
# Ubuntu/Debian
sudo apt-get install chromium-browser

# macOS
brew install --cask google-chrome
```

---

### 问题 2：验证码识别失败

**现象：**
```
验证码服务不可用，使用手动输入
```

**检查：**
```bash
# 测试验证码服务
curl http://localhost:5000/health

# 查看日志
tail -f logs/captcha.log

# 重启服务
cd captcha-service
python3 captcha_service.py
```

---

### 问题 3：找不到元素

**现象：**
```
Error: element not found: tbody tr
```

**原因：** 选择器不正确

**解决：**
1. 打开浏览器手动操作一遍
2. 使用 DevTools 查看实际的 HTML 结构
3. 调整 `traces/shandong_list.json` 中的选择器
4. 重启程序测试

---

### 问题 4：验证码图片截取失败

**现象：**
```
Error: screenshot failed
```

**解决：**
```json
// 尝试不同的选择器
"image_selector": "img#captchaImg"
// 或
"image_selector": ".captcha-img"
// 或
"image_selector": "img[alt='验证码']"
```

---

### 问题 5：数据提取为空

**现象：** 采集完成，但数据库没有数据

**检查：**
```bash
# 查看数据库
sqlite3 data/tenders.db "SELECT * FROM tenders;"

# 检查日志
tail -f logs/tender-monitor.log
```

**常见原因：**
- 关键词不匹配
- 选择器错误
- 等待时间不够

---

## 📊 测试成功标准

### 最小成功标准

- ✅ 浏览器能正常启动
- ✅ 能访问山东省采购网
- ✅ 验证码能截图（自动识别或手动输入）
- ✅ 能提取到列表数据（至少1条）
- ✅ 能进入详情页
- ✅ 能提取详情信息
- ✅ 数据保存到数据库

### 完整成功标准

- ✅ 验证码自动识别率 > 80%
- ✅ 列表页采集成功率 > 90%
- ✅ 详情页采集成功率 > 90%
- ✅ 无重复数据
- ✅ 数据字段完整

---

## 📝 测试记录模板

测试完成后，记录以下信息：

```
测试日期：2026-02-13
测试人：Tom He

【环境信息】
- Go 版本：
- Python 版本：
- 浏览器：
- 操作系统：

【测试结果】
1. 浏览器启动：✅/❌
2. 验证码识别：✅/❌（识别率：___%）
3. 列表采集：✅/❌（成功：__ 条）
4. 详情采集：✅/❌（成功：__ 条）
5. 数据保存：✅/❌

【遇到的问题】
1.
2.
3.

【需要调整的地方】
1.
2.
3.
```

---

## 🎯 测试完成后

如果测试成功，你会有：

1. ✅ **2 个可用的轨迹文件**
   - `traces/shandong_list.json`
   - `traces/shandong_detail.json`

2. ✅ **数据库中有数据**
   ```bash
   sqlite3 data/tenders.db "SELECT COUNT(*) FROM tenders;"
   ```

3. ✅ **Web 界面可以查询和展示**
   - http://localhost:8080

---

## 📞 需要帮助？

遇到问题时：

1. **查看日志**
   ```bash
   ./deploy.sh logs
   ```

2. **截图发给我**
   - 错误信息
   - 网页结构
   - 轨迹文件内容

3. **告诉我具体情况**
   - 在哪一步卡住了
   - 错误提示是什么
   - 你尝试了什么

我会立即帮你解决！

---

**准备好了吗？开始测试吧！** 🚀

记得把录制的 JSON 文件和遇到的问题告诉我！
