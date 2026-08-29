#!/usr/bin/env bash

set -euo pipefail

repo_root="${1:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
failures=0
demo_asset='docs/assets/aikit-demo.gif'
demo_max_bytes=$((10 * 1024 * 1024))

fail() {
	printf 'docs check: %s\n' "$*" >&2
	failures=$((failures + 1))
}

require_file() {
	local relative_path="$1"
	if [[ ! -f "$repo_root/$relative_path" ]]; then
		fail "missing required file: $relative_path"
	fi
}

require_literal() {
	local relative_path="$1"
	local literal="$2"
	if [[ -f "$repo_root/$relative_path" ]] && ! grep -Fq -- "$literal" "$repo_root/$relative_path"; then
		fail "$relative_path is missing: $literal"
	fi
}

forbid_literal() {
	local relative_path="$1"
	local literal="$2"
	if [[ -f "$repo_root/$relative_path" ]] && grep -Fq -- "$literal" "$repo_root/$relative_path"; then
		fail "$relative_path must not contain: $literal"
	fi
}

require_heading_order() {
	local relative_path="$1"
	local first="$2"
	local second="$3"
	local third="$4"
	local first_line second_line third_line

	[[ -f "$repo_root/$relative_path" ]] || return
	first_line="$(awk -v heading="$first" '$0 == heading { print NR; exit }' "$repo_root/$relative_path")"
	second_line="$(awk -v heading="$second" '$0 == heading { print NR; exit }' "$repo_root/$relative_path")"
	third_line="$(awk -v heading="$third" '$0 == heading { print NR; exit }' "$repo_root/$relative_path")"
	[[ -n "$first_line" && -n "$second_line" && -n "$third_line" ]] || return
	if ! ((first_line < second_line && second_line < third_line)); then
		fail "$relative_path headings must appear in order: $first -> $second -> $third"
	fi
}

required_files=(
	README.md
	README.zh-CN.md
	CONTRIBUTING.md
	SECURITY.md
	CODE_OF_CONDUCT.md
	SUPPORT.md
	CHANGELOG.md
	LICENSE
	.github/ISSUE_TEMPLATE/bug_report.yml
	.github/ISSUE_TEMPLATE/feature_request.yml
	.github/ISSUE_TEMPLATE/documentation.yml
	.github/ISSUE_TEMPLATE/config.yml
	.github/pull_request_template.md
	"$demo_asset"
)

for relative_path in "${required_files[@]}"; do
	require_file "$relative_path"
done

require_literal README.md '[简体中文](README.zh-CN.md)'
require_literal README.zh-CN.md '[English](README.md)'
require_literal README.md "$demo_asset"
require_literal README.zh-CN.md "$demo_asset"
forbid_literal README.md '## Project status'
forbid_literal README.zh-CN.md '## 项目状态'

if [[ -f "$repo_root/$demo_asset" ]]; then
	demo_size="$(wc -c <"$repo_root/$demo_asset" | tr -d '[:space:]')"
	if ((demo_size == 0)); then
		fail "demo GIF is empty: $demo_asset"
	elif ((demo_size >= demo_max_bytes)); then
		fail "demo GIF must be smaller than 10 MiB: $demo_asset"
	fi
fi

homebrew_commands=(
	'brew tap silenceper/tap'
	'brew trust --formula silenceper/tap/aikit'
	'brew install silenceper/tap/aikit'
)
for command in "${homebrew_commands[@]}"; do
	require_literal README.md "$command"
	require_literal README.zh-CN.md "$command"
done

english_headings=(
	'## Preview'
	'## Why aikit'
	'## Features'
	'## Supported agents and platforms'
	'## Installation'
	'## Quick start'
	'## TUI'
	'## CLI'
	'## Configuration and data'
	'## Safety and recovery'
	'## Troubleshooting'
	'## Contributing and support'
	'## License'
)

chinese_headings=(
	'## 界面预览'
	'## 为什么选择 aikit'
	'## 核心能力'
	'## 支持的 Agent 与平台'
	'## 安装'
	'## 快速开始'
	'## TUI'
	'## CLI'
	'## 配置与数据'
	'## 安全与恢复'
	'## 故障排查'
	'## 贡献与支持'
	'## 许可证'
)

for heading in "${english_headings[@]}"; do
	require_literal README.md "$heading"
done
for heading in "${chinese_headings[@]}"; do
	require_literal README.zh-CN.md "$heading"
done

require_heading_order README.md '## Preview' '## Why aikit' '## Features'
require_heading_order README.zh-CN.md '## 界面预览' '## 为什么选择 aikit' '## 核心能力'

check_relative_links() {
	local relative_path="$1"
	local source_file="$repo_root/$relative_path"
	local match target resolved

	[[ -f "$source_file" ]] || return
	while IFS= read -r match; do
		target="${match#](}"
		target="${target%)}"
		target="${target%%#*}"
		target="${target#<}"
		target="${target%>}"
		case "$target" in
			''|'#'*|http://*|https://*|mailto:*|tel:*) continue ;;
		esac
		resolved="$(dirname "$source_file")/$target"
		if [[ ! -e "$resolved" ]]; then
			fail "$relative_path has a broken relative link: $target"
		fi
	done < <(grep -oE '\]\([^)]*\)' "$source_file" || true)
}

for relative_path in README.md README.zh-CN.md CONTRIBUTING.md SECURITY.md CODE_OF_CONDUCT.md SUPPORT.md CHANGELOG.md .github/pull_request_template.md; do
	check_relative_links "$relative_path"
done

if ((failures > 0)); then
	printf 'docs check: failed with %d issue(s)\n' "$failures" >&2
	exit 1
fi

printf 'docs check: PASS\n'
