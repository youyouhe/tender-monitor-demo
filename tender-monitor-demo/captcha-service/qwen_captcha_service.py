#!/usr/bin/env python3
"""
基于 Qwen2-VL 的智能验证码识别服务
支持复杂验证码、算术题、常识问答等多种类型
"""

from flask import Flask, request, jsonify
from flask_cors import CORS
import logging
import base64
import io
import os
from PIL import Image
import torch
from transformers import Qwen2VLForConditionalGeneration, AutoProcessor
from qwen_vl_utils import process_vision_info

# 配置日志
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

app = Flask(__name__)
CORS(app)

# 全局变量
model = None
processor = None
device = None

# 配置参数
MODEL_NAME = os.getenv("QWEN_MODEL", "Qwen/Qwen2-VL-2B-Instruct")  # 可选: 2B, 7B, 72B
USE_GPU = os.getenv("USE_GPU", "true").lower() == "true"
MAX_PIXELS = int(os.getenv("MAX_PIXELS", "360000"))  # 图片最大像素
MIN_PIXELS = int(os.getenv("MIN_PIXELS", "64000"))   # 图片最小像素


def initialize_model():
    """初始化 Qwen2-VL 模型"""
    global model, processor, device
    
    logger.info(f"正在初始化 Qwen2-VL 模型: {MODEL_NAME}")
    
    # 检测设备
    if USE_GPU and torch.cuda.is_available():
        device = "cuda"
        logger.info(f"使用 GPU: {torch.cuda.get_device_name(0)}")
    else:
        device = "cpu"
        logger.info("使用 CPU (如有GPU建议启用以提升速度)")
    
    try:
        # 加载模型和处理器
        model = Qwen2VLForConditionalGeneration.from_pretrained(
            MODEL_NAME,
            torch_dtype=torch.float16 if device == "cuda" else torch.float32,
            device_map="auto" if device == "cuda" else None,
        )
        
        processor = AutoProcessor.from_pretrained(
            MODEL_NAME,
            min_pixels=MIN_PIXELS,
            max_pixels=MAX_PIXELS
        )
        
        if device == "cpu":
            model = model.to(device)
        
        logger.info("✅ Qwen2-VL 模型初始化成功")
        return True
        
    except Exception as e:
        logger.error(f"❌ 模型初始化失败: {str(e)}")
        logger.error("请先下载模型: huggingface-cli download Qwen/Qwen2-VL-2B-Instruct")
        return False


def recognize_captcha_with_qwen(image_data: bytes, prompt: str = None) -> dict:
    """
    使用 Qwen2-VL 识别验证码
    
    Args:
        image_data: 图片二进制数据
        prompt: 自定义提示词（可选）
    
    Returns:
        dict: {"success": bool, "text": str, "confidence": float, "raw_response": str}
    """
    if model is None or processor is None:
        return {
            "success": False,
            "error": "模型未初始化",
            "text": "",
            "confidence": 0.0
        }
    
    try:
        # 加载图片
        image = Image.open(io.BytesIO(image_data))
        
        # 如果是RGBA转RGB
        if image.mode == 'RGBA':
            image = image.convert('RGB')
        
        # 构建提示词（支持多种验证码类型）
        if prompt is None:
            prompt = """请识别图片中的验证码。
            
规则：
1. 如果是数字/字母组合，直接返回内容（如：a3b9）
2. 如果是算术题，返回计算结果（如：3+5=? 返回 8）
3. 如果是汉字，直接返回汉字（如：验证码）
4. 如果是问答题，返回答案（如：1+1=? 返回 2）
5. 只返回验证码内容，不要任何解释

验证码是："""
        
        # 构建消息
        messages = [
            {
                "role": "user",
                "content": [
                    {
                        "type": "image",
                        "image": image,
                    },
                    {"type": "text", "text": prompt},
                ],
            }
        ]
        
        # 准备推理
        text = processor.apply_chat_template(
            messages, tokenize=False, add_generation_prompt=True
        )
        image_inputs, video_inputs = process_vision_info(messages)
        
        inputs = processor(
            text=[text],
            images=image_inputs,
            videos=video_inputs,
            padding=True,
            return_tensors="pt",
        )
        inputs = inputs.to(device)
        
        # 生成结果
        logger.info("正在进行推理...")
        with torch.no_grad():
            generated_ids = model.generate(
                **inputs,
                max_new_tokens=128,
                do_sample=False,  # 确定性生成
                temperature=0.1,
            )
        
        generated_ids_trimmed = [
            out_ids[len(in_ids):] for in_ids, out_ids in zip(inputs.input_ids, generated_ids)
        ]
        
        output_text = processor.batch_decode(
            generated_ids_trimmed,
            skip_special_tokens=True,
            clean_up_tokenization_spaces=False
        )[0]
        
        # 清理输出（去除多余空格和换行）
        result_text = output_text.strip()
        
        logger.info(f"识别成功: {result_text}")
        
        # 计算置信度（简化版）
        confidence = 0.9 if len(result_text) > 0 else 0.0
        
        return {
            "success": True,
            "text": result_text,
            "confidence": confidence,
            "raw_response": output_text
        }
        
    except Exception as e:
        logger.error(f"识别失败: {str(e)}", exc_info=True)
        return {
            "success": False,
            "error": str(e),
            "text": "",
            "confidence": 0.0
        }


@app.route('/health', methods=['GET'])
def health_check():
    """健康检查接口"""
    model_status = "ready" if model is not None else "not_initialized"
    
    return jsonify({
        'status': 'ok',
        'service': 'qwen2-vl-captcha',
        'version': '2.0.0',
        'model': MODEL_NAME,
        'device': device if device else 'unknown',
        'model_status': model_status,
        'gpu_available': torch.cuda.is_available()
    })


@app.route('/ocr', methods=['POST'])
def recognize_captcha():
    """
    验证码识别接口
    
    支持三种请求方式：
    1. multipart/form-data 上传图片文件（字段名：image）
    2. application/json 传递 base64 编码的图片（字段名：image_base64）
    3. 直接传递图片二进制数据（Content-Type: image/png 或 image/jpeg）
    
    可选参数：
    - prompt: 自定义提示词（用于特殊验证码类型）
    
    返回格式：
    {
        "success": true,
        "text": "识别结果",
        "confidence": 0.95,
        "raw_response": "模型原始输出"
    }
    """
    if model is None:
        return jsonify({
            'success': False,
            'error': '模型未初始化，请先启动模型'
        }), 503
    
    try:
        image_data = None
        custom_prompt = None
        
        # 方式1：接收文件上传
        if 'image' in request.files:
            file = request.files['image']
            image_data = file.read()
            custom_prompt = request.form.get('prompt')
            logger.info(f"接收到文件上传，大小：{len(image_data)} 字节")
        
        # 方式2：接收 base64 编码
        elif request.is_json:
            json_data = request.json
            if 'image_base64' in json_data:
                base64_str = json_data['image_base64']
                # 移除可能的 data:image/png;base64, 前缀
                if ',' in base64_str:
                    base64_str = base64_str.split(',')[1]
                image_data = base64.b64decode(base64_str)
                custom_prompt = json_data.get('prompt')
                logger.info(f"接收到 base64 数据，解码后大小：{len(image_data)} 字节")
        
        # 方式3：接收原始二进制数据
        elif request.content_type and 'image' in request.content_type:
            image_data = request.get_data()
            logger.info(f"接收到原始图片数据，大小：{len(image_data)} 字节")
        
        if not image_data:
            logger.warning("请求中未找到图片数据")
            return jsonify({
                'success': False,
                'error': '未找到图片数据'
            }), 400
        
        # 执行识别
        result = recognize_captcha_with_qwen(image_data, custom_prompt)
        
        if result['success']:
            return jsonify(result)
        else:
            return jsonify(result), 500
    
    except Exception as e:
        logger.error(f"处理请求失败：{str(e)}", exc_info=True)
        return jsonify({
            'success': False,
            'error': str(e)
        }), 500


@app.route('/batch-ocr', methods=['POST'])
def batch_recognize():
    """批量识别接口"""
    if model is None:
        return jsonify({
            'success': False,
            'error': '模型未初始化'
        }), 503
    
    try:
        if 'images' not in request.files:
            return jsonify({
                'success': False,
                'error': '请上传图片文件（字段名：images，支持多文件）'
            }), 400
        
        files = request.files.getlist('images')
        results = []
        
        for file in files:
            image_data = file.read()
            result = recognize_captcha_with_qwen(image_data)
            results.append({
                'filename': file.filename,
                'text': result.get('text', ''),
                'confidence': result.get('confidence', 0.0),
                'success': result.get('success', False)
            })
        
        logger.info(f"批量识别完成，共 {len(results)} 张图片")
        
        return jsonify({
            'success': True,
            'count': len(results),
            'results': results
        })
    
    except Exception as e:
        logger.error(f"批量识别失败：{str(e)}", exc_info=True)
        return jsonify({
            'success': False,
            'error': str(e)
        }), 500


if __name__ == '__main__':
    print("\n" + "="*70)
    print("🚀 Qwen2-VL 智能验证码识别服务")
    print("="*70)
    
    # 初始化模型
    if not initialize_model():
        print("\n❌ 模型初始化失败，服务无法启动")
        print("\n请执行以下步骤：")
        print("1. 安装依赖: pip install -r requirements_qwen.txt")
        print("2. 下载模型: huggingface-cli download Qwen/Qwen2-VL-2B-Instruct")
        print("3. 如果下载慢，可使用镜像: export HF_ENDPOINT=https://hf-mirror.com")
        exit(1)
    
    print(f"\n✅ 模型加载成功")
    print(f"📦 模型: {MODEL_NAME}")
    print(f"💻 设备: {device}")
    print(f"🌐 服务地址: http://localhost:5000")
    print(f"❤️  健康检查: http://localhost:5000/health")
    print(f"🔍 识别接口: POST http://localhost:5000/ocr")
    print("="*70 + "\n")
    
    # 启动服务
    app.run(
        host='0.0.0.0',
        port=5000,
        debug=False,
        threaded=False  # Qwen2-VL 建议单线程
    )
