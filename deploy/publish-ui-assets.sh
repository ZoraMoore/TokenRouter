#!/usr/bin/env bash
set -euo pipefail

if [[ $# != 2 || ! -d "$1" || ! -d "$2" ]]; then
  echo "用法 $0 构建资源目录 发布资源目录" >&2
  exit 2
fi

source_dir=$(realpath -- "$1")
target_dir=$(realpath -- "$2")
if [[ "$source_dir" == "$target_dir" ]]; then
  echo '构建目录和发布目录需要分开' >&2
  exit 2
fi

# 锁定发布目录，串行检查同名资源和写入文件。
exec 9<"$target_dir"
flock -x 9

manifest=$(mktemp)
trap 'rm -f -- "$manifest"' EXIT
(
  cd "$source_dir"
  find . -type f -print0 | LC_ALL=C sort -z | xargs -0 -r sha256sum
) > "$manifest"
if [[ ! -s "$manifest" ]]; then
  echo '构建目录没有可发布的文件' >&2
  exit 1
fi

# 逐个核对源路径及目标路径，避免资源进入站点目录外。
while IFS= read -r -d '' path; do
  relative=${path#"$source_dir"/}
  target="$target_dir/$relative"
  if [[ -L "$path" || -L "$target" ]]; then
    printf '资源路径包含符号链接 %s\n' "$relative" >&2
    exit 1
  fi
  if [[ -d "$path" ]]; then
    if [[ ! -e "$target" || -d "$target" ]]; then
      continue
    fi
  elif [[ -f "$path" ]] && { [[ ! -e "$target" ]] || { [[ -f "$target" ]] && cmp -s -- "$path" "$target"; }; }; then
    continue
  fi
  printf '资源路径冲突 %s\n' "$relative" >&2
  exit 1
done < <(find "$source_dir" -mindepth 1 -print0)

rsync -r --ignore-existing --omit-dir-times --chmod=Du=rwx,Dgo=rx,Fu=rw,Fgo=r "$source_dir/" "$target_dir/"
(cd "$target_dir" && sha256sum -c --quiet "$manifest")
printf '已校验 %s 个静态资源\n' "$(wc -l < "$manifest")"
