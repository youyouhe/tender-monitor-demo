# 验证码识别服务

基于 `ddddocr` 的验证码自动识别服务，提供 HTTP API 接口。

## 功能特点

- ✅ 支持常见数字、字母验证码识别
- ✅ 识别率 90%+
- ✅ 提供 HTTP REST API
- ✅ 支持文件上传和 Base64 两种方式
- ✅ Docker 一键部署
- ✅ 健康检查接口

## 快速启动

### 方式 1：Python 直接运行

```bash
# 安装依赖
pip install -r requirements.txt

# 启动服务
python captcha_service.py
```

### 方式 2：Docker 部署

```bash
# 构建镜像
docker build -t captcha-ocr .

# 运行容器
docker run -d -p 5000:5000 --name captcha-ocr captcha-ocr
```

### 方式 3：Docker Compose（推荐）

```bash
# 启动服务
docker-compose up -d

# 查看日志
docker-compose logs -f

# 停止服务
docker-compose down
```

## API 文档

### 1. 健康检查

```bash
GET http://localhost:5000/health
```

**响应示例：**
```json
{
  "status": "ok",
  "service": "captcha-ocr",
  "version": "1.0.0"
}
```

### 2. 识别验证码

**接口地址：** `POST http://localhost:5000/ocr`

**方式 A：文件上传（multipart/form-data）**

```bash
curl -X POST http://localhost:5000/ocr \
  -F "image=@captcha.png"
```

**方式 B：Base64 编码（application/json）**

```bash
curl -X POST http://localhost:5000/ocr \
  -H "Content-Type: application/json" \
  -d '{"image_base64": "iVBORw0KGgoAAAANS..."}'
```

**响应示例：**
```json
{
  "success": true,
  "text": "9847",
  "confidence": 1.0
}
```

### 3. 批量识别（预留）

```bash
POST http://localhost:5000/batch-ocr
```

上传多个文件，返回批量识别结果。

## 测试

```bash
# 测试健康检查
python test_captcha.py

# 测试识别（需要提供验证码图片）
python test_captcha.py captcha.png
```

## 集成到 Go 程序

```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
)

type CaptchaResponse struct {
    Success    bool    `json:"success"`
    Text       string  `json:"text"`
    Confidence float64 `json:"confidence"`
}

func solveCaptcha(imageBytes []byte) (string, error) {
    // 创建请求
    req, err := http.NewRequest(
        "POST",
        "http://localhost:5000/ocr",
        bytes.NewReader(imageBytes),
    )
    if err != nil {
        return "", err
    }
    req.Header.Set("Content-Type", "image/png")

    // 发送请求
    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()

    // 解析响应
    body, _ := io.ReadAll(resp.Body)
    var result CaptchaResponse
    if err := json.Unmarshal(body, &result); err != nil {
        return "", err
    }

    if !result.Success {
        return "", fmt.Errorf("识别失败")
    }

    return result.Text, nil
}
```

## 性能建议

- **单实例并发：** 建议 10-20 个并发请求
- **响应时间：** 通常 100-300ms
- **资源占用：** 内存约 500MB-1GB

如需更高并发，可部署多个实例并使用负载均衡。

## 故障排查

### 服务无法启动

```bash
# 检查端口占用
lsof -i :5000

# 查看详细日志
python captcha_service.py
```

### 识别率低

- 验证码图片质量太低
- 验证码类型不支持（如复杂背景、扭曲严重）
- 考虑使用付费 API（如阿里云、腾讯云）

### Docker 容器启动失败

```bash
# 查看容器日志
docker logs captcha-ocr-service

# 进入容器调试
docker exec -it captcha-ocr-service bash
```

## 限制

- `ddddocr` 适用于简单验证码，复杂验证码识别率可能较低
- 不支持滑块、点选等行为验证码
- 如需更高识别率，建议使用商业 API 服务

## 升级计划

- [ ] 支持滑块验证码
- [ ] 支持点选验证码
- [ ] 添加识别结果缓存
- [ ] 支持自定义模型训练
- [ ] 添加 API 访问限流
