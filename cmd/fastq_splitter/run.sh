#!/bin/bash
# run.sh

# 设置输出目录
OUTPUT_DIR="./split_results"

# 检查输入文件
if [ $# -lt 1 ]; then
    echo "用法: $0 <Excel文件> [输出目录]"
    echo "示例: $0 samples.xlsx $OUTPUT_DIR"
    exit 1
fi

EXCEL_FILE=$1

if [ $# -ge 2 ]; then
    OUTPUT_DIR=$2
fi

# 检查Excel文件是否存在
if [ ! -f "$EXCEL_FILE" ]; then
    echo "错误: Excel文件不存在: $EXCEL_FILE"
    exit 1
fi

# 检查fastp是否安装
if ! command -v fastp &> /dev/null; then
    echo "错误: fastp未安装，请先运行 install_deps.sh"
    exit 1
fi

# 创建输出目录
mkdir -p "$OUTPUT_DIR"

# 运行程序
echo "开始处理Excel文件: $EXCEL_FILE"
echo "输出目录: $OUTPUT_DIR"
echo "========================================"

go run main.go "$EXCEL_FILE" "$OUTPUT_DIR"

echo "========================================"
echo "处理完成！结果保存在: $OUTPUT_DIR"