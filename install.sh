#!/usr/bin/env bash
# elite-clipboard installer
# Builds binaries, installs symlinks, and writes systemd user services with
# the correct display-server environment for this machine.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVICE_DIR="$HOME/.config/systemd/user"

echo "──────────────────────────────────────────────"
echo "  elite-clipboard  ::  installer"
echo "  install dir: $SCRIPT_DIR"
echo "──────────────────────────────────────────────"

# ── 1. detect display server ───────────────────────────────────────────────────
ENV_BLOCK=""

if [[ -n "${WAYLAND_DISPLAY:-}" ]]; then
    echo "[info]  display server : Wayland  (WAYLAND_DISPLAY=${WAYLAND_DISPLAY})"
    ENV_BLOCK="Environment=WAYLAND_DISPLAY=${WAYLAND_DISPLAY}"
    if [[ -n "${XDG_RUNTIME_DIR:-}" ]]; then
        ENV_BLOCK+=$'\n'"Environment=XDG_RUNTIME_DIR=${XDG_RUNTIME_DIR}"
    fi
    # Include DISPLAY too when XWayland is active (both may be set)
    if [[ -n "${DISPLAY:-}" ]]; then
        ENV_BLOCK+=$'\n'"Environment=DISPLAY=${DISPLAY}"
    fi
elif [[ -n "${DISPLAY:-}" ]]; then
    echo "[info]  display server : X11  (DISPLAY=${DISPLAY})"
    ENV_BLOCK="Environment=DISPLAY=${DISPLAY}"
    ENV_BLOCK+=$'\n'"Environment=XAUTHORITY=${HOME}/.Xauthority"
else
    echo "[warn]  no DISPLAY or WAYLAND_DISPLAY found — defaulting to DISPLAY=:0"
    echo "        edit ~/.config/systemd/user/elite-clipboard.service after install if needed"
    ENV_BLOCK="Environment=DISPLAY=:0"
    ENV_BLOCK+=$'\n'"Environment=XAUTHORITY=${HOME}/.Xauthority"
fi

# ── 2. build ───────────────────────────────────────────────────────────────────
echo "[build] compiling binaries..."
cd "$SCRIPT_DIR"
mkdir -p bin
go build -o bin/elite-clipd ./cmd/daemon/
go build -o bin/ecb        ./cmd/ecb/
go build -o bin/elite-tray ./cmd/tray/
echo "[build] done"

# ── 3. install symlinks ────────────────────────────────────────────────────────
echo "[link]  installing to /usr/local/bin  (requires sudo)"
sudo ln -sf "${SCRIPT_DIR}/bin/elite-clipd" /usr/local/bin/elite-clipd
sudo ln -sf "${SCRIPT_DIR}/bin/ecb"         /usr/local/bin/ecb
sudo ln -sf "${SCRIPT_DIR}/bin/ecb-pick"    /usr/local/bin/ecb-pick
echo "[link]  done"

# ── 4. systemd user services ───────────────────────────────────────────────────
mkdir -p "$SERVICE_DIR"

cat > "$SERVICE_DIR/elite-clipboard.service" <<SERVICE
[Unit]
Description=elite-clipboard daemon
After=graphical-session.target
PartOf=graphical-session.target

[Service]
Type=simple
ExecStart=${SCRIPT_DIR}/bin/elite-clipd
Restart=on-failure
RestartSec=3
StandardOutput=append:%h/.local/share/elite-clipboard/daemon.log
StandardError=append:%h/.local/share/elite-clipboard/daemon.log
${ENV_BLOCK}

[Install]
WantedBy=graphical-session.target
SERVICE

cat > "$SERVICE_DIR/elite-tray.service" <<SERVICE
[Unit]
Description=elite-clipboard tray
After=graphical-session.target elite-clipboard.service
Wants=elite-clipboard.service

[Service]
Type=simple
ExecStart=${SCRIPT_DIR}/bin/elite-tray
Restart=on-failure
RestartSec=3
${ENV_BLOCK}

[Install]
WantedBy=graphical-session.target
SERVICE

echo "[svc]   service files written to $SERVICE_DIR"

# ── 5. enable & start ──────────────────────────────────────────────────────────
systemctl --user daemon-reload
systemctl --user enable elite-clipboard.service elite-tray.service
systemctl --user restart elite-clipboard.service elite-tray.service
echo "[svc]   services enabled and started"

echo ""
echo "──────────────────────────────────────────────"
echo "  Installation complete!"
echo "  Verify: ecb ping"
echo "──────────────────────────────────────────────"
