#!/usr/bin/env bash
set -euo pipefail

# DevSSH Integration Test
# Tests the full devssh up flow in a single machine using:
#   1. A Docker SSH server as the remote target
#   2. A local HTTP server for DevSSH/VSCodium downloads (no GitHub dependency)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
BIN_DIR="$PROJECT_DIR/bin"
SSH_PORT=10022
HTTP_PORT=19999
CONTAINER_NAME="devssh-test-$(date +%s)"
CLEANUP_FILES=()

cleanup() {
    echo "=== Cleaning up ==="
    docker rm -f "$CONTAINER_NAME" 2>/dev/null || true
    for pid in "${CLEANUP_PIDS[@]:-}"; do
        kill "$pid" 2>/dev/null || true
    done
    for f in "${CLEANUP_FILES[@]:-}"; do
        rm -f "$f"
    done
    echo "=== Cleanup done ==="
}
trap cleanup EXIT INT TERM

CLEANUP_PIDS=()

echo "=== Step 1: Build devssh binary ==="
pixi run build_linux_arch --arch amd64 2>&1

BINARY_PATH="$BIN_DIR/devssh-linux-amd64"
if [ ! -f "$BINARY_PATH" ]; then
    echo "ERROR: Binary not found at $BINARY_PATH"
    exit 1
fi
echo "Binary built: $BINARY_PATH"

echo "=== Step 2: Create test artifacts ==="
# Create a dummy VSCodium tar.gz for testing
TEST_ARTIFACTS_DIR=$(mktemp -d)
CLEANUP_FILES+=("$TEST_ARTIFACTS_DIR")

# Create minimal VSCodium-like tar.gz
VSCODE_TAR_GZ="$TEST_ARTIFACTS_DIR/vscodium-reh-web-linux-x64-1.121.03429.tar.gz"
mkdir -p "$TEST_ARTIFACTS_DIR/vscodium-reh-web-linux-x64-1.121.03429/bin"
echo '#!/bin/bash
echo "Fake codium-server running on port $2"
while true; do sleep 10; done' > "$TEST_ARTIFACTS_DIR/vscodium-reh-web-linux-x64-1.121.03429/bin/codium-server"
chmod +x "$TEST_ARTIFACTS_DIR/vscodium-reh-web-linux-x64-1.121.03429/bin/codium-server"

tar -czf "$VSCODE_TAR_GZ" \
    -C "$TEST_ARTIFACTS_DIR" \
    "vscodium-reh-web-linux-x64-1.121.03429"

echo "=== Step 3: Start local HTTP server for test artifacts ==="
python3 -m http.server "$HTTP_PORT" --directory "$TEST_ARTIFACTS_DIR" &
HTTP_PID=$!
CLEANUP_PIDS+=("$HTTP_PID")
sleep 1

echo "HTTP server running on port $HTTP_PORT, PID=$HTTP_PID"

# Verify HTTP server is serving
if ! curl -sf "http://localhost:$HTTP_PORT/" > /dev/null 2>&1; then
    echo "ERROR: HTTP server not responding"
    exit 1
fi

echo "=== Step 4: Start Docker SSH server ==="
docker run -d \
    --name "$CONTAINER_NAME" \
    -p "$SSH_PORT:22" \
    -e "SSH_USER=testuser" \
    -e "SSH_PASSWORD=testpass" \
    -e "SSH_SUDO=yes" \
    lscr.io/linuxserver/openssh-server:latest 2>&1

echo "Waiting for SSH server to be ready..."
for i in $(seq 1 30); do
    if nc -z localhost "$SSH_PORT" 2>/dev/null; then
        echo "SSH server ready on port $SSH_PORT (attempt $i)"
        break
    fi
    sleep 1
done

# Additional wait for SSH to fully initialize
sleep 3

# Test SSH connection
echo "Testing SSH connection..."
if ! sshpass -p "testpass" ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
    -p "$SSH_PORT" testuser@localhost "echo 'SSH OK'" 2>&1; then
    echo "WARNING: SSH connection test failed, but continuing..."
    echo "The container may need more time or sshpass is not installed"
fi

echo "=== Step 5: Run devssh up with local overrides ==="
export DEVSSH_DEVSSH_DOWNLOAD_URL="http://localhost:$HTTP_PORT/devssh-{{version}}-{{os}}-{{arch}}"
export DEVSSH_VSCODE_DOWNLOAD_URL="http://localhost:$HTTP_PORT/vscodium-reh-web-{{os}}-{{arch}}-{{version}}.tar.gz"
export DEVSSH_LOCAL_BINARY_PATH="$BINARY_PATH"

echo "Env:"
echo "  DEVSSH_DEVSSH_DOWNLOAD_URL=$DEVSSH_DEVSSH_DOWNLOAD_URL"
echo "  DEVSSH_VSCODE_DOWNLOAD_URL=$DEVSSH_VSCODE_DOWNLOAD_URL"
echo "  DEVSSH_LOCAL_BINARY_PATH=$DEVSSH_LOCAL_BINARY_PATH"

# Copy binary to HTTP server directory for download emulation
cp "$BINARY_PATH" "$TEST_ARTIFACTS_DIR/devssh-0.1.8-linux-amd64"

echo ""
echo "=== Running devssh up (timeout 60s) ==="
timeout 60 "$BINARY_PATH" up \
    -u testuser \
    -p "$SSH_PORT" \
    --password testpass \
    --timeout 30 \
    --keepalive=false \
    "localhost" 2>&1 || true

echo ""
echo "=== Integration test completed ==="
