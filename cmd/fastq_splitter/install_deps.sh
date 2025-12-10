#!/bin/bash
# install_deps.sh

echo "安装Go依赖包..."
go mod download

echo "检查fastp是否安装..."
if ! command -v fastp &> /dev/null; then
    echo "fastp未安装，正在安装fastp..."
    
    # 下载fastp
    wget http://opengene.org/fastp/fastp.0.23.2
    chmod a+x fastp.0.23.2
    sudo mv fastp.0.23.2 /usr/local/bin/fastp
    
    echo "fastp安装完成"
else
    echo "fastp已安装"
fi

echo "所有依赖安装完成！"