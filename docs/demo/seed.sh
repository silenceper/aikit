#!/usr/bin/env bash

set -euo pipefail

readonly marker_name=".aikit-readme-demo"
readonly marker_value="aikit-readme-demo:v1"

die() {
	printf 'demo seed: %s\n' "$*" >&2
	exit 1
}

usage() {
	printf 'usage: %s <prepare|verify|verify-result|clean> <demo-root> <protected-home>\n' "$0" >&2
	exit 2
}

validate_root_argument() {
	local root="$1"
	local protected_home="$2"
	local root_name="${root##*/}"
	local root_parent="${root%/*}"

	[[ -n "$root" ]] || die "demo root must not be empty"
	[[ "$root" == /* ]] || die "demo root must be absolute: $root"
	[[ "$root" != "/" ]] || die "refusing to use filesystem root"
	[[ -n "$protected_home" ]] || die "protected home must not be empty"
	[[ "$protected_home" == /* ]] || die "protected home must be absolute: $protected_home"
	[[ "$root" != "$protected_home" ]] || die "demo root must not be the protected home"
	[[ "$root_parent" == /tmp ]] || die "demo root must be an immediate child of /tmp"
	[[ "$root_name" == aikit-readme-demo || "$root_name" == aikit-readme-demo.* ]] ||
		die "demo root must be /tmp/aikit-readme-demo or its test variant"
}

require_owned_root() {
	local root="$1"

	[[ -d "$root" ]] || die "demo root is missing: $root"
	[[ ! -L "$root" ]] || die "demo root must not be a symlink: $root"
	[[ -f "$root/$marker_name" ]] || die "ownership marker is missing: $root/$marker_name"
	[[ ! -L "$root/$marker_name" ]] || die "ownership marker must not be a symlink"
	[[ "$(<"$root/$marker_name")" == "$marker_value" ]] || die "ownership marker is invalid"
}

write_skill() {
	local path="$1"
	local name="$2"
	local description="$3"
	local body="$4"

	mkdir -p "$path"
	printf '%s\n' \
		'---' \
		"name: $name" \
		"description: $description" \
		'---' \
		'' \
		"# $name" \
		'' \
		"$body" >"$path/SKILL.md"
}

prepare() {
	local root="$1"

	if [[ -e "$root" || -L "$root" ]]; then
		die "demo root already exists; clean it explicitly before recording: $root"
	fi
	mkdir "$root"
	printf '%s\n' "$marker_value" >"$root/$marker_name"

	write_skill \
		"$root/home/.codex/skills/code-review" \
		"code-review" \
		"Review code for correctness, security, and maintainability." \
		"Check behavior, edge cases, and tests before suggesting focused improvements."
	write_skill \
		"$root/home/.claude/skills/release-checklist" \
		"release-checklist" \
		"Prepare reliable releases with a concise, repeatable checklist." \
		"Verify tests, changelog, version, artifacts, and rollback notes before publishing."
	write_skill \
		"$root/home/.cursor/skills/api-design" \
		"api-design" \
		"Design clear APIs with stable contracts and useful failure modes." \
		"Review naming, inputs, outputs, compatibility, validation, and error responses."

	mkdir -p \
		"$root/aikit" \
		"$root/bin" \
		"$root/cache/go-build" \
		"$root/cache/go-mod" \
		"$root/cache/gopath" \
		"$root/cache/xdg"
}

verify_seed() {
	local root="$1"
	local skill_file

	for skill_file in \
		"$root/home/.codex/skills/code-review/SKILL.md" \
		"$root/home/.claude/skills/release-checklist/SKILL.md" \
		"$root/home/.cursor/skills/api-design/SKILL.md"; do
		[[ -s "$skill_file" ]] || die "seeded Skill is missing or empty: $skill_file"
		grep -Fq -- 'description:' "$skill_file" || die "Skill description is missing: $skill_file"
	done
}

verify_result() {
	local root="$1"
	local aikit_bin="$root/bin/aikit"
	local all_skills codex_skills config_file
	local source_dir release_library

	require_owned_root "$root"
	[[ -x "$aikit_bin" ]] || die "demo binary is missing: $aikit_bin"
	config_file="$root/aikit/config.yaml"
	[[ -s "$config_file" ]] || die "aikit config was not created"

	all_skills="$(HOME="$root/home" AIKIT_HOME="$root/aikit" "$aikit_bin" --json list --offline)"
	[[ "$(grep -Fc '"id": "' <<<"$all_skills")" -eq 3 ]] || die "expected exactly three Library Skills"
	for skill_id in local/code-review local/release-checklist local/api-design; do
		[[ "$(grep -Fc "\"id\": \"$skill_id\"" <<<"$all_skills")" -eq 1 ]] ||
			die "expected one Library entry for $skill_id"
	done

	codex_skills="$(HOME="$root/home" AIKIT_HOME="$root/aikit" "$aikit_bin" --json list --offline --agent codex)"
	[[ "$(grep -Fc '"id": "local/release-checklist"' <<<"$codex_skills")" -eq 1 ]] ||
		die "expected exactly one Codex binding to release-checklist"

	for source_dir in \
		"$root/home/.codex/skills/code-review" \
		"$root/home/.claude/skills/release-checklist" \
		"$root/home/.cursor/skills/api-design"; do
		[[ -d "$source_dir" && ! -L "$source_dir" ]] || die "original Skill directory was not preserved: $source_dir"
		[[ -s "$source_dir/SKILL.md" ]] || die "original Skill content is missing: $source_dir"
	done

	local link_path="$root/home/.codex/skills/release-checklist"
	local link_target="$root/aikit/library/skills/local/release-checklist"
	[[ -L "$link_path" ]] || die "managed link is missing: $link_path"
	[[ "$(readlink "$link_path")" == "$link_target" ]] || die "managed link target is incorrect: $link_path"

	release_library="$root/aikit/library/skills/local/release-checklist/SKILL.md"
	[[ -s "$release_library" ]] || die "managed release-checklist content is missing"
	if grep -Fq -- 'pending_operations:' "$config_file"; then
		die "pending operations remain after the demo"
	fi
}

clean() {
	local root="$1"

	require_owned_root "$root"
	find "$root" -type d -exec chmod u+w {} +
	find "$root" -mindepth 1 ! -path "$root/$marker_name" -depth -delete
	find "$root/$marker_name" -delete
	rmdir "$root"
	[[ ! -e "$root" && ! -L "$root" ]] || die "failed to remove demo root: $root"
}

[[ $# -eq 3 ]] || usage
operation="$1"
demo_root="${2%/}"
protected_home="${3%/}"
validate_root_argument "$demo_root" "$protected_home"

case "$operation" in
	prepare) prepare "$demo_root" ;;
	verify) require_owned_root "$demo_root"; verify_seed "$demo_root" ;;
	verify-result) verify_result "$demo_root" ;;
	clean) clean "$demo_root" ;;
	*) usage ;;
esac
