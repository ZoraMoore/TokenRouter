#!/usr/bin/env bash
set -euo pipefail

TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${TEST_DIR}/../install.sh"
TEST_ROOT="$(mktemp -d)"
trap 'rm -rf "$TEST_ROOT"' EXIT

# 检测只操作临时根目录，不调用 systemctl 或修改宿主机安装。
detect_existing_installation "$TEST_ROOT"
[[ "$SERVICE_NAME" == tokenrouter && "$INSTALL_DIR" == "$TEST_ROOT/opt/tokenrouter" ]]
mkdir -p "$TEST_ROOT/opt/sub2api" "$TEST_ROOT/etc/systemd/system"
printf 'User=legacy-service-user\n' > "$TEST_ROOT/etc/systemd/system/sub2api.service"
detect_existing_installation "$TEST_ROOT"
[[ "$SERVICE_NAME" == sub2api && "$BINARY_NAME" == sub2api ]]
[[ "$SERVICE_USER" == legacy-service-user && "$CONFIG_DIR" == "$TEST_ROOT/etc/sub2api" ]]
mkdir -p "$TEST_ROOT/opt/tokenrouter"
if detect_existing_installation "$TEST_ROOT" >/dev/null 2>&1; then
    echo 'Installer accepted conflicting installations' >&2
    exit 1
fi
# 使用本地归档和下载替身验证新安装器读取旧版本，以及旧安装位置接收新版本。
rmdir "$TEST_ROOT/opt/tokenrouter"
detect_existing_installation "$TEST_ROOT"
FIXTURE_DIR="$TEST_ROOT/releases"
mkdir -p "$FIXTURE_DIR/old" "$FIXTURE_DIR/new"
printf 'legacy-binary' > "$FIXTURE_DIR/old/sub2api"
printf 'new-binary' > "$FIXTURE_DIR/new/tokenrouter"
printf 'new-binary' > "$FIXTURE_DIR/new/sub2api"
OS=linux
ARCH=amd64
LATEST_VERSION=v1.2.3
tar -czf "$FIXTURE_DIR/sub2api_1.2.3_linux_amd64.tar.gz" -C "$FIXTURE_DIR/old" sub2api
tar -czf "$FIXTURE_DIR/tokenrouter_1.2.3_linux_amd64.tar.gz" -C "$FIXTURE_DIR/new" tokenrouter sub2api
(cd "$FIXTURE_DIR" && sha256sum ./*.tar.gz | sed 's|  ./|  |' > checksums.txt)
LEGACY_ONLY=true
curl() {
    local url="" output=""
    while [[ $# -gt 0 ]]; do
        case "$1" in
            https://*) url="$1" ;;
            -o) shift; output="$1" ;;
        esac
        shift
    done
    local name="${url##*/}"
    if [[ "$LEGACY_ONLY" == true && "$name" == tokenrouter_* ]]; then return 1; fi
    cp "$FIXTURE_DIR/$name" "$output"
}
download_and_extract
[[ "$(cat "$INSTALL_DIR/$BINARY_NAME")" == legacy-binary ]]
[[ "$INSTALL_DIR" == "$TEST_ROOT/opt/sub2api" ]]
rm -rf "$TEMP_DIR"
LEGACY_ONLY=false
download_and_extract
[[ "$(cat "$INSTALL_DIR/$BINARY_NAME")" == new-binary ]]
[[ "$BINARY_NAME" == sub2api ]]
# 默认补充已内嵌；安装器不创建外部资源，也不覆盖部署者留下的文件。
PRICING_RESOURCE="resources/model-pricing/model_pricing_supplements.json"
[[ ! -e "$INSTALL_DIR/$PRICING_RESOURCE" ]]
mkdir -p "$INSTALL_DIR/resources/model-pricing"
printf 'custom-pricing' > "$INSTALL_DIR/$PRICING_RESOURCE"
rm -rf "$TEMP_DIR"
download_and_extract
[[ "$(cat "$INSTALL_DIR/$PRICING_RESOURCE")" == custom-pricing ]]
# 被测下载函数会注册自己的退出清理，因此在断言之后恢复测试清理。
trap 'rm -rf "$TEST_ROOT" "$TEMP_DIR"' EXIT
echo 'Installer brand compatibility tests passed.'
