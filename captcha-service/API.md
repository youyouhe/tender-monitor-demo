# 验证码识别服务 API 调用指南

## 基本信息

| 项目       | 值                              |
| ---------- | ------------------------------- |
| 基础地址   | `http://<host>:5000`            |
| 默认引擎   | ddddocr（轻量 OCR）            |
| 可选引擎   | qwen（Qwen2-VL 多模态大模型）  |
| 图片限制   | 最大 10 MB                      |
| 支持格式   | PNG / JPEG / GIF / WebP / BMP   |

---

## 启动服务

```bash
cd captcha-service
source venv/bin/activate

# 默认（仅 ddddocr）
uvicorn app:app --host 0.0.0.0 --port 5000

# 启用 Qwen2-VL GPU 推理（需安装 requirements_qwen.txt 依赖）
QWEN_MODEL=Qwen/Qwen2-VL-2B-Instruct USE_GPU=true \
  uvicorn app:app --host 0.0.0.0 --port 5000
```

### 环境变量

| 变量           | 默认值                          | 说明                       |
| -------------- | ------------------------------- | -------------------------- |
| `PORT`         | `5000`                          | 监听端口                   |
| `QWEN_MODEL`   | `Qwen/Qwen2-VL-2B-Instruct`   | Qwen 模型名或本地路径       |
| `USE_GPU`      | `true`                          | Qwen 是否使用 GPU          |
| `MAX_PIXELS`   | `360000`                        | Qwen 输入图像最大像素数     |
| `MIN_PIXELS`   | `64000`                         | Qwen 输入图像最小像素数     |

---

## 接口列表

### 1. 健康检查

```
GET /health
```

**响应示例：**

```json
{
  "status": "ok",
  "service": "captcha-ocr",
  "version": "2.0.0",
  "engines": {
    "ddddocr": { "engine": "ddddocr", "available": true },
    "qwen": {
      "engine": "qwen",
      "available": true,
      "model": "Qwen/Qwen2-VL-2B-Instruct",
      "model_loaded": false,
      "device": null,
      "gpu_available": true
    }
  }
}
```

---

### 2. 单张识别

```
POST /ocr
```

支持三种请求方式，以及通过查询参数或请求体切换引擎。

#### 2.1 原始二进制（Go 服务调用方式）

```bash
curl -X POST http://localhost:5000/ocr \
  -H "Content-Type: image/png" \
  --data-binary @captcha.png
```

#### 2.2 表单文件上传

```bash
curl -X POST http://localhost:5000/ocr \
  -F "image=@captcha.png"
```

#### 2.3 JSON + Base64

```bash
curl -X POST http://localhost:5000/ocr \
  -H "Content-Type: application/json" \
  -d '{
    "image_base64": "<base64 编码的图片>",
    "engine": "qwen",
    "prompt": "请识别验证码"
  }'
```

> Base64 支持 Data URI 前缀（如 `data:image/png;base64,iVBOR...`），服务会自动去除。

#### 引擎切换

- **查询参数**：`POST /ocr?engine=qwen`
- **JSON 请求体**：`{"engine": "qwen", "image_base64": "..."}`
- 不指定时默认使用 `ddddocr`

#### 自定义提示词（仅 Qwen 引擎）

- **表单方式**：`-F "prompt=请计算这道数学题"`
- **JSON 方式**：`{"prompt": "请计算这道数学题", ...}`

#### 成功响应

```json
{
  "success": true,
  "text": "a3b9",
  "confidence": 1.0,
  "engine": "ddddocr"
}
```

#### 错误响应

```json
{
  "success": false,
  "error": "引擎 qwen 不可用"
}
```

**HTTP 状态码：**

| 状态码 | 含义                                |
| ------ | ----------------------------------- |
| 200    | 识别成功                            |
| 400    | 请求参数错误（缺少图片、格式错误等）|
| 500    | 识别失败或服务内部错误              |
| 503    | 指定引擎不可用                      |

---

### 3. 批量识别

```
POST /batch-ocr
```

通过 multipart/form-data 上传多张图片，一次请求识别全部。

```bash
curl -X POST "http://localhost:5000/batch-ocr?engine=ddddocr" \
  -F "images=@captcha1.png" \
  -F "images=@captcha2.png" \
  -F "images=@captcha3.png"
```

#### 成功响应

```json
{
  "success": true,
  "count": 3,
  "results": [
    { "filename": "captcha1.png", "text": "a3b9", "confidence": 1.0, "success": true },
    { "filename": "captcha2.png", "text": "x7m2", "confidence": 1.0, "success": true },
    { "filename": "captcha3.png", "text": "k5p1", "confidence": 1.0, "success": true }
  ]
}
```

---

## Go 服务集成示例

`main.go` 中的调用方式（原始二进制）：

```go
resp, err := http.Post(
    "http://localhost:5000/ocr",
    "image/png",
    bytes.NewReader(captchaImageBytes),
)
// 解析 JSON 响应
var result struct {
    Success bool   `json:"success"`
    Text    string `json:"text"`
}
json.NewDecoder(resp.Body).Decode(&result)
```

## Python 调用示例

```python
import requests

# 文件上传
with open("captcha.png", "rb") as f:
    resp = requests.post(
        "http://localhost:5000/ocr",
        files={"image": f},
        params={"engine": "ddddocr"},
    )
print(resp.json())

# Base64
import base64
with open("captcha.png", "rb") as f:
    b64 = base64.b64encode(f.read()).decode()
resp = requests.post(
    "http://localhost:5000/ocr",
    json={"image_base64": b64, "engine": "qwen"},
)
print(resp.json())
```

---

## 自动文档

FastAPI 内置 OpenAPI 文档，启动服务后访问：

- Swagger UI：`http://localhost:5000/docs`
- ReDoc：`http://localhost:5000/redoc`
