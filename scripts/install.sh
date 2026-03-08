#!/usr/bin/env bash
set -euo pipefail

SCRIPT_PATH="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"
SCRIPT_DIR="$(dirname "$SCRIPT_PATH")"

INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/share/hathgo}"
BINARY_NAME="${BINARY_NAME:-hathgo}"
SERVICE_NAME="${SERVICE_NAME:-hathgo}"
DATA_DIR="${DATA_DIR:-}"
LOG_DIR="${LOG_DIR:-}"
CACHE_DIR="${CACHE_DIR:-}"
TEMP_DIR="${TEMP_DIR:-}"
DOWNLOAD_DIR="${DOWNLOAD_DIR:-}"

FORCE=0
YES=0
SYSTEMD_MODE="ask"
NO_START=0
SKIP_CREDENTIALS=0
CONFIGURE_ONLY=0

CLIENT_ID="${HATH_CLIENT_ID:-${HATHGO_CLIENT_ID:-}}"
CLIENT_KEY="${HATH_CLIENT_KEY:-${HATHGO_CLIENT_KEY:-}}"

usage() {
  cat <<EOF
Usage: bash $0 [options]

Core options:
  --install-dir=PATH       Install directory (default: $HOME/.local/share/hathgo)
  --binary-name=NAME       Installed binary name (default: hathgo)
  --force                  Overwrite existing install
  --yes                    Assume yes for interactive installer prompts
  --configure-only         Only rewrite credentials / service using existing binary
  --skip-credentials       Do not prompt for or write client_login

Credential options:
  --client-id=ID           Write Client ID into data/client_login
  --client-key=KEY         Write Client Key into data/client_login

Path options:
  --data-dir=PATH          Data directory
  --log-dir=PATH           Log directory
  --cache-dir=PATH         Cache directory
  --temp-dir=PATH          Temp directory
  --download-dir=PATH      Download directory

Service options:
  --systemd                Install and enable systemd service
  --no-systemd             Do not install systemd service
  --service-name=NAME      systemd service name (default: hathgo)
  --no-start               Install/enable service but do not start it

This installer is release-only.
Place it next to the prebuilt hathgo binary from GitHub Releases.

Environment variables:
  HATH_CLIENT_ID / HATHGO_CLIENT_ID
  HATH_CLIENT_KEY / HATHGO_CLIENT_KEY
EOF
}

trim() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

is_tty() {
  [[ -t 0 ]]
}

confirm_yes() {
  local prompt="$1"
  if [[ "$YES" == "1" ]]; then
    return 0
  fi
  if ! is_tty; then
    return 1
  fi
  local answer
  read -r -p "$prompt [Y/n] " answer
  answer="$(trim "$answer")"
  [[ -z "$answer" || "$answer" =~ ^[Yy]([Ee][Ss])?$ ]]
}

validate_client_id() {
  [[ "$1" =~ ^[0-9]+$ ]] && ((10#$1 >= 1000))
}

validate_client_key() {
  [[ "$1" =~ ^[A-Za-z0-9]{20}$ ]]
}

resolve_paths() {
  [[ -n "$DATA_DIR" ]] || DATA_DIR="$INSTALL_DIR/data"
  [[ -n "$LOG_DIR" ]] || LOG_DIR="$INSTALL_DIR/log"
  [[ -n "$CACHE_DIR" ]] || CACHE_DIR="$INSTALL_DIR/cache"
  [[ -n "$TEMP_DIR" ]] || TEMP_DIR="$INSTALL_DIR/tmp"
  [[ -n "$DOWNLOAD_DIR" ]] || DOWNLOAD_DIR="$INSTALL_DIR/download"
}

bundled_binary_path() {
  local candidate
  for candidate in "$SCRIPT_DIR/$BINARY_NAME" "$SCRIPT_DIR/hathgo" "$SCRIPT_DIR/hathgo.exe"; do
    if [[ -f "$candidate" ]]; then
      printf '%s' "$candidate"
      return 0
    fi
  done
  return 1
}

copy_self_into_install_dir() {
  local target="$INSTALL_DIR/install.sh"
  if [[ "$(cd "$(dirname "$SCRIPT_PATH")" && pwd)/$(basename "$SCRIPT_PATH")" != "$(cd "$(dirname "$target")" && pwd)/$(basename "$target")" ]]; then
    cp "$SCRIPT_PATH" "$target"
  fi
  chmod +x "$target"
}

write_configure_wrapper() {
  cat > "$INSTALL_DIR/configure-hathgo.sh" <<EOF
#!/usr/bin/env bash
set -euo pipefail
exec "$INSTALL_DIR/install.sh" \
  --install-dir="$INSTALL_DIR" \
  --binary-name="$BINARY_NAME" \
  --service-name="$SERVICE_NAME" \
  --data-dir="$DATA_DIR" \
  --log-dir="$LOG_DIR" \
  --cache-dir="$CACHE_DIR" \
  --temp-dir="$TEMP_DIR" \
  --download-dir="$DOWNLOAD_DIR" \
  --configure-only \
  "\$@"
EOF
  chmod +x "$INSTALL_DIR/configure-hathgo.sh"
}

ensure_directories() {
  mkdir -p "$INSTALL_DIR" "$DATA_DIR" "$LOG_DIR" "$CACHE_DIR" "$TEMP_DIR" "$DOWNLOAD_DIR"
}

install_binary() {
  local bin_path="$INSTALL_DIR/$BINARY_NAME"
  if [[ "$CONFIGURE_ONLY" == "1" ]]; then
    if [[ ! -f "$bin_path" ]]; then
      echo "No installed binary found at $bin_path for configure-only mode." >&2
      exit 1
    fi
    return
  fi

  if [[ -f "$bin_path" && "$FORCE" != "1" ]]; then
    echo "Binary already exists at $bin_path" >&2
    echo "Use --force to overwrite." >&2
    exit 1
  fi

  local bundled_binary
  if ! bundled_binary="$(bundled_binary_path)"; then
    echo "No bundled binary found next to install.sh. Download a GitHub release package first." >&2
    exit 1
  fi

  echo "Installing bundled binary from $bundled_binary to $bin_path"
  if [[ "$(cd "$(dirname "$bundled_binary")" && pwd)/$(basename "$bundled_binary")" != "$(cd "$(dirname "$bin_path")" && pwd)/$(basename "$bin_path")" ]]; then
    cp "$bundled_binary" "$bin_path"
  fi
  chmod +x "$bin_path" 2>/dev/null || true
}

read_existing_login() {
  local login_path="$DATA_DIR/client_login"
  if [[ ! -f "$login_path" ]]; then
    return 1
  fi

  local current_login current_id current_key
  current_login="$(<"$login_path")"
  current_id="${current_login%%-*}"
  current_key="${current_login#*-}"
  if validate_client_id "$current_id" && validate_client_key "$current_key"; then
    printf '%s\n%s\n' "$current_id" "$current_key"
    return 0
  fi
  return 1
}

prompt_client_credentials_if_needed() {
  local login_path="$DATA_DIR/client_login"
  local force_rewrite_credentials=0
  local existing_id=""
  local existing_key=""

  if [[ "$SKIP_CREDENTIALS" == "1" ]]; then
    return
  fi

  if read_existing_login >/dev/null; then
    mapfile -t existing_login < <(read_existing_login)
    existing_id="${existing_login[0]}"
    existing_key="${existing_login[1]}"
  fi

  if [[ "$CONFIGURE_ONLY" == "1" && -z "$CLIENT_ID" && -z "$CLIENT_KEY" && -n "$existing_id" && -n "$existing_key" ]]; then
    if is_tty && ! confirm_yes "Rewrite existing client credentials?"; then
      return
    fi
    CLIENT_ID=""
    CLIENT_KEY=""
    force_rewrite_credentials=1
  fi

  if [[ "$force_rewrite_credentials" != "1" && -z "$CLIENT_ID" && -z "$CLIENT_KEY" && -f "$login_path" ]]; then
    return
  fi

  while [[ -z "$CLIENT_ID" ]]; do
    if ! is_tty; then
      echo "Client ID is required. Pass --client-id or set HATH_CLIENT_ID/HATHGO_CLIENT_ID." >&2
      exit 1
    fi
    read -r -p "Client ID: " CLIENT_ID
    CLIENT_ID="$(trim "$CLIENT_ID")"
    if ! validate_client_id "$CLIENT_ID"; then
      echo "Invalid Client ID. It must be a number >= 1000." >&2
      CLIENT_ID=""
    fi
  done

  while ! validate_client_id "$CLIENT_ID"; do
    echo "Invalid Client ID: $CLIENT_ID" >&2
    exit 1
  done

  while [[ -z "$CLIENT_KEY" ]]; do
    if ! is_tty; then
      echo "Client Key is required. Pass --client-key or set HATH_CLIENT_KEY/HATHGO_CLIENT_KEY." >&2
      exit 1
    fi
    read -r -s -p "Client Key: " CLIENT_KEY
    echo
    CLIENT_KEY="$(trim "$CLIENT_KEY")"
    if ! validate_client_key "$CLIENT_KEY"; then
      echo "Invalid Client Key. It must be exactly 20 alphanumeric characters." >&2
      CLIENT_KEY=""
    fi
  done

  while ! validate_client_key "$CLIENT_KEY"; do
    echo "Invalid Client Key format." >&2
    exit 1
  done

  printf '%s-%s\n' "$CLIENT_ID" "$CLIENT_KEY" > "$login_path"
  chmod 600 "$login_path" 2>/dev/null || true
}

write_run_wrapper() {
  local bin_path="$INSTALL_DIR/$BINARY_NAME"
  cat > "$INSTALL_DIR/run-hathgo.sh" <<EOF
#!/usr/bin/env bash
set -euo pipefail
exec "$bin_path" \
  --data-dir="$DATA_DIR" \
  --log-dir="$LOG_DIR" \
  --cache-dir="$CACHE_DIR" \
  --temp-dir="$TEMP_DIR" \
  --download-dir="$DOWNLOAD_DIR"
EOF
  chmod +x "$INSTALL_DIR/run-hathgo.sh"
}

generate_systemd_unit() {
  local user_name group_name
  user_name="$(id -un)"
  group_name="$(id -gn)"
  cat <<EOF
[Unit]
Description=HentaiAtHomeGo client
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$user_name
Group=$group_name
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/run-hathgo.sh
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF
}

install_systemd_service() {
  local unit_path="$INSTALL_DIR/$SERVICE_NAME.service"
  generate_systemd_unit > "$unit_path"

  if ! command -v systemctl >/dev/null 2>&1; then
    echo "systemctl not found; wrote service template to $unit_path"
    return
  fi

  local system_unit_path="/etc/systemd/system/$SERVICE_NAME.service"
  if [[ "$(id -u)" == "0" ]]; then
    cp "$unit_path" "$system_unit_path"
    systemctl daemon-reload
    systemctl enable "$SERVICE_NAME"
    if [[ "$NO_START" != "1" ]]; then
      systemctl restart "$SERVICE_NAME"
    fi
    return
  fi

  if command -v sudo >/dev/null 2>&1; then
    cat "$unit_path" | sudo tee "$system_unit_path" >/dev/null
    sudo systemctl daemon-reload
    sudo systemctl enable "$SERVICE_NAME"
    if [[ "$NO_START" != "1" ]]; then
      sudo systemctl restart "$SERVICE_NAME"
    fi
    return
  fi

  echo "No permission to install systemd service automatically." >&2
  echo "Service template written to $unit_path" >&2
}

for arg in "$@"; do
  case "$arg" in
    --install-dir=*) INSTALL_DIR="${arg#*=}" ;;
    --binary-name=*) BINARY_NAME="${arg#*=}" ;;
    --service-name=*) SERVICE_NAME="${arg#*=}" ;;
    --client-id=*) CLIENT_ID="${arg#*=}" ;;
    --client-key=*) CLIENT_KEY="${arg#*=}" ;;
    --data-dir=*) DATA_DIR="${arg#*=}" ;;
    --log-dir=*) LOG_DIR="${arg#*=}" ;;
    --cache-dir=*) CACHE_DIR="${arg#*=}" ;;
    --temp-dir=*) TEMP_DIR="${arg#*=}" ;;
    --download-dir=*) DOWNLOAD_DIR="${arg#*=}" ;;
    --force) FORCE=1 ;;
    --yes) YES=1 ;;
    --systemd) SYSTEMD_MODE="on" ;;
    --no-systemd) SYSTEMD_MODE="off" ;;
    --no-start) NO_START=1 ;;
    --skip-credentials) SKIP_CREDENTIALS=1 ;;
    --configure-only) CONFIGURE_ONLY=1 ;;
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

resolve_paths
ensure_directories
install_binary
copy_self_into_install_dir
write_configure_wrapper
prompt_client_credentials_if_needed
write_run_wrapper

if [[ "$SYSTEMD_MODE" == "ask" ]] && command -v systemctl >/dev/null 2>&1 && is_tty; then
  if confirm_yes "Install and enable systemd service '$SERVICE_NAME'?"; then
    SYSTEMD_MODE="on"
  else
    SYSTEMD_MODE="off"
  fi
fi

if [[ "$SYSTEMD_MODE" == "on" ]]; then
  install_systemd_service
fi

cat <<EOF
Install complete.

Binary:
  $INSTALL_DIR/$BINARY_NAME

Launcher:
  $INSTALL_DIR/run-hathgo.sh

Credentials:
  $DATA_DIR/client_login

Reconfigure later:
  $INSTALL_DIR/configure-hathgo.sh
EOF

if [[ "$SYSTEMD_MODE" == "on" ]]; then
  if [[ "$NO_START" == "1" ]]; then
    echo
    echo "Service installed but not started (--no-start)."
  else
    echo
    echo "Service installed and started: $SERVICE_NAME"
  fi
else
  echo
  echo "Start manually with:"
  echo "  $INSTALL_DIR/run-hathgo.sh"
fi
