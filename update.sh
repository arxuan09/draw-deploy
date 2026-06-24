#!/usr/bin/env bash
# drawnext 更新：拉取最新镜像并重建 drawnext 容器（在部署目录内运行）
set -euo pipefail

[ -f docker-compose.yml ] || { echo "请在部署目录内运行（找不到 docker-compose.yml）"; exit 1; }

docker compose pull drawnext
docker compose up -d drawnext
docker compose ps
echo "更新完成。"
