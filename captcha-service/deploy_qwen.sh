#!/bin/bash

# Qwen2-VL 验证码服务一键部署脚本
# 支持 CPU 和 GPU 模式

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 配置
SERVICE_NAME="qwen-captcha"
VENV_DIR="venv_qwen"
PYTHON_VERSION="3.10"

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

# 检查 Python 版本
check_python() {
    if command -v python3 &> /dev/null; then
        PYTHON_CMD="python3"
    elif command -v python &> /dev/null; then
        PYTHON_CMD="python"
    else
        print_error "未找到 Python，请先安装 Python 3.10+"
        exit 1
    fi
    
    PYTHON_VER=$($PYTHON_CMD --version | cut -d' ' -f2 | cut -d'.' -f1,2)
    print_info "检测到 Python 版本: $PYTHON_VER"
}

# 检查 GPU
check_gpu() {
    if command -v nvidia-smi &> /dev/null; then
        print_info "检测到 NVIDIA GPU"
        nvidia-smi --query-gpu=name,memory.total --format=csv,noheader
        USE_GPU=true
    else
        print_warn "未检测到 NVIDIA GPU，将使用 CPU 模式（速度较慢）"
        USE_GPU=false
    fi
}

# 创建虚拟环境
create_venv() {
    print_info "创建 Python 虚拟环境..."
    
    if [ -d "$VENV_DIR" ]; then
        print_warn "虚拟环境已存在，跳过创建"
    else
        $PYTHON_CMD -m venv $VENV_DIR
        print_info "虚拟环境创建完成"
    fi
    
    # 激活虚拟环境
    source $VENV_DIR/bin/activate
}

# 安装依赖
install_dependencies() {
    print_info "安装 Python 依赖..."
    
    # 升级 pip
    pip install --upgrade pip
    
    # 安装 PyTorch（根据 GPU 情况选择版本）
    if [ "$USE_GPU" = true ]; then
        print_info "安装 PyTorch GPU 版本..."
        pip install torch torchvision --index-url https://download.pytorch.org/whl/cu121
    else
        print_info "安装 PyTorch CPU 版本..."
        pip install torch torchvision --index-url https://download.pytorch.org/whl/cpu
    fi
    
    # 安装其他依赖
    pip install -r requirements_qwen.txt
    
    print_info "依赖安装完成"
}

# 下载模型
download_model() {
    print_info "检查模型文件..."
    
    # 设置国内镜像（可选）
    read -p "是否使用国内镜像加速下载？(y/n): " use_mirror
    if [ "$use_mirror" = "y" ]; then
        export HF_ENDPOINT=https://hf-mirror.com
        print_info "已启用 HuggingFace 国内镜像"
    fi
    
    # 检查模型是否已下载
    MODEL_PATH="$HOME/.cache/huggingface/hub/models--Qwen--Qwen2-VL-2B-Instruct"
    
    if [ -d "$MODEL_PATH" ]; then
        print_info "模型已存在，跳过下载"
    else
        print_info "首次运行需要下载模型（约 4GB），请耐心等待..."
        huggingface-cli download Qwen/Qwen2-VL-2B-Instruct
        print_info "模型下载完成"
    fi
}

# 启动服务
start_service() {
    print_info "启动 Qwen2-VL 验证码识别服务..."
    
    # 激活虚拟环境
    source $VENV_DIR/bin/activate
    
    # 设置环境变量
    export USE_GPU=$USE_GPU
    export QWEN_MODEL="Qwen/Qwen2-VL-2B-Instruct"
    
    # 启动服务
    nohup uvicorn app:app --host 0.0.0.0 --port 5000 > logs/qwen_captcha.log 2>&1 &
    echo $! > $SERVICE_NAME.pid
    
    sleep 3
    
    # 检查服务是否启动成功
    if curl -s http://localhost:5000/health > /dev/null; then
        print_info "✅ 服务启动成功！"
        print_info "服务地址: http://localhost:5000"
        print_info "健康检查: http://localhost:5000/health"
        print_info "日志文件: logs/qwen_captcha.log"
    else
        print_error "服务启动失败，请查看日志: tail -f logs/qwen_captcha.log"
        exit 1
    fi
}

# 停止服务
stop_service() {
    print_info "停止服务..."
    
    if [ -f "$SERVICE_NAME.pid" ]; then
        PID=$(cat $SERVICE_NAME.pid)
        if ps -p $PID > /dev/null; then
            kill $PID
            rm $SERVICE_NAME.pid
            print_info "服务已停止"
        else
            print_warn "进程不存在"
            rm $SERVICE_NAME.pid
        fi
    else
        print_warn "未找到 PID 文件"
    fi
}

# 查看状态
show_status() {
    if [ -f "$SERVICE_NAME.pid" ]; then
        PID=$(cat $SERVICE_NAME.pid)
        if ps -p $PID > /dev/null; then
            print_info "服务运行中 (PID: $PID)"
            
            # 检查健康状态
            if curl -s http://localhost:5000/health > /dev/null; then
                print_info "健康检查: ✅ 正常"
            else
                print_warn "健康检查: ❌ 异常"
            fi
        else
            print_warn "服务未运行"
        fi
    else
        print_warn "服务未运行"
    fi
}

# 查看日志
show_logs() {
    if [ -f "logs/qwen_captcha.log" ]; then
        tail -f logs/qwen_captcha.log
    else
        print_error "日志文件不存在"
    fi
}

# 完整安装
install_all() {
    print_info "开始安装 Qwen2-VL 验证码识别服务"
    print_info "============================================"
    
    check_python
    check_gpu
    create_venv
    install_dependencies
    download_model
    
    # 创建日志目录
    mkdir -p logs
    
    print_info "============================================"
    print_info "✅ 安装完成！"
    print_info ""
    print_info "使用方法:"
    print_info "  启动服务: ./deploy_qwen.sh start"
    print_info "  停止服务: ./deploy_qwen.sh stop"
    print_info "  查看状态: ./deploy_qwen.sh status"
    print_info "  查看日志: ./deploy_qwen.sh logs"
}

# 主菜单
case "$1" in
    install)
        install_all
        ;;
    start)
        start_service
        ;;
    stop)
        stop_service
        ;;
    restart)
        stop_service
        sleep 2
        start_service
        ;;
    status)
        show_status
        ;;
    logs)
        show_logs
        ;;
    *)
        echo "Qwen2-VL 验证码识别服务管理脚本"
        echo ""
        echo "使用方法: $0 {install|start|stop|restart|status|logs}"
        echo ""
        echo "命令说明:"
        echo "  install  - 完整安装（首次使用）"
        echo "  start    - 启动服务"
        echo "  stop     - 停止服务"
        echo "  restart  - 重启服务"
        echo "  status   - 查看状态"
        echo "  logs     - 查看日志"
        exit 1
esac
