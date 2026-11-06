#!/bin/sh
# Installs the birdie binary from a GitHub release, then starts onboarding.
#   curl -fsSL https://getbirdie.xyz/install.sh | sh
# Options: --no-onboard, --version vX.Y.Z, --dir PATH
set -eu

REPO_URL="${BIRDIE_RELEASE_URL:-https://github.com/koshikraj/birdie/releases}"

# A download with no data for this long counts as stalled and is retried.
STALL_SECONDS=30
RETRIES=3

die() {
	echo "birdie install: $*" >&2
	exit 1
}

# download URL FILE [progress]. It runs in the background so that stopping the
# installer can stop the download too.
download() {
	if command -v curl >/dev/null 2>&1; then
		show=-sS
		if [ "${3:-}" = progress ] && [ -t 2 ]; then
			show=--progress-bar
		fi
		curl -fL "$show" --connect-timeout "$STALL_SECONDS" \
			--speed-limit 1 --speed-time "$STALL_SECONDS" \
			--retry "$RETRIES" --retry-delay 2 "$1" -o "$2" </dev/null &
	elif command -v wget >/dev/null 2>&1; then
		wget -q -T "$STALL_SECONDS" -t "$((RETRIES + 1))" "$1" -O "$2" </dev/null &
	else
		die "needs curl or wget"
	fi
	child=$!
	wait "$child"
}

stop() {
	[ -z "${child:-}" ] || kill "$child" 2>/dev/null || true
	exit "$1"
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | cut -d' ' -f1
	else
		die "needs sha256sum or shasum to check the download"
	fi
}

main() {
	onboard=1
	version=""
	dir="${BIRDIE_INSTALL_DIR:-$HOME/.local/bin}"
	while [ $# -gt 0 ]; do
		case "$1" in
		--no-onboard) onboard=0 ;;
		--version)
			[ $# -ge 2 ] || die "--version needs a value"
			version="$2"
			shift
			;;
		--dir)
			[ $# -ge 2 ] || die "--dir needs a value"
			dir="$2"
			shift
			;;
		*) die "unknown option: $1" ;;
		esac
		shift
	done

	case "$(uname -s)" in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	*) die "only Linux and macOS are supported here; on Windows, download a binary from $REPO_URL" ;;
	esac
	case "$(uname -m)" in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	armv6* | armv7*) arch=arm ;;
	*) die "unsupported processor: $(uname -m)" ;;
	esac

	if [ -n "$version" ]; then
		base="$REPO_URL/download/$version"
	else
		base="$REPO_URL/latest/download"
	fi
	asset="birdie-$os-$arch"

	tmp="$(mktemp -d)"
	trap 'rm -rf "$tmp"' EXIT
	trap 'stop 130' INT
	trap 'stop 143' TERM

	echo "Downloading $asset..."
	download "$base/$asset" "$tmp/$asset" progress || die "couldn't download $base/$asset; check your connection and run this again"
	download "$base/checksums.txt" "$tmp/checksums.txt" || die "couldn't download $base/checksums.txt; check your connection and run this again"

	want="$(awk -v f="$asset" '$2 == f || $2 == "*" f { print $1 }' "$tmp/checksums.txt")"
	[ -n "$want" ] || die "checksums.txt doesn't list $asset"
	[ "$(sha256 "$tmp/$asset")" = "$want" ] || die "checksum mismatch for $asset; not installing it"

	mkdir -p "$dir"
	chmod 755 "$tmp/$asset"
	mv "$tmp/$asset" "$dir/birdie"
	echo "Installed $("$dir/birdie" version) to $dir/birdie"

	case ":$PATH:" in
	*":$dir:"*) ;;
	*) echo "Add $dir to your PATH to run birdie by name." ;;
	esac

	if [ "$onboard" = 1 ]; then
		if (: </dev/tty) 2>/dev/null; then
			echo
			"$dir/birdie" onboard </dev/tty
		else
			echo "No terminal here, so run this when you can: $dir/birdie onboard"
		fi
	fi
}

main "$@"
