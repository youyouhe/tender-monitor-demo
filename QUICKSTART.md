# 快速开始指南

## 🎯 5分钟快速部署

### 步骤 1：环境检查

确保已安装：
- **Go 1.21+**
- **Python 3.10+**
- **Chrome/Chromium 浏览器**

```bash
# 检查版本
go version
python3 --version
```

### 步骤 2：一键部署

```bash
# 进入项目目录
cd tender-monitor

# 赋予执行权限
chmod +x deploy.sh

# 运行部署脚本
./deploy.sh install
```

部署脚本会自动：
1. ✅ 检查依赖环境
2. ✅ 创建必要目录
3. ✅ 安装 Go 和 Python 依赖
4. ✅ 编译 Go 程序
5. ✅ 启动验证码服务
6. ✅ 启动主程序

### 步骤 3：访问系统

打开浏览器访问：**http://localhost:8080**

你会看到美观的 Web 界面，包含：
- 🔍 搜索和筛选功能
- 📋 招标项目列表
- 📊 统计信息
- 🚀 采集任务触发按钮

### 步骤 4：启动第一次采集

1. 在界面上选择 **省份**（例如：山东省）
2. 点击 **🚀 启动采集** 按钮
3. 等待采集完成（查看终端日志）
4. 点击 **🔍 查询** 查看结果

---

## 📝 录制新的轨迹文件

### 准备工作

1. 打开 Chrome 浏览器
2. 按 **F12** 打开开发者工具
3. 切换到 **Recorder** 标签页
4. 点击 **Create a new recording**

### 录制列表页轨迹

1. **开始录制**
2. 导航到省份的采购网首页
3. 点击"采购公告"或"招标公告"
4. 在搜索框输入"软件"（或其他关键词）
5. 输入验证码（如果有）
6. 点击"查询"按钮
7. 等待列表加载完成
8. **停止录制**
9. 导出 JSON 文件（命名：`province_list_recording.json`）

### 录制详情页轨迹

1. **开始录制**
2. 从列表页点击第一条记录
3. 等待详情页加载完成
4. **停止录制**
5. 导出 JSON 文件（命名：`province_detail_recording.json`）

### 转换轨迹文件

```bash
# 转换列表页轨迹
go run convert_trace.go province_list_recording.json list traces/province_list.json

# 转换详情页轨迹
go run convert_trace.go province_detail_recording.json detail traces/province_detail.json
```

### 调整轨迹文件

**重要：** 转换后需要手动检查和调整：

1. **验证码图片选择器**
   ```json
   "image_selector": "img[src*='captcha']"
   ```
   根据实际页面调整选择器

2. **列表提取选择器**
   ```json
   "selector": "tbody tr",
   "fields": {
     "title": "td:nth-child(3) span",
     "date": "td:nth-child(4)",
     "url": "td:nth-child(3) a"
   }
   ```
   根据实际表格结构调整

3. **详情提取选择器**
   ```json
   "fields": {
     "amount": "td:contains('预算金额') + td",
     "contact": "td:contains('联系人') + td",
     "phone": "td:contains('联系电话') + td"
   }
   ```
   根据实际页面结构调整

### 测试新轨迹

重启程序测试：

```bash
./deploy.sh restart
```

在 Web 界面选择新省份并启动采集。

---

## 🔧 常用命令

### 服务管理

```bash
# 启动服务
./deploy.sh start

# 停止服务
./deploy.sh stop

# 重启服务
./deploy.sh restart

# 查看状态
./deploy.sh status

# 查看日志
./deploy.sh logs
```

### 数据库操作

```bash
# 查看数据库
sqlite3 data/tenders.db

# 查询记录数
sqlite3 data/tenders.db "SELECT COUNT(*) FROM tenders;"

# 查看最新10条记录
sqlite3 data/tenders.db "SELECT province, title, publish_date FROM tenders ORDER BY created_at DESC LIMIT 10;"

# 按省份统计
sqlite3 data/tenders.db "SELECT province, COUNT(*) FROM tenders GROUP BY province;"

# 导出数据到CSV
sqlite3 -header -csv data/tenders.db "SELECT * FROM tenders;" > export.csv
```

### 日志查看

```bash
# 实时查看主程序日志
tail -f logs/tender-monitor.log

# 实时查看验证码服务日志
tail -f logs/captcha.log

# 查看最近100行日志
tail -n 100 logs/tender-monitor.log

# 搜索错误日志
grep "ERROR" logs/tender-monitor.log
```

---

## 🐛 常见问题

### Q1: 验证码服务启动失败

**现象：** `验证码服务不可用`

**解决方案：**
```bash
# 检查端口占用
lsof -i :5000

# 手动启动测试
cd captcha-service
python3 captcha_service.py

# 查看详细错误
tail -f ../logs/captcha.log
```

### Q2: 浏览器无法启动

**现象：** `browser not found`

**解决方案：**
```bash
# Ubuntu/Debian
sudo apt-get install chromium-browser

# macOS
brew install --cask google-chrome

# 或设置环境变量指向已安装的浏览器
export CHROME_BIN=/path/to/chrome
```

### Q3: 采集任务卡住

**现象：** 采集任务长时间无响应

**解决方案：**
1. 查看日志定位问题：`tail -f logs/tender-monitor.log`
2. 检查轨迹文件选择器是否正确
3. 验证目标网站是否正常访问
4. 尝试关闭无头模式调试：在 `main.go` 中设置 `browserHeadless = false`

### Q4: 验证码识别率低

**现象：** 频繁需要手动输入

**解决方案：**
1. 检查保存的验证码图片：`ls -lh data/captcha_*.png`
2. 如果图片质量差，调整截图参数
3. 考虑使用付费验证码识别服务
4. 手动输入降级功能已内置，可以继续使用

### Q5: 数据库被锁定

**现象：** `database is locked`

**解决方案：**
```bash
# 关闭所有访问数据库的进程
./deploy.sh stop

# 检查数据库完整性
sqlite3 data/tenders.db "PRAGMA integrity_check;"

# 重启服务
./deploy.sh start
```

---

## 📊 性能优化

### 1. 并发采集

修改 `main.go`，使用 goroutine 并发采集多个省份：

```go
provinces := []string{"shandong", "beijing", "shanghai"}
for _, province := range provinces {
    go runCollectTask(province, keywords)
}
```

### 2. 缓存验证码识别

在内存中缓存相同验证码的识别结果（需要计算图片哈希）。

### 3. 数据库索引

已创建索引，无需额外优化：
- `idx_province` - 省份索引
- `idx_publish_date` - 发布日期索引

### 4. 限流控制

在采集时添加延迟，避免触发反爬：

```go
time.Sleep(2 * time.Second) // 每个请求间隔2秒
```

---

## 🚀 下一步

### 短期（1-2天）

- ✅ 完成山东省采集测试
- ⬜ 录制2-3个重点省份轨迹
- ⬜ 优化验证码识别准确率

### 中期（1周）

- ⬜ 扩展到10个省份
- ⬜ 添加定时采集功能
- ⬜ 实现微信/邮件通知

### 长期（持续）

- ⬜ 覆盖所有34个省份
- ⬜ 添加数据分析和可视化
- ⬜ 支持更多招标平台

---

## 📚 更多文档

- [完整文档](README.md)
- [验证码服务文档](captcha-service/README.md)
- [项目设计文档](tender_monitor_project.md)

---

**祝你使用愉快！** 🎉

如有问题，请查看日志或提交 Issue。
