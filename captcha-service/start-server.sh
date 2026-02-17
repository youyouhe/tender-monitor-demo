#!/bin/bash

# ============================================================
# 验证码识别服务 - 服务器端启动脚本
# 支持 ddddocr (轻量) 和 Qwen2-VL (智能) 两种引擎
# ============================================================

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 配置
SERVICE_NAME="captcha-service"
VENV_DIR="venv"
PORT="${PORT:-5000}"
HOST="${HOST:-0.0.0.0}"
WORKERS="${WORKERS:-1}"
PID_FILE="/tmp/${SERVICE_NAME}.pid"
LOG_FILE="logs/captcha-service.log"

# 打印带颜色的消息
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_title() {
    echo -e "${BLUE}$1${NC}"
}

# 显示帮助信息
show_help() {
    cat << EOF
用法: ./start-server.sh [命令] [选项]

命令:
  install       安装依赖（首次使用）
  start         启动服务（前台运行）
  daemon        启动服务（后台运行）
  stop          停止服务
  restart       重启服务
  status        查看服务状态
  logs          查看日志
  test          测试服务

选项:
  --port=5000   指定端口（默认: 5000）
  --workers=1   工作进程数（默认: 1）
  --help        显示帮助信息

示例:
  ./start-server.sh install              # 首次安装
  ./start-server.sh start                # 前台启动
  ./start-server.sh daemon               # 后台启动
  ./start-server.sh stop                 # 停止服务
  ./start-server.sh status               # 查看状态
  ./start-server.sh start --port=8000    # 指定端口启动

EOF
}

# 解析参数
parse_args() {
    for arg in "$@"; do
        case $arg in
            --port=*)
                PORT="${arg#*=}"
                ;;
            --workers=*)
                WORKERS="${arg#*=}"
                ;;
            --help)
                show_help
                exit 0
                ;;
        esac
    done
}

# 检查 Python
check_python() {
    if command -v python3 &> /dev/null; then
        PYTHON_CMD="python3"
    elif command -v python &> /dev/null; then
        PYTHON_CMD="python"
    else
        print_error "未找到 Python，请先安装 Python 3.10+"
        echo "Ubuntu/Debian: sudo apt install python3 python3-pip python3-venv"
        echo "CentOS/RHEL:   sudo yum install python3 python3-pip"
        exit 1
    fi

    PYTHON_VER=$($PYTHON_CMD --version 2>&1 | grep -oP '\d+\.\d+' | head -1)
    print_info "检测到 Python 版本: $PYTHON_VER"
}

# 安装依赖
install_dependencies() {
    print_title "============================================================"
    print_title "  安装验证码识别服务依赖"
    print_title "============================================================"
    echo ""

    check_python

    # 创建虚拟环境
    if [ ! -d "$VENV_DIR" ]; then
        print_info "创建 Python 虚拟环境..."
        $PYTHON_CMD -m venv $VENV_DIR
    else
        print_info "虚拟环境已存在"
    fi

    # 激活虚拟环境
    source $VENV_DIR/bin/activate

    # 升级 pip
    print_info "升级 pip..."
    pip install --upgrade pip -q

    # 安装基础依赖（ddddocr 引擎）
    print_info "安装基础依赖（FastAPI + ddddocr）..."
    pip install -r requirements.txt -q

    # 询问是否安装 Qwen2-VL
    echo ""
    print_warn "是否安装 Qwen2-VL 智能识别引擎？（需要 GPU，可选）"
    read -p "输入 y 安装，其他键跳过: " install_qwen

    if [ "$install_qwen" = "y" ] || [ "$install_qwen" = "Y" ]; then
        print_info "安装 Qwen2-VL 依赖..."
        pip install transformers torch torchvision pillow qwen-vl-utils -q
        print_info "Qwen2-VL 安装完成（首次使用会自动下载模型 ~4GB）"
    fi

    # 创建日志目录
    mkdir -p logs

    echo ""
    print_info "✅ 依赖安装完成！"
    echo ""
    print_info "下一步："
    print_info "  启动服务: ./start-server.sh start"
    print_info "  后台运行: ./start-server.sh daemon"
    echo ""
}

# 启动服务（前台）
start_service() {
    print_title "============================================================"
    print_title "  启动验证码识别服务"
    print_title "============================================================"
    echo ""

    # 检查虚拟环境
    if [ ! -d "$VENV_DIR" ]; then
        print_error "虚拟环境不存在，请先运行: ./start-server.sh install"
        exit 1
    fi

    # 激活虚拟环境
    source $VENV_DIR/bin/activate

    # 检查端口占用
    if lsof -Pi :$PORT -sTCP:LISTEN -t >/dev/null 2>&1; then
        print_warn "端口 $PORT 已被占用"
        PID=$(lsof -Pi :$PORT -sTCP:LISTEN -t)
        print_warn "占用进程 PID: $PID"
        print_error "请先停止现有服务: ./start-server.sh stop"
        exit 1
    fi

    print_info "配置信息:"
    print_info "  监听地址: $HOST:$PORT"
    print_info "  工作进程: $WORKERS"
    print_info "  虚拟环境: $VENV_DIR"
    echo ""
    print_info "启动服务..."
    echo ""

    # 启动服务
    if [ "$WORKERS" -eq 1 ]; then
        # 单进程模式
        uvicorn app:app --host $HOST --port $PORT
    else
        # 多进程模式
        uvicorn app:app --host $HOST --port $PORT --workers $WORKERS
    fi
}

# 后台启动服务
start_daemon() {
    print_title "============================================================"
    print_title "  后台启动验证码识别服务"
    print_title "============================================================"
    echo ""

    # 检查虚拟环境
    if [ ! -d "$VENV_DIR" ]; then
        print_error "虚拟环境不存在，请先运行: ./start-server.sh install"
        exit 1
    fi

    # 检查是否已运行
    if [ -f "$PID_FILE" ] && kill -0 $(cat "$PID_FILE") 2>/dev/null; then
        print_warn "服务已在运行，PID: $(cat $PID_FILE)"
        exit 0
    fi

    # 检查端口占用
    if lsof -Pi :$PORT -sTCP:LISTEN -t >/dev/null 2>&1; then
        print_warn "端口 $PORT 已被占用"
        PID=$(lsof -Pi :$PORT -sTCP:LISTEN -t)
        print_warn "占用进程 PID: $PID"
        print_error "请先停止现有服务: ./start-server.sh stop"
        exit 1
    fi

    # 创建日志目录
    mkdir -p logs

    # 激活虚拟环境并启动
    source $VENV_DIR/bin/activate

    print_info "配置信息:"
    print_info "  监听地址: $HOST:$PORT"
    print_info "  工作进程: $WORKERS"
    print_info "  日志文件: $LOG_FILE"
    echo ""

    # 后台启动
    nohup uvicorn app:app --host $HOST --port $PORT --workers $WORKERS > $LOG_FILE 2>&1 &
    echo $! > $PID_FILE

    sleep 2

    # 验证启动
    if kill -0 $(cat "$PID_FILE") 2>/dev/null; then
        print_info "✅ 服务已启动"
        print_info "  PID: $(cat $PID_FILE)"
        print_info "  访问地址: http://$HOST:$PORT"
        print_info "  健康检查: curl http://localhost:$PORT/health"
        echo ""
        print_info "查看日志: ./start-server.sh logs"
    else
        print_error "❌ 服务启动失败，请查看日志: $LOG_FILE"
        exit 1
    fi
}

# 停止服务
stop_service() {
    print_title "============================================================"
    print_title "  停止验证码识别服务"
    print_title "============================================================"
    echo ""

    if [ ! -f "$PID_FILE" ]; then
        print_warn "PID 文件不存在"

        # 尝试通过端口查找进程
        if lsof -Pi :$PORT -sTCP:LISTEN -t >/dev/null 2>&1; then
            PID=$(lsof -Pi :$PORT -sTCP:LISTEN -t)
            print_info "找到占用端口 $PORT 的进程: $PID"
            kill $PID
            print_info "✅ 已停止进程 $PID"
        else
            print_warn "未找到运行中的服务"
        fi
        return
    fi

    PID=$(cat "$PID_FILE")

    if kill -0 $PID 2>/dev/null; then
        print_info "停止服务 (PID: $PID)..."
        kill $PID

        # 等待进程结束
        for i in {1..10}; do
            if ! kill -0 $PID 2>/dev/null; then
                break
            fi
            sleep 1
        done

        # 强制结束
        if kill -0 $PID 2>/dev/null; then
            print_warn "进程未响应，强制结束..."
            kill -9 $PID
        fi

        rm -f $PID_FILE
        print_info "✅ 服务已停止"
    else
        print_warn "进程不存在 (PID: $PID)"
        rm -f $PID_FILE
    fi
}

# 重启服务
restart_service() {
    print_title "============================================================"
    print_title "  重启验证码识别服务"
    print_title "============================================================"
    echo ""

    stop_service
    sleep 2
    start_daemon
}

# 查看状态
show_status() {
    print_title "============================================================"
    print_title "  验证码识别服务状态"
    print_title "============================================================"
    echo ""

    # 检查 PID 文件
    if [ -f "$PID_FILE" ]; then
        PID=$(cat "$PID_FILE")
        if kill -0 $PID 2>/dev/null; then
            print_info "✅ 服务运行中"
            print_info "  PID: $PID"
            print_info "  端口: $PORT"

            # 尝试健康检查
            if command -v curl &> /dev/null; then
                echo ""
                print_info "健康检查:"
                curl -s http://localhost:$PORT/health | python3 -m json.tool 2>/dev/null || echo "  无法连接"
            fi
        else
            print_warn "❌ 服务未运行（PID 文件存在但进程不存在）"
            rm -f $PID_FILE
        fi
    else
        # 检查端口
        if lsof -Pi :$PORT -sTCP:LISTEN -t >/dev/null 2>&1; then
            PID=$(lsof -Pi :$PORT -sTCP:LISTEN -t)
            print_warn "⚠️  端口 $PORT 被占用 (PID: $PID)，但 PID 文件不存在"
        else
            print_warn "❌ 服务未运行"
        fi
    fi

    echo ""
    print_info "命令:"
    print_info "  启动: ./start-server.sh daemon"
    print_info "  停止: ./start-server.sh stop"
    print_info "  日志: ./start-server.sh logs"
    echo ""
}

# 查看日志
show_logs() {
    if [ -f "$LOG_FILE" ]; then
        print_info "实时日志 (Ctrl+C 退出):"
        echo ""
        tail -f $LOG_FILE
    else
        print_warn "日志文件不存在: $LOG_FILE"
        print_info "服务可能未以后台模式启动"
    fi
}

# 测试服务
test_service() {
    print_title "============================================================"
    print_title "  测试验证码识别服务"
    print_title "============================================================"
    echo ""

    # 检查虚拟环境
    if [ ! -d "$VENV_DIR" ]; then
        print_error "虚拟环境不存在，请先运行: ./start-server.sh install"
        exit 1
    fi

    # 激活虚拟环境
    source $VENV_DIR/bin/activate

    # 运行测试脚本
    if [ -f "test_captcha.py" ]; then
        print_info "运行 ddddocr 测试..."
        python3 test_captcha.py
    else
        print_warn "测试文件不存在: test_captcha.py"
    fi
}

# 主函数
main() {
    # 解析参数
    parse_args "$@"

    # 获取命令
    COMMAND="${1:-help}"

    case $COMMAND in
        install)
            install_dependencies
            ;;
        start)
            start_service
            ;;
        daemon)
            start_daemon
            ;;
        stop)
            stop_service
            ;;
        restart)
            restart_service
            ;;
        status)
            show_status
            ;;
        logs)
            show_logs
            ;;
        test)
            test_service
            ;;
        help|--help|-h)
            show_help
            ;;
        *)
            print_error "未知命令: $COMMAND"
            echo ""
            show_help
            exit 1
            ;;
    esac
}

# 运行主函数
main "$@"
