#!/bin/sh
# Installs the birdie binary from a GitHub release, then starts onboarding.
#   curl -fsSL https://raw.githubusercontent.com/koshikraj/birdie/main/install.sh | sh
# Options: --no-onboard, --version vX.Y.Z, --dir PATH
set -eu

REPO_URL="${BIRDIE_RELEASE_URL:-https://github.com/koshikraj/birdie/releases}"

die() {
	echo "birdie install: $*" >&2
	exit 1
}

download() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$1" -o "$2"
	elif command -v wget >/dev/null 2>&1; then
		wget -q "$1" -O "$2"
	else
		die "needs curl or wget"
	fi
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
	trap 'rm -rf "$tmp"' EXIT INT TERM

	echo "Downloading $asset..."
	download "$base/$asset" "$tmp/$asset" || die "couldn't download $base/$asset"
	download "$base/checksums.txt" "$tmp/checksums.txt" || die "couldn't download checksums.txt"

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
