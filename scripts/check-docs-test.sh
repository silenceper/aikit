#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
checker="$script_dir/check-docs.sh"
test_root="$(mktemp -d "${TMPDIR:-/tmp}/aikit-docs-test.XXXXXX")"
trap 'rm -rf "$test_root"' EXIT

write_fixture() {
	local root="$1"
	mkdir -p "$root/.github/ISSUE_TEMPLATE"

	printf '%s\n' \
		'[简体中文](README.zh-CN.md)' \
		'# Fixture' \
		'## Why aikit' \
		'## Project status' \
		'## Features' \
		'## Supported agents and platforms' \
		'## Installation' \
		'## Quick start' \
		'## TUI' \
		'## CLI' \
		'## Configuration and data' \
		'## Safety and recovery' \
		'## Troubleshooting' \
		'## Contributing and support' \
		'## License' \
		'[License](LICENSE)' >"$root/README.md"

	printf '%s\n' \
		'[English](README.md)' \
		'# 测试' \
		'## 为什么选择 aikit' \
		'## 项目状态' \
		'## 核心能力' \
		'## 支持的 Agent 与平台' \
		'## 安装' \
		'## 快速开始' \
		'## TUI' \
		'## CLI' \
		'## 配置与数据' \
		'## 安全与恢复' \
		'## 故障排查' \
		'## 贡献与支持' \
		'## 许可证' \
		'[许可证](LICENSE)' >"$root/README.zh-CN.md"

	for relative_path in CONTRIBUTING.md SECURITY.md CODE_OF_CONDUCT.md SUPPORT.md CHANGELOG.md LICENSE .github/ISSUE_TEMPLATE/bug_report.yml .github/ISSUE_TEMPLATE/feature_request.yml .github/ISSUE_TEMPLATE/documentation.yml .github/ISSUE_TEMPLATE/config.yml .github/pull_request_template.md; do
		: >"$root/$relative_path"
	done
}

expect_failure() {
	local name="$1"
	local expected="$2"
	local fixture="$test_root/$name"
	local output="$test_root/$name.output"

	cp -R "$test_root/good" "$fixture"
	case "$name" in
		broken-link) printf '\n[Broken](missing.md)\n' >>"$fixture/README.md" ;;
		brew-command) printf '\n```bash\nbrew install aikit\n```\n' >>"$fixture/README.md" ;;
		brew-command-support) printf '\n```bash\n$ brew install aikit\n```\n' >>"$fixture/SUPPORT.md" ;;
		missing-language-link) grep -Fv '[English](README.md)' "$fixture/README.zh-CN.md" >"$fixture/README.zh-CN.md.next" && mv "$fixture/README.zh-CN.md.next" "$fixture/README.zh-CN.md" ;;
		missing-english-heading) grep -Fvx '## Features' "$fixture/README.md" >"$fixture/README.md.next" && mv "$fixture/README.md.next" "$fixture/README.md" ;;
		missing-chinese-heading) grep -Fvx '## 核心能力' "$fixture/README.zh-CN.md" >"$fixture/README.zh-CN.md.next" && mv "$fixture/README.zh-CN.md.next" "$fixture/README.zh-CN.md" ;;
		*) printf 'unknown fixture: %s\n' "$name" >&2; exit 1 ;;
	esac

	if bash "$checker" "$fixture" >"$output" 2>&1; then
		printf 'docs checker self-test: %s unexpectedly passed\n' "$name" >&2
		exit 1
	fi
	if ! grep -Fq -- "$expected" "$output"; then
		printf 'docs checker self-test: %s failed for the wrong reason\n' "$name" >&2
		cat "$output" >&2
		exit 1
	fi
}

write_fixture "$test_root/good"
bash "$checker" "$test_root/good" >/dev/null

expect_failure broken-link 'broken relative link: missing.md'
expect_failure brew-command 'advertises an unverified Homebrew command'
expect_failure brew-command-support 'SUPPORT.md advertises an unverified Homebrew command'
expect_failure missing-language-link 'README.zh-CN.md is missing: [English](README.md)'
expect_failure missing-english-heading 'README.md is missing: ## Features'
expect_failure missing-chinese-heading 'README.zh-CN.md is missing: ## 核心能力'

printf 'docs checker self-test: PASS\n'
