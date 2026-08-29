#!/usr/bin/env bash

set -euo pipefail

readonly minimum_vhs_version="0.9.0"
readonly maximum_gif_bytes=$((10 * 1024 * 1024))
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
assets_dir="$repo_root/docs/assets"
gif_path="$repo_root/docs/assets/aikit-demo.gif"

die() {
	printf 'demo render: %s\n' "$*" >&2
	exit 1
}

require_program() {
	command -v "$1" >/dev/null 2>&1 || die "missing required program: $1"
}

version_at_least() {
	local actual="$1"
	local required="$2"
	local actual_major actual_minor actual_patch
	local required_major required_minor required_patch

	IFS=. read -r actual_major actual_minor actual_patch <<<"${actual%%-*}"
	IFS=. read -r required_major required_minor required_patch <<<"$required"
	actual_patch="${actual_patch:-0}"
	required_patch="${required_patch:-0}"
	[[ "$actual_major" =~ ^[0-9]+$ && "$actual_minor" =~ ^[0-9]+$ && "$actual_patch" =~ ^[0-9]+$ ]] || return 1
	((actual_major > required_major)) ||
		((actual_major == required_major && actual_minor > required_minor)) ||
		((actual_major == required_major && actual_minor == required_minor && actual_patch >= required_patch))
}

for program in awk bash dscl ffmpeg ffprobe go mktemp ttyd; do
	require_program "$program"
done

command -v vhs >/dev/null 2>&1 ||
	die "VHS >= $minimum_vhs_version is required (install with: brew install vhs)"

vhs_version="$(vhs --version | awk '{print $NF}')"
version_at_least "$vhs_version" "$minimum_vhs_version" ||
	die "VHS >= $minimum_vhs_version is required; found ${vhs_version:-unknown} (install with: brew install vhs)"

if [[ ! -f /System/Library/Fonts/Menlo.ttc && ! -f /Library/Fonts/Menlo.ttc ]]; then
	die "Menlo is required at /System/Library/Fonts or /Library/Fonts"
fi

printf 'demo render: Go %s · VHS %s · Menlo available\n' "$(go env GOVERSION)" "$vhs_version"

case "${1:-}" in
	'') ;;
	--check) exit 0 ;;
	*) die "usage: $0 [--check]" ;;
esac

cd "$repo_root"
render_dir="$(mktemp -d "$assets_dir/.aikit-demo-render.XXXXXX")"
raw_gif_path="$render_dir/aikit-demo-raw.gif"
optimized_gif_path="$render_dir/aikit-demo-12fps.gif"
cleanup() {
	if [[ -n "${render_dir:-}" && -d "$render_dir" && ! -L "$render_dir" ]]; then
		find "$render_dir" -depth -delete
	fi
}
trap cleanup EXIT

# Override the tape output so an interrupted render never touches the last
# verified README asset. The final rename stays on the same filesystem.
vhs --output "$raw_gif_path" docs/demo/aikit.tape

# VHS currently captures GIFs at 25 fps even when the tape requests 12 fps.
# Normalize the generated artifact so its real frame rate matches the contract.
ffmpeg -loglevel error -y -i "$raw_gif_path" \
	-filter_complex '[0:v]fps=12,split[a][b];[a]palettegen=max_colors=128:stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=4:diff_mode=rectangle' \
	-loop 0 "$optimized_gif_path"

width="$(ffprobe -v error -select_streams v:0 -show_entries stream=width -of default=noprint_wrappers=1:nokey=1 "$optimized_gif_path")"
height="$(ffprobe -v error -select_streams v:0 -show_entries stream=height -of default=noprint_wrappers=1:nokey=1 "$optimized_gif_path")"
frame_rate="$(ffprobe -v error -select_streams v:0 -show_entries stream=r_frame_rate -of default=noprint_wrappers=1:nokey=1 "$optimized_gif_path")"
frame_count="$(ffprobe -v error -count_frames -select_streams v:0 -show_entries stream=nb_read_frames -of default=noprint_wrappers=1:nokey=1 "$optimized_gif_path")"
duration="$(ffprobe -v error -show_entries format=duration -of default=noprint_wrappers=1:nokey=1 "$optimized_gif_path")"
gif_bytes="$(wc -c <"$optimized_gif_path")"

[[ "$width" == 1100 && "$height" == 660 ]] ||
	die "unexpected GIF dimensions: ${width:-unknown}x${height:-unknown}"
[[ "$frame_rate" == 12/1 ]] || die "unexpected GIF frame rate: ${frame_rate:-unknown}"
[[ "$frame_count" =~ ^[0-9]+$ ]] && ((frame_count >= 240 && frame_count <= 360)) ||
	die "unexpected GIF frame count: ${frame_count:-unknown}"
awk -v duration="$duration" 'BEGIN { exit !(duration >= 20 && duration <= 30) }' ||
	die "unexpected GIF duration: ${duration:-unknown}s"
((gif_bytes > 0 && gif_bytes < maximum_gif_bytes)) ||
	die "unexpected GIF size: ${gif_bytes:-unknown} bytes"

mv "$optimized_gif_path" "$gif_path"
find "$render_dir" -depth -delete
render_dir=''
trap - EXIT
printf 'demo render: verified %sx%s · %s fps · %s frames · %ss · %s bytes\n' \
	"$width" "$height" "${frame_rate%/1}" "$frame_count" "$duration" "$gif_bytes"
