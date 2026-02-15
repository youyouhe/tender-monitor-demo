#!/bin/bash

# 招标信息监控系统 - 部署脚本

set -e

echo "================================"
echo "🚀 招标信息监控系统部署"
echo "================================"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 检查依赖
check_dependencies() {
    echo -e "\n${YELLOW}📋 检查依赖...${NC}"

    # 检查 Go
    if ! command -v go &> /dev/null; then
        echo -e "${RED}❌ Go 未安装${NC}"
        echo "请访问 https://golang.org/dl/ 安装 Go 1.21+"
        exit 1
    fi
    echo -e "${GREEN}✅ Go $(go version | awk '{print $3}')${NC}"

    # 检查 Python
    if ! command -v python3 &> /dev/null; then
        echo -e "${RED}❌ Python3 未安装${NC}"
        exit 1
    fi
    echo -e "${GREEN}✅ Python $(python3 --version | awk '{print $2}')${NC}"

    # 检查 Docker（可选）
    if command -v docker &> /dev/null; then
        echo -e "${GREEN}✅ Docker $(docker --version | awk '{print $3}' | tr -d ',')${NC}"
    else
        echo -e "${YELLOW}⚠️  Docker 未安装（可选，用于容器化部署）${NC}"
    fi
}

# 创建目录结构
setup_directories() {
    echo -e "\n${YELLOW}📁 创建目录结构...${NC}"
    mkdir -p data traces logs
    echo -e "${GREEN}✅ 目录创建完成${NC}"
}

# 安装 Go 依赖
install_go_deps() {
    echo -e "\n${YELLOW}📦 安装 Go 依赖...${NC}"
    go mod init tender-monitor 2>/dev/null || true
    go get github.com/go-rod/rod
    go get github.com/mattn/go-sqlite3
    echo -e "${GREEN}✅ Go 依赖安装完成${NC}"
}

# 安装 Python 依赖
install_python_deps() {
    echo -e "\n${YELLOW}📦 安装 Python 依赖...${NC}"
    cd captcha-service

    # 创建虚拟环境（可选）
    if [ ! -d "venv" ]; then
        echo "创建 Python 虚拟环境..."
        python3 -m venv venv
    fi

    # 激活虚拟环境
    source venv/bin/activate 2>/dev/null || true

    # 安装依赖
    pip install -r requirements.txt -i https://pypi.tuna.tsinghua.edu.cn/simple

    cd ..
    echo -e "${GREEN}✅ Python 依赖安装完成${NC}"
}

# 构建 Go 程序
build_go() {
    echo -e "\n${YELLOW}🔨 编译 Go 程序...${NC}"
    go build -o tender-monitor main.go
    chmod +x tender-monitor
    echo -e "${GREEN}✅ 编译完成: ./tender-monitor${NC}"
}

# 启动验证码服务
start_captcha_service() {
    echo -e "\n${YELLOW}🚀 启动验证码服务...${NC}"

    cd captcha-service

    # 检查服务是否已在运行
    if lsof -Pi :5000 -sTCP:LISTEN -t >/dev/null 2>&1; then
        echo -e "${YELLOW}⚠️  端口 5000 已被占用${NC}"
        echo "是否停止现有服务？(y/n)"
        read -r answer
        if [ "$answer" = "y" ]; then
            kill $(lsof -t -i:5000) 2>/dev/null || true
            sleep 2
        else
            cd ..
            return
        fi
    fi

    # 使用 Docker 启动
    if command -v docker-compose &> /dev/null; then
        echo "使用 Docker Compose 启动..."
        docker-compose up -d
    else
        # 直接用 Python 启动
        echo "使用 Python 直接启动..."
        source venv/bin/activate 2>/dev/null || true
        nohup python captcha_service.py > ../logs/captcha.log 2>&1 &
        echo $! > ../logs/captcha.pid
    fi

    cd ..
    sleep 3

    # 检查服务
    if curl -s http://localhost:5000/health > /dev/null; then
        echo -e "${GREEN}✅ 验证码服务启动成功${NC}"
    else
        echo -e "${RED}❌ 验证码服务启动失败${NC}"
        exit 1
    fi
}

# 启动主程序
start_main_service() {
    echo -e "\n${YELLOW}🚀 启动主程序...${NC}"

    # 检查端口
    if lsof -Pi :8080 -sTCP:LISTEN -t >/dev/null 2>&1; then
        echo -e "${RED}❌ 端口 8080 已被占用${NC}"
        exit 1
    fi

    # 启动
    nohup ./tender-monitor > logs/tender-monitor.log 2>&1 &
    echo $! > logs/tender-monitor.pid

    sleep 3

    # 检查服务
    if curl -s http://localhost:8080/api/health > /dev/null; then
        echo -e "${GREEN}✅ 主程序启动成功${NC}"
        echo -e "\n${GREEN}🌐 访问地址: http://localhost:8080${NC}"
    else
        echo -e "${RED}❌ 主程序启动失败${NC}"
        echo "请查看日志: tail -f logs/tender-monitor.log"
        exit 1
    fi
}

# 停止服务
stop_services() {
    echo -e "\n${YELLOW}🛑 停止服务...${NC}"

    # 停止主程序
    if [ -f logs/tender-monitor.pid ]; then
        kill $(cat logs/tender-monitor.pid) 2>/dev/null || true
        rm logs/tender-monitor.pid
        echo -e "${GREEN}✅ 主程序已停止${NC}"
    fi

    # 停止验证码服务
    if [ -f logs/captcha.pid ]; then
        kill $(cat logs/captcha.pid) 2>/dev/null || true
        rm logs/captcha.pid
        echo -e "${GREEN}✅ 验证码服务已停止${NC}"
    fi

    # 停止 Docker 容器
    if [ -d captcha-service ]; then
        cd captcha-service
        docker-compose down 2>/dev/null || true
        cd ..
    fi
}

# 查看状态
check_status() {
    echo -e "\n${YELLOW}📊 服务状态${NC}"
    echo "================================"

    # 主程序
    if [ -f logs/tender-monitor.pid ] && ps -p $(cat logs/tender-monitor.pid) > /dev/null 2>&1; then
        echo -e "${GREEN}✅ 主程序运行中 (PID: $(cat logs/tender-monitor.pid))${NC}"
        echo "   地址: http://localhost:8080"
    else
        echo -e "${RED}❌ 主程序未运行${NC}"
    fi

    # 验证码服务
    if curl -s http://localhost:5000/health > /dev/null 2>&1; then
        echo -e "${GREEN}✅ 验证码服务运行中${NC}"
        echo "   地址: http://localhost:5000"
    else
        echo -e "${RED}❌ 验证码服务未运行${NC}"
    fi

    echo "================================"
}

# 查看日志
view_logs() {
    echo -e "\n${YELLOW}📋 查看日志${NC}"
    echo "1. 主程序日志"
    echo "2. 验证码服务日志"
    echo "3. 全部日志"
    read -p "选择 (1-3): " choice

    case $choice in
        1)
            tail -f logs/tender-monitor.log
            ;;
        2)
            tail -f logs/captcha.log
            ;;
        3)
            tail -f logs/*.log
            ;;
        *)
            echo "无效选择"
            ;;
    esac
}

# 主菜单
show_menu() {
    echo -e "\n${YELLOW}================================${NC}"
    echo -e "${YELLOW}招标信息监控系统 - 管理菜单${NC}"
    echo -e "${YELLOW}================================${NC}"
    echo "1. 完整部署（首次安装）"
    echo "2. 启动服务"
    echo "3. 停止服务"
    echo "4. 重启服务"
    echo "5. 查看状态"
    echo "6. 查看日志"
    echo "7. 退出"
    echo "================================"
    read -p "请选择操作 (1-7): " choice

    case $choice in
        1)
            check_dependencies
            setup_directories
            install_go_deps
            install_python_deps
            build_go
            start_captcha_service
            start_main_service
            check_status
            ;;
        2)
            start_captcha_service
            start_main_service
            check_status
            ;;
        3)
            stop_services
            ;;
        4)
            stop_services
            sleep 2
            start_captcha_service
            start_main_service
            check_status
            ;;
        5)
            check_status
            ;;
        6)
            view_logs
            ;;
        7)
            echo "👋 再见！"
            exit 0
            ;;
        *)
            echo -e "${RED}无效选择${NC}"
            show_menu
            ;;
    esac
}

# 如果有参数，直接执行命令
if [ $# -gt 0 ]; then
    case $1 in
        install)
            check_dependencies
            setup_directories
            install_go_deps
            install_python_deps
            build_go
            ;;
        start)
            start_captcha_service
            start_main_service
            ;;
        stop)
            stop_services
            ;;
        restart)
            stop_services
            sleep 2
            start_captcha_service
            start_main_service
            ;;
        status)
            check_status
            ;;
        logs)
            view_logs
            ;;
        *)
            echo "用法: $0 {install|start|stop|restart|status|logs}"
            exit 1
            ;;
    esac
else
    # 显示菜单
    show_menu
fi
