#!/usr/bin/env bash
set -euo pipefail

REPO="${REPO:-MoeclubM/HentaiAtHomeGo}"
VERSION="${VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/share/hathgo}"
FORWARD_ARGS=()
RAW_REF="${RAW_REF:-main}"

usage() {
  cat <<EOF
Usage: curl -fsSL https://raw.githubusercontent.com/MoeclubM/HentaiAtHomeGo/main/scripts/bootstrap-install.sh | bash -s -- [options]

Bootstrap options:
  --repo=OWNER/REPO        GitHub repo (default: MoeclubM/HentaiAtHomeGo)
  --version=TAG|latest     Release tag to install (default: latest)
  --raw-ref=REF            Branch/tag used to fetch raw install.sh (default: main)

All other options are forwarded to the release install.sh, for example:
  --install-dir=/opt/hathgo
  --client-id=51839
  --client-key=YOUR20CHARKEY
  --systemd
  --yes
  --force
EOF
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Required command not found: $1" >&2
    exit 1
  fi
}

download_to_file() {
  local url="$1"
  local output="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$output"
    return
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -qO "$output" "$url"
    return
  fi
  echo "curl or wget is required" >&2
  exit 1
}

resolve_latest_version() {
  local api_url="https://api.github.com/repos/$REPO/releases/latest"
  local response
  response="$(curl -fsSL "$api_url")"
  printf '%s' "$response" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1
}

detect_arch() {
  local machine
  machine="$(uname -m)"
  case "$machine" in
    x86_64|amd64) printf 'amd64' ;;
    aarch64|arm64) printf 'arm64' ;;
    *)
      echo "Unsupported architecture: $machine" >&2
      exit 1
      ;;
  esac
}

for arg in "$@"; do
  case "$arg" in
    --repo=*) REPO="${arg#*=}" ;;
    --version=*) VERSION="${arg#*=}" ;;
    --raw-ref=*) RAW_REF="${arg#*=}" ;;
    -h|--help)
      usage
      exit 0
      ;;
    *) FORWARD_ARGS+=("$arg") ;;
  esac
done

require_cmd tar

if [[ "$VERSION" == "latest" ]]; then
  require_cmd curl
  VERSION="$(resolve_latest_version)"
  if [[ -z "$VERSION" ]]; then
    echo "Unable to resolve latest release version from GitHub" >&2
    exit 1
  fi
fi

ARCH="$(detect_arch)"
ASSET="hathgo_${VERSION}_linux_${ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/$REPO/releases/download/$VERSION/$ASSET"

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

ARCHIVE_PATH="$WORKDIR/$ASSET"
EXTRACT_DIR="$WORKDIR/extract"

echo "Downloading $DOWNLOAD_URL"
download_to_file "$DOWNLOAD_URL" "$ARCHIVE_PATH"

mkdir -p "$EXTRACT_DIR"
tar -C "$EXTRACT_DIR" -xzf "$ARCHIVE_PATH"

PACKAGE_DIR="$EXTRACT_DIR/hathgo_${VERSION}_linux_${ARCH}"
if [[ ! -d "$PACKAGE_DIR" ]]; then
  echo "Unexpected archive layout: $PACKAGE_DIR not found" >&2
  exit 1
fi

STAGE_DIR="$WORKDIR/stage"
mkdir -p "$STAGE_DIR"

if [[ -f "$PACKAGE_DIR/hathgo" ]]; then
  cp "$PACKAGE_DIR/hathgo" "$STAGE_DIR/hathgo"
else
  echo "Extracted package does not contain a Linux hathgo binary" >&2
  exit 1
fi

INSTALLER_URL="https://raw.githubusercontent.com/$REPO/$RAW_REF/scripts/install.sh"
download_to_file "$INSTALLER_URL" "$STAGE_DIR/install.sh"
chmod +x "$STAGE_DIR/install.sh" "$STAGE_DIR/hathgo"

echo "Installing $VERSION to $INSTALL_DIR"
bash "$STAGE_DIR/install.sh" --install-dir="$INSTALL_DIR" "${FORWARD_ARGS[@]}"
