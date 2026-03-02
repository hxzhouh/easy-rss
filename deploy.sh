#!/bin/bash

# Easy-RSS 部署脚本
# 功能：更新代码 -> 编译 -> 停止旧进程 -> 启动新进程

set -e  # 遇到错误立即退出

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 配置
APP_NAME="easy-rss"
CONFIG_FILE="configs/config.yaml"
LOG_FILE="easy-rss.log"

echo -e "${YELLOW}========== Easy-RSS 部署脚本 ==========${NC}"

# 1. 更新代码
echo -e "${YELLOW}[1/5] 从 GitHub 拉取最新代码...${NC}"
git pull origin main
echo -e "${GREEN}✓ 代码更新完成${NC}"

# 2. 编译
echo -e "${YELLOW}[2/5] 编译项目...${NC}"
go build -o ${APP_NAME} cmd/server/main.go
echo -e "${GREEN}✓ 编译完成${NC}"

# 3. 停止旧进程
echo -e "${YELLOW}[3/5] 停止旧进程...${NC}"
PID=$(pgrep -f "./${APP_NAME}" || true)
if [ -n "$PID" ]; then
    echo "找到旧进程 PID: $PID，正在停止..."
    kill $PID 2>/dev/null || true
    
    # 等待进程完全停止
    for i in {1..10}; do
        if ! pgrep -f "./${APP_NAME}" > /dev/null; then
            break
        fi
        echo "等待进程停止... ($i/10)"
        sleep 1
    done
    
    # 如果还没停，强制 kill
    if pgrep -f "./${APP_NAME}" > /dev/null; then
        echo "强制停止进程..."
        pkill -9 -f "./${APP_NAME}" 2>/dev/null || true
    fi
    
    echo -e "${GREEN}✓ 旧进程已停止${NC}"
else
    echo -e "${YELLOW}ℹ 未找到运行中的进程${NC}"
fi

# 4. 备份旧日志（如果存在）
if [ -f "$LOG_FILE" ]; then
    echo -e "${YELLOW}[4/5] 备份旧日志...${NC}"
    mv ${LOG_FILE} ${LOG_FILE}.$(date +%Y%m%d_%H%M%S).bak
    echo -e "${GREEN}✓ 日志已备份${NC}"
else
    echo -e "${YELLOW}[4/5] 无需备份日志${NC}"
fi

# 5. 启动新进程
echo -e "${YELLOW}[5/5] 启动新进程...${NC}"
nohup ./${APP_NAME} --config ${CONFIG_FILE} > ${LOG_FILE} 2>&1 &

# 等待一下确认进程启动
sleep 2
NEW_PID=$(pgrep -f "./${APP_NAME}" || true)
if [ -n "$NEW_PID" ]; then
    echo -e "${GREEN}✓ 新进程已启动，PID: $NEW_PID${NC}"
    echo -e "${GREEN}✓ 日志文件: ${LOG_FILE}${NC}"
    echo ""
    echo -e "${GREEN}========== 部署成功 ==========${NC}"
    echo ""
    echo "常用命令:"
    echo "  查看日志: tail -f ${LOG_FILE}"
    echo "  查看进程: ps aux | grep ${APP_NAME}"
    echo "  停止服务: pkill -f \"./${APP_NAME}\""
else
    echo -e "${RED}✗ 进程启动失败，请检查日志${NC}"
    exit 1
fi
