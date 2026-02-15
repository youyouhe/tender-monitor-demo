#!/usr/bin/env python3
"""
统一验证码识别服务 (FastAPI)
支持 ddddocr (轻量) 和 Qwen2-VL (智能) 两种引擎，通过请求参数切换。
"""

from __future__ import annotations

import base64
import io
import logging
import os
from abc import ABC, abstractmethod
from contextlib import asynccontextmanager
from enum import Enum
from typing import Optional

from fastapi import FastAPI, File, Form, Query, Request, UploadFile
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
from pydantic import BaseModel

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
)
logger = logging.getLogger(__name__)

MAX_IMAGE_SIZE = 10 * 1024 * 1024  # 10MB
ALLOWED_CONTENT_TYPES = {"image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp"}


# ==================== Pydantic Models ====================


class EngineType(str, Enum):
    DDDDOCR = "ddddocr"
    QWEN = "qwen"


class OCRResult(BaseModel):
    success: bool
    text: str = ""
    confidence: float = 0.0
    engine: str = ""
    error: Optional[str] = None
    raw_response: Optional[str] = None


class Base64Request(BaseModel):
    image_base64: str
    engine: Optional[EngineType] = None
    prompt: Optional[str] = None


class BatchResultItem(BaseModel):
    filename: str
    text: str = ""
    confidence: float = 0.0
    success: bool = False


class BatchResponse(BaseModel):
    success: bool
    count: int = 0
    results: list[BatchResultItem] = []
    error: Optional[str] = None


# ==================== Engine Abstraction ====================


class OCREngine(ABC):
    """验证码识别引擎抽象基类"""

    @abstractmethod
    def recognize(self, image_data: bytes, prompt: Optional[str] = None) -> OCRResult:
        ...

    @abstractmethod
    def is_available(self) -> bool:
        ...

    @abstractmethod
    def engine_name(self) -> str:
        ...

    def status_info(self) -> dict:
        return {
            "engine": self.engine_name(),
            "available": self.is_available(),
        }


def _load_ddddocr_class():
    """加载 DdddOcr 类，兼容 1.4.x 和 1.6.x 版本

    ddddocr 1.6.0 存在包结构缺陷：core.py 和 core/ 目录同时存在，
    导致 __init__.py 中 ``from .core import DdddOcr`` 失败。
    此处通过 importlib 直接加载 core.py 绕过冲突。
    """
    # 先尝试正常导入 (1.4.x 无冲突)
    try:
        from ddddocr import DdddOcr
        return DdddOcr
    except ImportError:
        pass

    # 1.6.x: 直接加载 core.py 文件
    import importlib.util
    import site
    import sys

    pkg_dir = None
    for base in site.getsitepackages() + [site.getusersitepackages()]:
        candidate = os.path.join(base, "ddddocr")
        if os.path.isdir(candidate) and os.path.isfile(os.path.join(candidate, "core.py")):
            pkg_dir = candidate
            break

    if pkg_dir is None:
        raise ImportError("ddddocr package not found")

    # 预注册一个空的 ddddocr 模块占位，防止 core.py 内部的相对导入触发 __init__.py
    import types
    fake_pkg = types.ModuleType("ddddocr")
    fake_pkg.__path__ = [pkg_dir]
    fake_pkg.__file__ = os.path.join(pkg_dir, "__init__.py")
    sys.modules["ddddocr"] = fake_pkg

    # 加载 ddddocr 子模块
    def _load_submodule(name: str, filepath: str):
        spec = importlib.util.spec_from_file_location(name, filepath)
        mod = importlib.util.module_from_spec(spec)
        sys.modules[name] = mod
        spec.loader.exec_module(mod)
        return mod

    _load_submodule("ddddocr.charsets", os.path.join(pkg_dir, "charsets.py"))
    _load_submodule("ddddocr.utils", os.path.join(pkg_dir, "utils.py"))

    core_mod = _load_submodule("ddddocr.core_module", os.path.join(pkg_dir, "core.py"))
    return core_mod.DdddOcr


class DdddocrEngine(OCREngine):
    """基于 ddddocr 的轻量级 OCR 引擎"""

    def __init__(self) -> None:
        self._ocr = None
        try:
            cls = _load_ddddocr_class()
            self._ocr = cls(show_ad=False)
            logger.info("ddddocr 引擎初始化成功")
        except Exception as e:
            logger.warning("ddddocr 引擎初始化失败: %s", e)

    def recognize(self, image_data: bytes, prompt: Optional[str] = None) -> OCRResult:
        if self._ocr is None:
            return OCRResult(success=False, error="ddddocr 引擎未初始化", engine=self.engine_name())
        try:
            text = self._ocr.classification(image_data)
            logger.info("ddddocr 识别成功: %s", text)
            return OCRResult(
                success=True,
                text=text,
                confidence=1.0,
                engine=self.engine_name(),
            )
        except Exception as e:
            logger.error("ddddocr 识别失败: %s", e, exc_info=True)
            return OCRResult(success=False, error=str(e), engine=self.engine_name())

    def is_available(self) -> bool:
        return self._ocr is not None

    def engine_name(self) -> str:
        return "ddddocr"


class QwenEngine(OCREngine):
    """基于 Qwen3-VL 的智能验证码识别引擎 (按需加载)

    使用 Qwen3-VL-Thinking 模型，对验证码识别禁用 thinking 模式以降低延迟。
    apply_chat_template 直接返回 tokenized tensors，不再依赖 qwen_vl_utils。
    """

    DEFAULT_PROMPT = (
        "请识别图片中的验证码。\n"
        "规则：\n"
        "1. 如果是数字/字母组合，直接返回内容（如：a3b9）\n"
        "2. 如果是算术题，返回计算结果（如：3+5=? 返回 8）\n"
        "3. 如果是汉字，直接返回汉字\n"
        "4. 只返回验证码内容，不要任何解释\n\n"
        "验证码是："
    )

    # </think> 特殊 token id，用于剥离 thinking 输出
    _THINK_END_TOKEN_ID = 151668

    def __init__(self) -> None:
        self._model = None
        self._processor = None
        self._device: Optional[str] = None
        self._model_name = os.getenv("QWEN_MODEL", "Qwen/Qwen3-VL-2B-Thinking")
        self._use_gpu = os.getenv("USE_GPU", "true").lower() == "true"
        self._enable_thinking = os.getenv("QWEN_THINKING", "false").lower() == "true"
        self._max_pixels = int(os.getenv("MAX_PIXELS", "360000"))
        self._min_pixels = int(os.getenv("MIN_PIXELS", "64000"))

    def _ensure_loaded(self) -> bool:
        """按需加载模型，首次调用时初始化"""
        if self._model is not None:
            return True
        try:
            import torch
            from transformers import AutoProcessor, Qwen3VLForConditionalGeneration

            if self._use_gpu and torch.cuda.is_available():
                self._device = "cuda"
                logger.info("Qwen3-VL 使用 GPU: %s", torch.cuda.get_device_name(0))
            else:
                self._device = "cpu"
                logger.info("Qwen3-VL 使用 CPU")

            self._model = Qwen3VLForConditionalGeneration.from_pretrained(
                self._model_name,
                torch_dtype=torch.float16 if self._device == "cuda" else torch.float32,
                device_map="auto" if self._device == "cuda" else None,
            )
            self._processor = AutoProcessor.from_pretrained(
                self._model_name,
                min_pixels=self._min_pixels,
                max_pixels=self._max_pixels,
            )
            if self._device == "cpu":
                self._model = self._model.to(self._device)

            logger.info("Qwen3-VL 模型加载成功: %s", self._model_name)
            return True
        except Exception as e:
            logger.error("Qwen3-VL 模型加载失败: %s", e, exc_info=True)
            return False

    @staticmethod
    def _strip_thinking(output_ids: list[int], full_text: str) -> str:
        """从输出中剥离 <think>...</think> 块，仅保留最终回答"""
        try:
            # 从后向前查找 </think> token
            idx = len(output_ids) - output_ids[::-1].index(QwenEngine._THINK_END_TOKEN_ID)
            # idx 之后的内容即为最终回答（skip_special_tokens 已去除 token 本身）
            # 但 batch_decode 已将所有 token 解码，因此改用文本切分
        except ValueError:
            pass

        # 文本级别的 fallback：去除 <think>...</think> 块
        import re
        cleaned = re.sub(r"<think>.*?</think>", "", full_text, flags=re.DOTALL)
        return cleaned.strip()

    def recognize(self, image_data: bytes, prompt: Optional[str] = None) -> OCRResult:
        if not self._ensure_loaded():
            return OCRResult(
                success=False,
                error="Qwen3-VL 模型未加载，请确认已安装依赖和下载模型",
                engine=self.engine_name(),
            )
        try:
            import torch
            from PIL import Image

            image = Image.open(io.BytesIO(image_data))
            if image.mode == "RGBA":
                image = image.convert("RGB")

            messages = [
                {
                    "role": "user",
                    "content": [
                        {"type": "image", "image": image},
                        {"type": "text", "text": prompt or self.DEFAULT_PROMPT},
                    ],
                }
            ]

            # Qwen3-VL: apply_chat_template 直接返回 tokenized tensors
            inputs = self._processor.apply_chat_template(
                messages,
                tokenize=True,
                add_generation_prompt=True,
                enable_thinking=self._enable_thinking,
                return_dict=True,
                return_tensors="pt",
            )
            inputs = inputs.to(self._model.device)

            with torch.no_grad():
                generated_ids = self._model.generate(**inputs, max_new_tokens=256)

            generated_ids_trimmed = [
                out_ids[len(in_ids):]
                for in_ids, out_ids in zip(inputs.input_ids, generated_ids)
            ]
            output_text = self._processor.batch_decode(
                generated_ids_trimmed,
                skip_special_tokens=True,
                clean_up_tokenization_spaces=False,
            )[0]

            # 剥离 thinking 内容（即使 enable_thinking=False 也做保险处理）
            result_text = self._strip_thinking(
                generated_ids_trimmed[0].tolist()
                if hasattr(generated_ids_trimmed[0], "tolist")
                else list(generated_ids_trimmed[0]),
                output_text,
            )
            logger.info("Qwen3-VL 识别成功: %s", result_text)

            return OCRResult(
                success=True,
                text=result_text,
                confidence=0.9 if result_text else 0.0,
                engine=self.engine_name(),
                raw_response=output_text,
            )
        except Exception as e:
            logger.error("Qwen3-VL 识别失败: %s", e, exc_info=True)
            return OCRResult(success=False, error=str(e), engine=self.engine_name())

    def is_available(self) -> bool:
        try:
            import torch  # noqa: F401
            from transformers import Qwen3VLForConditionalGeneration  # noqa: F401
            return True
        except ImportError:
            return False

    def engine_name(self) -> str:
        return "qwen"

    def status_info(self) -> dict:
        info = super().status_info()
        info["model"] = self._model_name
        info["model_loaded"] = self._model is not None
        info["device"] = self._device
        try:
            import torch
            info["gpu_available"] = torch.cuda.is_available()
        except ImportError:
            info["gpu_available"] = False
        return info


# ==================== Engine Registry ====================


engines: dict[str, OCREngine] = {}


def get_engine(name: Optional[str] = None) -> OCREngine:
    """获取指定引擎，默认返回 ddddocr"""
    key = name or EngineType.DDDDOCR.value
    engine = engines.get(key)
    if engine is None:
        raise ValueError(f"未知引擎: {key}")
    return engine


# ==================== Image Extraction ====================


async def extract_image_data(request: Request, file: Optional[UploadFile] = None) -> bytes:
    """从请求中提取图片数据，支持三种方式"""
    # 方式 1: multipart file upload
    if file is not None:
        data = await file.read()
        if len(data) > MAX_IMAGE_SIZE:
            raise ValueError(f"图片大小超过限制 ({MAX_IMAGE_SIZE // 1024 // 1024}MB)")
        logger.info("接收到文件上传，大小：%d 字节", len(data))
        return data

    # 方式 2: raw binary (Content-Type: image/*)
    content_type = request.headers.get("content-type", "")
    if content_type.startswith("image/"):
        data = await request.body()
        if len(data) > MAX_IMAGE_SIZE:
            raise ValueError(f"图片大小超过限制 ({MAX_IMAGE_SIZE // 1024 // 1024}MB)")
        logger.info("接收到原始图片数据，大小：%d 字节", len(data))
        return data

    # 方式 3: JSON with base64 (handled separately in endpoint)
    raise ValueError("未找到图片数据")


# ==================== FastAPI App ====================


@asynccontextmanager
async def lifespan(app: FastAPI):
    """应用生命周期：启动时初始化引擎，关闭时清理"""
    logger.info("初始化 OCR 引擎...")
    engines["ddddocr"] = DdddocrEngine()
    engines["qwen"] = QwenEngine()

    available = [name for name, eng in engines.items() if eng.is_available()]
    logger.info("可用引擎: %s", available)

    yield

    engines.clear()
    logger.info("OCR 引擎已清理")


app = FastAPI(
    title="验证码识别服务",
    version="2.0.0",
    lifespan=lifespan,
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
)


# ==================== Endpoints ====================


@app.get("/health")
async def health_check():
    """健康检查接口"""
    engine_statuses = {name: eng.status_info() for name, eng in engines.items()}
    return {
        "status": "ok",
        "service": "captcha-ocr",
        "version": "2.0.0",
        "engines": engine_statuses,
    }


@app.post("/ocr")
async def recognize_captcha(
    request: Request,
    image: Optional[UploadFile] = File(None),
    engine: Optional[EngineType] = Query(None),
    prompt: Optional[str] = Form(None),
):
    """
    验证码识别接口

    支持三种请求方式：
    1. multipart/form-data 上传图片文件（字段名：image）
    2. application/json 传递 base64 编码的图片（字段名：image_base64）
    3. 直接传递图片二进制数据（Content-Type: image/png 或 image/jpeg）

    可选参数：
    - engine: 识别引擎 (ddddocr|qwen)，默认 ddddocr
    - prompt: 自定义提示词（仅 qwen 引擎有效）
    """
    try:
        image_data: Optional[bytes] = None
        engine_name = engine.value if engine else None
        custom_prompt = prompt

        # 尝试 JSON body
        content_type = request.headers.get("content-type", "")
        if "application/json" in content_type:
            body = await request.json()
            base64_str = body.get("image_base64", "")
            if not base64_str:
                return JSONResponse(
                    {"success": False, "error": "JSON 请求中缺少 image_base64 字段"},
                    status_code=400,
                )
            if "," in base64_str:
                base64_str = base64_str.split(",", 1)[1]
            image_data = base64.b64decode(base64_str)
            if len(image_data) > MAX_IMAGE_SIZE:
                return JSONResponse(
                    {"success": False, "error": f"图片大小超过限制 ({MAX_IMAGE_SIZE // 1024 // 1024}MB)"},
                    status_code=400,
                )
            engine_name = engine_name or body.get("engine")
            custom_prompt = custom_prompt or body.get("prompt")
            logger.info("接收到 base64 数据，解码后大小：%d 字节", len(image_data))

        # 尝试 file upload 或 raw binary
        if image_data is None:
            try:
                image_data = await extract_image_data(request, image)
            except ValueError as e:
                return JSONResponse(
                    {"success": False, "error": str(e)},
                    status_code=400,
                )

        # 执行识别
        ocr_engine = get_engine(engine_name)
        if not ocr_engine.is_available():
            return JSONResponse(
                {"success": False, "error": f"引擎 {ocr_engine.engine_name()} 不可用"},
                status_code=503,
            )

        result = ocr_engine.recognize(image_data, custom_prompt)

        if result.success:
            return result.model_dump(exclude_none=True)
        else:
            return JSONResponse(result.model_dump(exclude_none=True), status_code=500)

    except ValueError as e:
        return JSONResponse({"success": False, "error": str(e)}, status_code=400)
    except Exception as e:
        logger.error("处理请求失败: %s", e, exc_info=True)
        return JSONResponse({"success": False, "error": str(e)}, status_code=500)


@app.post("/batch-ocr")
async def batch_recognize(
    images: list[UploadFile] = File(...),
    engine: Optional[EngineType] = Query(None),
):
    """批量识别接口"""
    try:
        ocr_engine = get_engine(engine.value if engine else None)
        if not ocr_engine.is_available():
            return JSONResponse(
                {"success": False, "error": f"引擎 {ocr_engine.engine_name()} 不可用"},
                status_code=503,
            )

        results: list[dict] = []
        for file in images:
            data = await file.read()
            if len(data) > MAX_IMAGE_SIZE:
                results.append({
                    "filename": file.filename or "",
                    "text": "",
                    "confidence": 0.0,
                    "success": False,
                })
                continue
            result = ocr_engine.recognize(data)
            results.append({
                "filename": file.filename or "",
                "text": result.text,
                "confidence": result.confidence,
                "success": result.success,
            })

        logger.info("批量识别完成，共 %d 张图片", len(results))
        return {"success": True, "count": len(results), "results": results}

    except Exception as e:
        logger.error("批量识别失败: %s", e, exc_info=True)
        return JSONResponse({"success": False, "error": str(e)}, status_code=500)


if __name__ == "__main__":
    import uvicorn

    port = int(os.getenv("PORT", "5000"))
    uvicorn.run(app, host="0.0.0.0", port=port, log_level="info")
