#!/bin/bash
set -e

echo "================================"
echo "🚀 启动招标监控系统"
echo "================================"

# 启动验证码服务（后台）
echo "📦 启动验证码识别服务..."
cd /app/captcha-service
uvicorn app:app --host 0.0.0.0 --port 5000 &
CAPTCHA_PID=$!

# 等待验证码服务启动
sleep 5

# 启动主程序
echo "🚀 启动主程序..."
cd /app
./tender-monitor &
MAIN_PID=$!

echo ""
echo "================================"
echo "✅ 服务已启动"
echo "================================"
echo "验证码服务 PID: $CAPTCHA_PID"
echo "主程序 PID: $MAIN_PID"
echo ""
echo "🌐 访问地址："
echo "   主程序: http://localhost:8080"
echo "   验证码服务: http://localhost:5000"
echo "================================"

# 等待进程
wait $MAIN_PID
