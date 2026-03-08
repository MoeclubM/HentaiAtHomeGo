#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/share/hathgo}"
BINARY_NAME="${BINARY_NAME:-hathgo}"
FORCE=0

usage() {
  cat <<EOF
Usage: bash $0 [--install-dir=PATH] [--binary-name=NAME] [--force]

Modes:
  - Source tree mode: run from repo/scripts/install.sh, builds ./cmd/client
  - Release package mode: run from extracted package install.sh, installs bundled binary
EOF
}

for arg in "$@"; do
  case "$arg" in
    --install-dir=*) INSTALL_DIR="${arg#*=}" ;;
    --binary-name=*) BINARY_NAME="${arg#*=}" ;;
    --force) FORCE=1 ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $arg" >&2
      usage >&2
      exit 1
      ;;
  esac
done

mkdir -p "$INSTALL_DIR"

BIN_PATH="$INSTALL_DIR/$BINARY_NAME"
WRAPPER_PATH="$INSTALL_DIR/run-hathgo.sh"

if [[ -f "$BIN_PATH" && "$FORCE" != "1" ]]; then
  echo "Binary already exists at $BIN_PATH" >&2
  echo "Use --force to overwrite." >&2
  exit 1
fi

SOURCE_TREE=0
if [[ -d "$REPO_ROOT/cmd/client" && -d "$REPO_ROOT/internal" ]]; then
  SOURCE_TREE=1
fi

if [[ "$SOURCE_TREE" == "1" ]]; then
  if ! command -v go >/dev/null 2>&1; then
    echo "Go is required in source tree mode but was not found in PATH." >&2
    exit 1
  fi

  echo "Building client into $BIN_PATH"
  (
    cd "$REPO_ROOT"
    go build -trimpath -o "$BIN_PATH" ./cmd/client
  )
else
  BUNDLED_BINARY=""
  for candidate in "$SCRIPT_DIR/$BINARY_NAME" "$SCRIPT_DIR/hathgo" "$SCRIPT_DIR/hathgo.exe"; do
    if [[ -f "$candidate" ]]; then
      BUNDLED_BINARY="$candidate"
      break
    fi
  done

  if [[ -z "$BUNDLED_BINARY" ]]; then
    echo "No source tree or bundled binary found next to install.sh." >&2
    exit 1
  fi

  echo "Installing bundled binary from $BUNDLED_BINARY to $BIN_PATH"
  if [[ "$(cd "$(dirname "$BUNDLED_BINARY")" && pwd)/$(basename "$BUNDLED_BINARY")" != "$(cd "$(dirname "$BIN_PATH")" && pwd)/$(basename "$BIN_PATH")" ]]; then
    cp "$BUNDLED_BINARY" "$BIN_PATH"
  fi
fi

for dir in data log cache tmp download certs; do
  mkdir -p "$INSTALL_DIR/$dir"
done

cat > "$WRAPPER_PATH" <<EOF
#!/usr/bin/env bash
set -euo pipefail
exec "$BIN_PATH" \
  --data-dir="$INSTALL_DIR/data" \
  --log-dir="$INSTALL_DIR/log" \
  --cache-dir="$INSTALL_DIR/cache" \
  --temp-dir="$INSTALL_DIR/tmp" \
  --download-dir="$INSTALL_DIR/download" \
  "\$@"
EOF

chmod +x "$BIN_PATH" "$WRAPPER_PATH"

cat <<EOF
Install complete.

Binary:
  $BIN_PATH

Launcher:
  $WRAPPER_PATH

Run with:
  $WRAPPER_PATH
EOF
