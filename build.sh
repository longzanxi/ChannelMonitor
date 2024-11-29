#!/bin/bash

# 版本信息
VERSION=$(cat VERSION)
COMMIT_ID=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(date "+%F %T")

# 创建输出目录
mkdir -p build

# 编译参数
LDFLAGS="-w -s -X 'main.Version=${VERSION}' -X 'main.CommitID=${COMMIT_ID}' -X 'main.BuildTime=${BUILD_TIME}'"

# 目标平台列表
PLATFORMS=(
    "darwin/amd64"
    "darwin/arm64"
    "linux/amd64"
    "linux/arm64"
    "windows/amd64"
    "windows/arm64"
)

# 安装 upx
if ! command -v upx >/dev/null 2>&1; then
    echo "Installing upx..."
    sudo apt-get update
    sudo apt-get install -y upx
fi

# 编译函数
build() {
    local GOOS=$1
    local GOARCH=$2
    
    # 设置输出文件名
    local OUTPUT="build/${BINARY_NAME}_${GOOS}_${GOARCH}"
    if [ "$GOOS" == "windows" ]; then
        OUTPUT="${OUTPUT}.exe"
    fi
    
    echo "Building for ${GOOS}/${GOARCH}..."
    CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build -trimpath -ldflags "${LDFLAGS}" -o "${OUTPUT}" .
    
    if [ $? -eq 0 ]; then
        echo "✅ Finished building for ${GOOS}/${GOARCH}"
        # 压缩二进制文件
        upx --best --lzma "${OUTPUT}"
    else
        echo "❌ Failed building for ${GOOS}/${GOARCH}"
    fi
}

# 获取二进制名称（默认为当前目录名）
BINARY_NAME=$(basename $(pwd))

# 开始编译
echo "🚀 Starting build process..."
echo "Binary name: ${BINARY_NAME}"
echo "Version: ${VERSION}"
echo "Commit ID: ${COMMIT_ID}"
echo "Build Time: ${BUILD_TIME}"

# 并行编译所有平台
echo "${PLATFORMS[@]}" | xargs -n 1 -P 4 -I {} bash -c 'GOOS=${1%/*} GOARCH=${1#*/} build $GOOS $GOARCH' _ {}

echo "✨ Build process completed!"

# 清理后台进程
trap 'kill $(jobs -p)' EXIT