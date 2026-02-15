#!/usr/bin/env python3
"""
验证码识别服务
使用 ddddocr 库提供 HTTP API 接口
"""

from flask import Flask, request, jsonify
from flask_cors import CORS
import ddddocr
import base64
import logging
from io import BytesIO

# 配置日志
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

app = Flask(__name__)
CORS(app)  # 允许跨域请求

# 初始化 OCR 引擎（程序启动时加载一次）
logger.info("正在初始化 ddddocr 引擎...")
ocr = ddddocr.DdddOcr(show_ad=False)
logger.info("ddddocr 引擎初始化完成")


@app.route('/health', methods=['GET'])
def health_check():
    """健康检查接口"""
    return jsonify({
        'status': 'ok',
        'service': 'captcha-ocr',
        'version': '1.0.0'
    })


@app.route('/ocr', methods=['POST'])
def recognize_captcha():
    """
    验证码识别接口

    请求方式：
    1. multipart/form-data 上传图片文件（字段名：image）
    2. application/json 传递 base64 编码的图片（字段名：image_base64）

    返回格式：
    {
        "success": true,
        "text": "识别结果",
        "confidence": 0.95  // 预留字段，当前版本固定返回1.0
    }
    """
    try:
        image_data = None

        # 方式1：接收文件上传
        if 'image' in request.files:
            file = request.files['image']
            image_data = file.read()
            logger.info(f"接收到文件上传，大小：{len(image_data)} 字节")

        # 方式2：接收 base64 编码
        elif request.is_json and 'image_base64' in request.json:
            base64_str = request.json['image_base64']
            # 移除可能的 data:image/png;base64, 前缀
            if ',' in base64_str:
                base64_str = base64_str.split(',')[1]
            image_data = base64.b64decode(base64_str)
            logger.info(f"接收到 base64 数据，解码后大小：{len(image_data)} 字节")

        # 方式3：接收原始二进制数据
        elif request.content_type and 'image' in request.content_type:
            image_data = request.get_data()
            logger.info(f"接收到原始图片数据，大小：{len(image_data)} 字节")

        if not image_data:
            logger.warning("请求中未找到图片数据")
            return jsonify({
                'success': False,
                'error': '未找到图片数据，请使用 multipart/form-data 上传文件或传递 base64 编码'
            }), 400

        # 执行 OCR 识别
        result_text = ocr.classification(image_data)
        logger.info(f"识别成功：{result_text}")

        return jsonify({
            'success': True,
            'text': result_text,
            'confidence': 1.0  # ddddocr 不提供置信度，固定返回1.0
        })

    except Exception as e:
        logger.error(f"识别失败：{str(e)}", exc_info=True)
        return jsonify({
            'success': False,
            'error': str(e)
        }), 500


@app.route('/batch-ocr', methods=['POST'])
def batch_recognize():
    """
    批量识别接口（预留）
    接收多张图片，返回多个识别结果
    """
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
            text = ocr.classification(image_data)
            results.append({
                'filename': file.filename,
                'text': text,
                'confidence': 1.0
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
    print("\n" + "="*60)
    print("🚀 验证码识别服务启动中...")
    print("="*60)
    print(f"📍 服务地址：http://localhost:5000")
    print(f"📍 健康检查：http://localhost:5000/health")
    print(f"📍 识别接口：POST http://localhost:5000/ocr")
    print("="*60 + "\n")

    # 生产环境建议使用 gunicorn 或 uwsgi
    app.run(
        host='0.0.0.0',
        port=5000,
        debug=False,  # 生产环境关闭 debug
        threaded=True  # 支持多线程
    )
