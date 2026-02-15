#!/usr/bin/env python3
"""
测试 Qwen2-VL 验证码识别服务
"""

import requests
import sys
import base64
from pathlib import Path


def test_health():
    """测试健康检查接口"""
    print("🔍 测试健康检查...")
    try:
        response = requests.get("http://localhost:5000/health")
        data = response.json()
        
        print(f"✅ 服务状态: {data.get('status')}")
        print(f"📦 模型: {data.get('model')}")
        print(f"💻 设备: {data.get('device')}")
        print(f"🎮 GPU可用: {data.get('gpu_available')}")
        print(f"📊 模型状态: {data.get('model_status')}")
        return True
    except Exception as e:
        print(f"❌ 健康检查失败: {e}")
        return False


def test_ocr_with_file(image_path: str):
    """测试文件上传方式识别验证码"""
    print(f"\n🔍 测试识别验证码 (文件上传): {image_path}")
    
    if not Path(image_path).exists():
        print(f"❌ 文件不存在: {image_path}")
        return False
    
    try:
        with open(image_path, 'rb') as f:
            files = {'image': f}
            response = requests.post(
                "http://localhost:5000/ocr",
                files=files,
                timeout=30
            )
        
        data = response.json()
        
        if data.get('success'):
            print(f"✅ 识别成功: {data.get('text')}")
            print(f"📊 置信度: {data.get('confidence'):.2%}")
            print(f"📝 原始输出: {data.get('raw_response', 'N/A')}")
            return True
        else:
            print(f"❌ 识别失败: {data.get('error')}")
            return False
            
    except Exception as e:
        print(f"❌ 请求失败: {e}")
        return False


def test_ocr_with_base64(image_path: str):
    """测试 Base64 方式识别验证码"""
    print(f"\n🔍 测试识别验证码 (Base64): {image_path}")
    
    if not Path(image_path).exists():
        print(f"❌ 文件不存在: {image_path}")
        return False
    
    try:
        # 读取并编码
        with open(image_path, 'rb') as f:
            image_base64 = base64.b64encode(f.read()).decode('utf-8')
        
        response = requests.post(
            "http://localhost:5000/ocr",
            json={'image_base64': image_base64},
            timeout=30
        )
        
        data = response.json()
        
        if data.get('success'):
            print(f"✅ 识别成功: {data.get('text')}")
            print(f"📊 置信度: {data.get('confidence'):.2%}")
            return True
        else:
            print(f"❌ 识别失败: {data.get('error')}")
            return False
            
    except Exception as e:
        print(f"❌ 请求失败: {e}")
        return False


def test_ocr_with_custom_prompt(image_path: str, prompt: str):
    """测试自定义提示词"""
    print(f"\n🔍 测试自定义提示词识别")
    print(f"📝 提示词: {prompt}")
    
    if not Path(image_path).exists():
        print(f"❌ 文件不存在: {image_path}")
        return False
    
    try:
        with open(image_path, 'rb') as f:
            files = {'image': f}
            data = {'prompt': prompt}
            response = requests.post(
                "http://localhost:5000/ocr",
                files=files,
                data=data,
                timeout=30
            )
        
        result = response.json()
        
        if result.get('success'):
            print(f"✅ 识别成功: {result.get('text')}")
            print(f"📊 置信度: {result.get('confidence'):.2%}")
            return True
        else:
            print(f"❌ 识别失败: {result.get('error')}")
            return False
            
    except Exception as e:
        print(f"❌ 请求失败: {e}")
        return False


def main():
    print("\n" + "="*70)
    print("🧪 Qwen2-VL 验证码识别服务测试")
    print("="*70 + "\n")
    
    # 测试健康检查
    if not test_health():
        print("\n❌ 服务未启动或不可用")
        print("请先启动服务: python qwen_captcha_service.py")
        sys.exit(1)
    
    # 如果提供了图片路径，进行识别测试
    if len(sys.argv) > 1:
        image_path = sys.argv[1]
        
        # 测试文件上传方式
        test_ocr_with_file(image_path)
        
        # 测试 Base64 方式
        test_ocr_with_base64(image_path)
        
        # 测试自定义提示词（算术题）
        if len(sys.argv) > 2:
            custom_prompt = sys.argv[2]
            test_ocr_with_custom_prompt(image_path, custom_prompt)
    else:
        print("\n💡 使用方法:")
        print("  python test_qwen_captcha.py <验证码图片路径> [自定义提示词]")
        print("\n示例:")
        print("  python test_qwen_captcha.py captcha.png")
        print("  python test_qwen_captcha.py math_captcha.png '请计算图片中的算术题'")
    
    print("\n" + "="*70)
    print("✅ 测试完成")
    print("="*70 + "\n")


if __name__ == '__main__':
    main()
