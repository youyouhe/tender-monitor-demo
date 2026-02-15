#!/usr/bin/env python3
"""
简单的验证码识别脚本
用于 shandong-tender skill
"""

import sys
import base64

try:
    import ddddocr
    ocr = ddddocr.DdddOcr(show_ad=False)

    # 读取图片文件或 base64
    if len(sys.argv) > 1:
        image_path = sys.argv[1]
        with open(image_path, 'rb') as f:
            image_data = f.read()
    else:
        # 从 stdin 读取 base64
        image_data = base64.b64decode(sys.stdin.read().strip())

    # 识别
    result = ocr.classification(image_data)
    print(result)

except ImportError:
    print("ERROR: ddddocr not installed", file=sys.stderr)
    sys.exit(1)
except Exception as e:
    print(f"ERROR: {e}", file=sys.stderr)
    sys.exit(1)
