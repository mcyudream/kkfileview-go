#!/usr/bin/env bash
# 构建并推送 kkfileview-go 镜像到私有仓库
#
# 本机网络无法访问 Docker Hub，因此：
#   1) 构建阶段改为「本地交叉编译 linux 二进制」，不依赖 golang 基础镜像；
#   2) 运行基础镜像改用内网 registry 里的 registry.yudream.online/library/kkfileview:5.0.2
#      （Ubuntu 24.04，已含 LibreOffice 与文泉驿中文字体），再用 apt 补 poppler/7z/ffmpeg/qpdf；
#   3) 仓库前置 nginx 的 client_max_body_size 约 1MB，docker push 的单体 PUT 会被 413 拒绝，
#      故推送走分块 PATCH 工具 tools/pushimg（scripts/../tools/pushimg）。
set -euo pipefail

cd "$(dirname "$0")/.."

REGISTRY=registry.yudream.online
REPO=library/kkfileview-go
BASE=registry.yudream.online/library/kkfileview:5.0.2
TAG="${1:-5.0.3}"
PLATFORM=linux/amd64
PUSHIMG="${PUSHIMG:-../tools/pushimg/pushimg}"

echo "==> 1/4 交叉编译 linux/amd64 二进制"
mkdir -p dist
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
    go build -trimpath -ldflags="-s -w" -o dist/kkfileview-linux .

echo "==> 2/4 构建镜像 ${REGISTRY}/${REPO}:${TAG}"
docker build --platform "${PLATFORM}" \
    --build-arg BASE_IMAGE="${BASE}" \
    -f Dockerfile.registry \
    -t "${REGISTRY}/${REPO}:${TAG}" .

echo "==> 3/4 导出并分块推送 ${TAG}"
rm -rf imgtar/oci && mkdir -p imgtar/oci
docker save "${REGISTRY}/${REPO}:${TAG}" -o imgtar/image.tar
tar -xf imgtar/image.tar -C imgtar/oci
REG_USER="${REG_USER:-robot\$push}" REG_PASS="${REG_PASS:?请设置 REG_PASS（仓库 robot 账号密码）}" \
    "${PUSHIMG}" -dir imgtar/oci -repo "${REPO}" -tag "${TAG}"

echo "==> 4/4 追加 latest 标签"
REG_USER="${REG_USER:-robot\$push}" REG_PASS="${REG_PASS}" \
    "${PUSHIMG}" -dir imgtar/oci -repo "${REPO}" -tag latest

rm -rf imgtar dist
echo "完成: docker pull ${REGISTRY}/${REPO}:${TAG}"
