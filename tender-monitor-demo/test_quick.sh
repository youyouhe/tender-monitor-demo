#!/bin/bash

# 快速测试脚本

echo "================================"
echo "🧪 山东省采集快速测试"
echo "================================"

cd /workspace/group/tender-monitor

# 检查依赖
echo ""
echo "📋 检查环境..."
go version || echo "❌ Go 未安装"
python3 --version || echo "❌ Python 未安装"

# 检查轨迹文件
echo ""
echo "📄 检查轨迹文件..."
if [ -f "traces/shandong_list.json" ]; then
    echo "✅ 列表页轨迹: traces/shandong_list.json"
else
    echo "❌ 列表页轨迹文件不存在"
fi

if [ -f "traces/shandong_detail.json" ]; then
    echo "✅ 详情页轨迹: traces/shandong_detail.json"
else
    echo "❌ 详情页轨迹文件不存在"
fi

echo ""
echo "================================"
echo "📝 当前轨迹文件内容"
echo "================================"

echo ""
echo "【列表页轨迹 - 关键步骤】"
cat traces/shandong_list.json | grep -A 2 "action"

echo ""
echo ""
echo "【详情页轨迹 - 关键步骤】"
cat traces/shandong_detail.json | grep -A 2 "action"

echo ""
echo "================================"
echo "🎯 下一步"
echo "================================"
echo ""
echo "轨迹文件已经存在，但可能需要调整："
echo ""
echo "1. 验证码图片选择器"
echo "   当前：img[src*='captcha']"
echo "   → 需要访问网站确认"
echo ""
echo "2. 列表数据选择器"
echo "   当前：tbody tr → td:nth-child(3)"
echo "   → 需要检查实际表格结构"
echo ""
echo "3. 详情页字段选择器"
echo "   当前：td:contains('预算金额') + td"
echo "   → 需要检查实际页面结构"
echo ""
echo "================================"
echo "🚀 开始测试"
echo "================================"
echo ""
echo "方式 1：完整部署（推荐）"
echo "   ./deploy.sh install"
echo ""
echo "方式 2：只启动服务（如果已安装）"
echo "   ./deploy.sh start"
echo ""
echo "方式 3：直接运行测试"
echo "   go run main.go"
echo ""
