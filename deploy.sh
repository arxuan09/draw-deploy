#!/usr/bin/env bash
# ============================================================================
# drawnext · 一键部署脚本
#
# 它会：
#   1. 检查依赖（docker / docker compose / curl）
#   2. 创建一个全新的部署目录（默认 drawnext；目录已存在则拒绝，保护既有数据）
#   3. 下载 docker-compose.yml / .env 模板 / update.sh / restart.sh 到该目录
#   4. 自动生成 JWT 密钥与后台加密密钥，持久化到 .env
#   5. 提示输入软件授权码，持久化到 .env
#   6. 拉取镜像并启动
#
# 用法：
#   bash deploy.sh [部署目录名]        # 默认目录名 drawnext
# ============================================================================
set -euo pipefail

# ===== 部署文件来源（如仓库/分支不同请改这里）=====
REPO="arxuan09/draw-deploy"
BRANCH="main"
RAW_BASE="https://raw.githubusercontent.com/${REPO}/${BRANCH}"

# ===== 部署目录 =====
DEPLOY_DIR="${1:-drawnext}"

# ---- 中文输出辅助 ----
red(){ printf '\033[31m%s\033[0m\n' "$*"; }
grn(){ printf '\033[32m%s\033[0m\n' "$*"; }
ylw(){ printf '\033[33m%s\033[0m\n' "$*"; }
info(){ printf '➜ %s\n' "$*"; }

# ---- 把 KEY=VALUE 写入 .env：存在则替换该行，不存在则追加。
#      值通过 awk -v 传入，避免 base64 中的 / + = 触发转义问题。----
set_env(){
  local key="$1" val="$2" file=".env" tmp
  tmp="$(mktemp)"
  awk -v k="$key" -v v="$val" '
    index($0, k"=")==1 && !done { print k"="v; done=1; next }
    { print }
    END { if (!done) print k"="v }
  ' "$file" > "$tmp" && mv "$tmp" "$file"
}

# ---- 生成随机密钥（优先 openssl，回退 /dev/urandom）----
gen_hex(){ openssl rand -hex 32 2>/dev/null || head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'; }
gen_b64(){ openssl rand -base64 32 2>/dev/null || head -c 32 /dev/urandom | base64 | tr -d '\n'; }

echo
grn "==================  drawnext 一键部署  =================="
echo

# 1. 依赖检查
info "检查依赖（docker / docker compose / curl）..."
command -v docker  >/dev/null 2>&1 || { red "未检测到 docker，请先安装 Docker 后重试。"; exit 1; }
docker compose version >/dev/null 2>&1 || { red "未检测到 Docker Compose v2（docker compose），请先安装/升级 Docker。"; exit 1; }
command -v curl    >/dev/null 2>&1 || { red "未检测到 curl，请先安装 curl 后重试。"; exit 1; }

# 2. 目录保护：已存在则拒绝，避免覆盖既有部署及其密钥
if [ -e "$DEPLOY_DIR" ]; then
  red  "部署目录 “$DEPLOY_DIR” 已存在！"
  ylw  "为保护既有部署的数据与密钥（重新生成密钥会导致后台已存配置无法解密），"
  ylw  "请先【删除或改名/移动】该目录，再重新运行本脚本。例如："
  echo "    mv $DEPLOY_DIR ${DEPLOY_DIR}.bak"
  echo "  或自定义新目录名： bash deploy.sh 新目录名"
  exit 1
fi

# 3. 创建并进入部署目录
info "创建部署目录： $DEPLOY_DIR"
mkdir -p "$DEPLOY_DIR"
cd "$DEPLOY_DIR"

# 4. 下载配置文件
info "下载 docker-compose.yml / .env 模板 / update.sh / restart.sh ..."
curl -fsSL "$RAW_BASE/docker-compose.yml" -o docker-compose.yml || { red "下载 docker-compose.yml 失败，请检查网络或仓库地址（REPO/BRANCH）。"; exit 1; }
curl -fsSL "$RAW_BASE/.env.example"        -o .env.example       || { red "下载 .env.example 失败。"; exit 1; }
curl -fsSL "$RAW_BASE/update.sh"           -o update.sh          || { red "下载 update.sh 失败。"; exit 1; }
curl -fsSL "$RAW_BASE/restart.sh"          -o restart.sh         || { red "下载 restart.sh 失败。"; exit 1; }
chmod +x update.sh restart.sh
cp .env.example .env

# 5. 生成并持久化密钥
info "生成 JWT 密钥与后台加密密钥，写入 .env ..."
set_env JWT_SECRET "$(gen_hex)"
set_env SETTINGS_ENCRYPTION_KEY "$(gen_b64)"
grn "  ✓ 已生成 JWT_SECRET 与 SETTINGS_ENCRYPTION_KEY"
ylw "  请妥善保管本目录下的 .env；其中 SETTINGS_ENCRYPTION_KEY 一旦更改，后台已存配置将无法解密。"

# 6. 提示输入授权码并持久化
echo
ylw "请输入【软件授权码 LICENSE_KEY】（向供应商获取；直接回车可留空、稍后再填）："
LICENSE=""
read -r LICENSE < /dev/tty 2>/dev/null || LICENSE=""
set_env LICENSE_KEY "$LICENSE"
if [ -z "$LICENSE" ]; then
  ylw "  未填写授权码：服务可启动，但所有功能会被拦截。填好后改 .env 再运行 ./update.sh 即可生效。"
else
  grn "  ✓ 已写入授权码"
fi

# 7. 拉取镜像并启动
echo
info "拉取镜像并启动（首次较慢，请耐心等待）..."
docker compose pull
docker compose up -d

# 8. 收尾提示
PORT="$(awk -F= '/^APP_PORT=/{print $2}' .env)"; PORT="${PORT:-16789}"
echo
grn "====================  部署完成  ===================="
echo "  • 访问地址：   http://<服务器公网IP>:${PORT}"
echo "  • 部署目录：   $(pwd)"
echo "  • 查看日志：   cd $(pwd) && docker compose logs -f drawnext"
echo "  • 首个管理员： 见启动日志（默认 admin@example.com / Admin123456!）"
echo "  • 安全建议：   登录后立即改密码，并到后台「用户管理」把管理员账号(邮箱/用户名)也改成你自己的"
echo "  • 下一步：     登录后台 → 系统设置，配置【对象存储】与【AI 供应商/模型】后即可出图"
echo "  • 升级：       在本目录执行  ./update.sh"
echo "  • 重启：       在本目录执行  ./restart.sh"
echo
ylw "建议用 mysqldump 定期备份（见 README「数据与备份」）；删除部署目录会一并删数据，请谨慎。"
echo
