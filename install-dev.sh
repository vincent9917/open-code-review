#!/usr/bin/env bash
#
# install-dev.sh — 从本地源码构建并安装 OpenCodeReview（开发模式）
#
# 与 install.sh（从 GitHub Release 下载）不同，本脚本：
#   1. 从本地 Go 源码编译 ocr 二进制
#   2. 安装到 /usr/local/bin（可通过 OCR_INSTALL_DIR 覆盖）
#   3. 将 skills/ 目录下的 Skill 安装到 ~/.agents/skills/
#   4. 软链接到 ~/.claude/skills/
#
# 使用：
#   chmod +x install-dev.sh
#   ./install-dev.sh
#
# 自定义路径：
#   OCR_INSTALL_DIR="$HOME/.local/bin" ./install-dev.sh
#   SKILLS_BASE_DIR="$HOME/.custom-skills" ./install-dev.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OCR_INSTALL_DIR="${OCR_INSTALL_DIR:-/usr/local/bin}"
BINARY_NAME="opencodereview"
CLI_NAME="ocr"

SKILLS_BASE_DIR="${SKILLS_BASE_DIR:-$HOME/.agents/skills}"
CLAUDE_SKILLS_DIR="${CLAUDE_SKILLS_DIR:-$HOME/.claude/skills}"

# 颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()  { printf "${BLUE}[INFO]${NC}  %s\n" "$*"; }
ok()    { printf "${GREEN}[OK]${NC}    %s\n" "$*"; }
warn()  { printf "${YELLOW}[WARN]${NC}  %s\n" "$*"; }
err()   { printf "${RED}[ERROR]${NC} %s\n" "$*" >&2; }

# ══════════════════════════════════════════════════════════════════════════════
# 步骤 1：编译
# ══════════════════════════════════════════════════════════════════════════════
info "步骤 1/3: 编译源码 (make build) ..."
cd "$SCRIPT_DIR"

if ! command -v go &>/dev/null; then
    err "未找到 Go 编译器，请先安装 Go"
    exit 1
fi

make build
ok "编译完成 → dist/${BINARY_NAME}"

# ══════════════════════════════════════════════════════════════════════════════
# 步骤 2：安装二进制
# ══════════════════════════════════════════════════════════════════════════════
info "步骤 2/3: 安装二进制到 ${OCR_INSTALL_DIR} ..."

mkdir -p "$OCR_INSTALL_DIR"

# 检测 npm 安装的旧版本
if command -v "$CLI_NAME" &>/dev/null; then
    EXISTING_PATH="$(command -v "$CLI_NAME")"
    if [[ "$EXISTING_PATH" == *"node"* || "$EXISTING_PATH" == *"npm"* || "$EXISTING_PATH" == *"nvm"* ]]; then
        warn "检测到 npm 安装的 ocr: ${EXISTING_PATH}"
        warn "如需清理，运行: npm uninstall -g @alibaba-group/open-code-review"
    fi
fi

cp "dist/${BINARY_NAME}" "${OCR_INSTALL_DIR}/${CLI_NAME}"
chmod +x "${OCR_INSTALL_DIR}/${CLI_NAME}"
ok "二进制已安装: ${OCR_INSTALL_DIR}/${CLI_NAME}"

if command -v "$CLI_NAME" &>/dev/null; then
    INSTALLED_VERSION="$("$CLI_NAME" version 2>/dev/null || echo "unknown")"
    ok "版本: ${INSTALLED_VERSION}"
else
    warn "${OCR_INSTALL_DIR} 不在 PATH 中，请手动添加"
fi

# ══════════════════════════════════════════════════════════════════════════════
# 步骤 3：安装 Skills
# ══════════════════════════════════════════════════════════════════════════════
info "步骤 3/3: 安装 Skills ..."

mkdir -p "$SKILLS_BASE_DIR"
mkdir -p "$CLAUDE_SKILLS_DIR"

SKILL_COUNT=0

for skill_dir in "$SCRIPT_DIR"/skills/*/; do
    [[ -d "$skill_dir" ]] || continue

    skill_name="$(basename "$skill_dir")"

    if [[ ! -f "${skill_dir}/SKILL.md" ]]; then
        warn "SKILL.md 不存在，跳过: ${skill_dir}"
        continue
    fi

    # 复制到 ~/.agents/skills/<skill_name>
    skill_dst="${SKILLS_BASE_DIR}/${skill_name}"
    rm -rf "$skill_dst"
    cp -r "$skill_dir" "$skill_dst"
    ok "Skill 已安装: ${skill_dst}"

    # 软链接到 ~/.claude/skills/<skill_name>
    link_path="${CLAUDE_SKILLS_DIR}/${skill_name}"
    if [[ -L "$link_path" ]] || [[ -e "$link_path" ]]; then
        rm -rf "$link_path"
    fi
    ln -sf "$skill_dst" "$link_path"
    ok "软链接已创建: ${link_path} -> ${skill_dst}"

    ((SKILL_COUNT++)) || true
done

if [[ $SKILL_COUNT -eq 0 ]]; then
    warn "未发现任何 Skill 目录"
else
    ok "共安装 ${SKILL_COUNT} 个 Skill"
fi

# ══════════════════════════════════════════════════════════════════════════════
echo ""
info "安装完成！"
echo ""
echo "  二进制路径:  ${OCR_INSTALL_DIR}/${CLI_NAME}"
echo "  Skills 路径: ${SKILLS_BASE_DIR}/"
echo "  Claude 链接: ${CLAUDE_SKILLS_DIR}/"
echo ""
echo "  验证:  ocr version"
echo "  审查:  ocr review"
