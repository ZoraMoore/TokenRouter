#!/usr/bin/env bash
set -euo pipefail

repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
fixture=$(mktemp -d)
trap 'rm -rf -- "$fixture"' EXIT
mkdir -- "$fixture/source" "$fixture/target"
printf '%s\n' 'current entry' > "$fixture/source/index-js-test.js"
printf '%s\n' 'current styles' > "$fixture/source/styles-test.css"
printf '%s\n' 'previous entry' > "$fixture/target/previous-js-test.js"

for attempt in first second; do
  bash "$repo/deploy/publish-ui-assets.sh" "$fixture/source" "$fixture/target"
  cmp "$fixture/source/index-js-test.js" "$fixture/target/index-js-test.js"
  cmp "$fixture/source/styles-test.css" "$fixture/target/styles-test.css"
  [[ $(cat "$fixture/target/previous-js-test.js") == 'previous entry' ]]
done

printf '%s\n' 'conflicting entry' > "$fixture/source/index-js-test.js"
printf '%s\n' 'another entry' > "$fixture/source/another-js-test.js"
if bash "$repo/deploy/publish-ui-assets.sh" "$fixture/source" "$fixture/target" > "$fixture/output" 2>&1; then
  echo '文件名冲突时应拒绝发布' >&2
  exit 1
fi
grep -q 'index-js-test.js' "$fixture/output"
[[ $(cat "$fixture/target/index-js-test.js") == 'current entry' ]]
[[ ! -e "$fixture/target/another-js-test.js" ]]

mkdir -- "$fixture/empty"
if bash "$repo/deploy/publish-ui-assets.sh" "$fixture/empty" "$fixture/target" > "$fixture/output" 2>&1; then
  echo '空构建目录应拒绝发布' >&2
  exit 1
fi

echo '静态资源发布测试通过'
