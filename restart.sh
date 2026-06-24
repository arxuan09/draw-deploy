#!/usr/bin/env bash
# drawnext 重启（在部署目录内运行）
set -euo pipefail

[ -f docker-compose.yml ] || { echo "请在部署目录内运行（找不到 docker-compose.yml）"; exit 1; }

docker compose restart
docker compose ps
echo "重启完成。"
