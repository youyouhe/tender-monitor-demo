#!/usr/bin/env python3
"""
验证码服务测试脚本
"""

import requests
import base64
import sys
import os


def test_health():
    """测试健康检查接口"""
    print("🔍 测试健康检查接口...")
    try:
        response = requests.get('http://localhost:5000/health', timeout=5)
        if response.status_code == 200:
            print("✅ 健康检查通过")
            print(f"   响应：{response.json()}")
            return True
        else:
            print(f"❌ 健康检查失败：HTTP {response.status_code}")
            return False
    except Exception as e:
        print(f"❌ 连接失败：{e}")
        return False


def test_ocr_file(image_path):
    """测试文件上传方式"""
    print(f"\n🔍 测试文件上传识别...")
    if not os.path.exists(image_path):
        print(f"❌ 图片文件不存在：{image_path}")
        return False

    try:
        with open(image_path, 'rb') as f:
            files = {'image': f}
            response = requests.post('http://localhost:5000/ocr', files=files, timeout=10)

        if response.status_code == 200:
            result = response.json()
            if result.get('success'):
                print(f"✅ 识别成功：{result['text']}")
                print(f"   置信度：{result['confidence']}")
                return True
            else:
                print(f"❌ 识别失败：{result.get('error')}")
                return False
        else:
            print(f"❌ 请求失败：HTTP {response.status_code}")
            return False
    except Exception as e:
        print(f"❌ 测试失败：{e}")
        return False


def test_ocr_base64(image_path):
    """测试 base64 方式"""
    print(f"\n🔍 测试 Base64 识别...")
    if not os.path.exists(image_path):
        print(f"❌ 图片文件不存在：{image_path}")
        return False

    try:
        with open(image_path, 'rb') as f:
            image_data = f.read()
            image_base64 = base64.b64encode(image_data).decode('utf-8')

        response = requests.post(
            'http://localhost:5000/ocr',
            json={'image_base64': image_base64},
            timeout=10
        )

        if response.status_code == 200:
            result = response.json()
            if result.get('success'):
                print(f"✅ 识别成功：{result['text']}")
                print(f"   置信度：{result['confidence']}")
                return True
            else:
                print(f"❌ 识别失败：{result.get('error')}")
                return False
        else:
            print(f"❌ 请求失败：HTTP {response.status_code}")
            return False
    except Exception as e:
        print(f"❌ 测试失败：{e}")
        return False


def main():
    print("="*60)
    print("🧪 验证码服务测试")
    print("="*60)

    # 测试健康检查
    if not test_health():
        print("\n❌ 服务未启动或不可用")
        print("   请先运行：python captcha_service.py")
        sys.exit(1)

    # 检查是否提供了测试图片
    if len(sys.argv) > 1:
        image_path = sys.argv[1]
        test_ocr_file(image_path)
        test_ocr_base64(image_path)
    else:
        print("\n💡 提示：可以指定验证码图片进行测试")
        print("   用法：python test_captcha.py <图片路径>")

    print("\n" + "="*60)
    print("✅ 测试完成")
    print("="*60)


if __name__ == '__main__':
    main()
