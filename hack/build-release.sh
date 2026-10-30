#!/usr/bin/env bash
# build-release.sh - cross compile frps/frpc and pack the release zips.
#
# The version is "<upstream version>.plus.<build number>", for example
# 0.71.1.plus.1. The upstream part comes from pkg/util/version/version.go and is
# never touched here, the fork build number is injected through -ldflags so that
# syncing with upstream stays conflict free.
#
# Usage (from the repository root):
#   bash hack/build-release.sh              # next build number, default platforms
#   bash hack/build-release.sh -b 3         # force the build number
#   bash hack/build-release.sh --no-web     # do not rebuild the dashboards
#   bash hack/build-release.sh -p "linux/amd64 linux/arm64"
#
# Output: the zips in bin/, the staged trees in release/pack/.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# default platform matrix, os/arch, arm is built with GOARM=7
DEFAULT_PLATFORMS="windows/amd64 linux/amd64 linux/arm linux/arm64"
PLATFORMS="$DEFAULT_PLATFORMS"
BUILD_NO=""
WITH_WEB=1

usage() {
	sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'
}

while [ $# -gt 0 ]; do
	case "$1" in
	-b | --build)
		BUILD_NO="${2:?missing build number}"
		shift 2
		;;
	-p | --platforms)
		PLATFORMS="${2:?missing platform list}"
		shift 2
		;;
	--no-web)
		WITH_WEB=0
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "unknown argument: $1" >&2
		usage >&2
		exit 1
		;;
	esac
done

# --- toolchain ---------------------------------------------------------------
GO="$(command -v go || true)"
if [ -z "$GO" ]; then
	for candidate in "/c/Program Files/Go/bin/go.exe" "/usr/local/go/bin/go"; do
		[ -x "$candidate" ] && GO="$candidate" && break
	done
fi
if [ -z "$GO" ]; then
	echo "go not found, add it to PATH first" >&2
	exit 1
fi
# A shell without a usable HOME makes go report GOCACHE=off, which is fatal for
# any modern go command, so treat it as missing.
usable_cache() {
	case "${1:-}" in
	"" | off) return 1 ;;
	*) return 0 ;;
	esac
}
if ! usable_cache "${GOCACHE:-}"; then
	candidate="$("$GO" env GOCACHE 2>/dev/null || true)"
	if usable_cache "$candidate"; then
		GOCACHE="$candidate"
	else
		GOCACHE="${TMPDIR:-/tmp}/frp-gocache"
	fi
fi
[ -n "${GOPATH:-}" ] || GOPATH="$("$GO" env GOPATH 2>/dev/null || true)"
[ -n "${GOPATH:-}" ] || GOPATH="${TMPDIR:-/tmp}/frp-gopath"
export GOCACHE GOPATH CGO_ENABLED=0

# --- version -----------------------------------------------------------------
BASE_VERSION="$(sed -n 's/^var version = "\(.*\)"/\1/p' pkg/util/version/version.go | head -1)"
if [ -z "$BASE_VERSION" ]; then
	echo "cannot read the version from pkg/util/version/version.go" >&2
	exit 1
fi

if [ -z "$BUILD_NO" ]; then
	# next number after the existing tags of this upstream version
	last="$(git tag --list "v${BASE_VERSION}.plus.*" 2>/dev/null |
		sed "s/^v${BASE_VERSION//./\\.}\.plus\.//" |
		grep -E '^[0-9]+$' | sort -n | tail -1 || true)"
	BUILD_NO=$(( ${last:-0} + 1 ))
fi

VERSION="${BASE_VERSION}.plus.${BUILD_NO}"
STAGE="release/pack"
LDFLAGS="-s -w -X github.com/fatedier/frp/pkg/util/version.buildVersion=${VERSION}"

echo "version : ${VERSION}"
echo "platform: ${PLATFORMS}"
echo

# --- dashboards --------------------------------------------------------------
if [ "$WITH_WEB" = "1" ]; then
	if command -v npm >/dev/null 2>&1; then
		echo "building dashboards..."
		(cd web && npm run build-only --workspace frps >/dev/null && npm run build-only --workspace frpc >/dev/null)
	else
		echo "npm not found, keeping the existing web dist" >&2
	fi
fi

WEB_TAGS=""
if [ -d web/frps/dist ] && [ -d web/frpc/dist ]; then
	WEB_TAGS=""
else
	echo "warning: web dist missing, building without embedded dashboard (noweb)" >&2
	WEB_TAGS=",noweb"
fi

# --- build -------------------------------------------------------------------
rm -rf "$STAGE"
mkdir -p "$STAGE" bin

for platform in $PLATFORMS; do
	os="${platform%%/*}"
	arch="${platform##*/}"
	ext=""
	arm_env=""
	target="$arch"

	if [ "$os" = "windows" ]; then
		ext=".exe"
	fi
	if [ "$os/$arch" = "linux/arm" ]; then
		arm_env="GOARM=7"
		target="arm"
	fi

	dir="${STAGE}/frpsplus_${VERSION}_${os}_${target}"
	mkdir -p "$dir"
	echo "build ${os}/${arch}..."

	env GOOS="$os" GOARCH="$arch" $arm_env "$GO" build -trimpath -ldflags "$LDFLAGS" \
		-tags "frps${WEB_TAGS}" -o "$dir/frpsplus${ext}" ./cmd/frps
	env GOOS="$os" GOARCH="$arch" $arm_env "$GO" build -trimpath -ldflags "$LDFLAGS" \
		-tags "frpc${WEB_TAGS}" -o "$dir/frpcplus${ext}" ./cmd/frpc

	cp LICENSE "$dir/"
	cp conf/frps.toml "$dir/frps.example.toml"
	cp conf/frpc.toml "$dir/frpc.example.toml"

	# pack: prefer zip, fall back to the bsdtar shipped with windows
	zip_out="bin/frpsplus_${VERSION}_${os}_${target}.zip"
	rm -f "$zip_out"
	if command -v zip >/dev/null 2>&1; then
		(cd "$STAGE" && zip -rq "$ROOT/$zip_out" "frpsplus_${VERSION}_${os}_${target}")
	elif [ -x /c/WINDOWS/system32/tar.exe ]; then
		(cd "$STAGE" && /c/WINDOWS/system32/tar.exe -a -c -f "$ROOT/$zip_out" "frpsplus_${VERSION}_${os}_${target}")
	else
		tar -a -c -f "$ROOT/$zip_out" -C "$STAGE" "frpsplus_${VERSION}_${os}_${target}"
	fi
	echo "  -> ${zip_out} ($(du -h "$zip_out" | cut -f1))"
done

# --- sanity check ------------------------------------------------------------
case "$(uname -s)" in
MSYS* | MINGW* | CYGWIN*)
	if [ -x "${STAGE}/frpsplus_${VERSION}_windows_amd64/frpsplus.exe" ]; then
		reported="$("${STAGE}/frpsplus_${VERSION}_windows_amd64/frpsplus.exe" -v 2>/dev/null | head -1)"
		echo
		echo "check  : windows/amd64 frpsplus -v -> ${reported}"
		[ "$reported" = "$VERSION" ] || {
			echo "version mismatch, expected ${VERSION}" >&2
			exit 1
		}
	fi
	;;
esac

# --- summary -----------------------------------------------------------------
echo
echo "packages in bin/:"
if command -v sha256sum >/dev/null 2>&1; then
	for f in bin/frpsplus_"${VERSION}"_*.zip; do
		echo "  $(sha256sum "$f" | cut -d' ' -f1)  $(basename "$f")"
	done
else
	ls -l bin/frpsplus_"${VERSION}"_*.zip 2>/dev/null || true
fi
echo
echo "next: git tag v${VERSION} && git push origin v${VERSION}"
