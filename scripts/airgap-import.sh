#!/usr/bin/env bash
# Air-gap image import: loads exported tar.gz into k3s on the Ubuntu VM.
# Usage: bash scripts/airgap-import.sh [path-to-tar.gz]
# Default: superstreaming-images.tar.gz in current directory.

set -euo pipefail

ARCHIVE="${1:-superstreaming-images.tar.gz}"

echo ""
echo "SuperStreaming — Air-Gap Image Import"
echo ""

# ── Validate ──────────────────────────────────────────────────────────────────
if [[ ! -f "$ARCHIVE" ]]; then
    echo "ERROR: Archive not found: $ARCHIVE" >&2
    exit 1
fi

if ! command -v k3s &>/dev/null; then
    echo "ERROR: k3s not found. Install k3s first:" >&2
    echo "  curl -sfL https://get.k3s.io | sh -" >&2
    exit 1
fi

# ── Import ────────────────────────────────────────────────────────────────────
SIZE_MB=$(du -m "$ARCHIVE" | cut -f1)
echo "[1/2] Importing $ARCHIVE (${SIZE_MB} MB) into k3s..."
echo "      This may take several minutes."

sudo k3s ctr images import "$ARCHIVE"

echo "      Import complete."

# ── Verify ────────────────────────────────────────────────────────────────────
echo "[2/2] Verifying imported images..."

EXPECTED_IMAGES=(
    "docker.io/library/postgres:16-alpine"
    "docker.io/minio/minio:latest"
    "docker.io/prom/prometheus:latest"
    "docker.io/grafana/grafana:latest"
    "superstreaming/mediamtx:local"
    "superstreaming/recording-uploader:latest"
    "superstreaming/api-server:latest"
    "superstreaming/web-frontend:latest"
)

MISSING=0
for img in "${EXPECTED_IMAGES[@]}"; do
    if sudo k3s ctr images ls 2>/dev/null | grep -q "$(echo $img | sed 's|docker.io/||')"; then
        echo "  OK  $img"
    else
        echo "  ??  $img (check manually: k3s ctr images ls | grep $(basename $img))"
        MISSING=$((MISSING + 1))
    fi
done

echo ""
if [[ $MISSING -eq 0 ]]; then
    echo "All images imported. Deploy with:"
    echo "  kubectl apply -f k8s/namespace.yaml"
    echo "  kubectl apply -f k8s/secrets/"
    echo "  kubectl apply -f k8s/configmaps/"
    echo "  kubectl apply -f k8s/postgres/ k8s/minio/ k8s/origin/ k8s/read/"
    echo "  kubectl apply -f k8s/api-server/ k8s/web-frontend/"
    echo "  kubectl apply -f k8s/prometheus/ k8s/grafana/"
    echo "  kubectl apply -f k8s/recording-uploader/ 2>/dev/null || true"
else
    echo "WARNING: $MISSING image(s) may not have imported correctly."
    echo "Run 'sudo k3s ctr images ls' to inspect."
fi
echo ""
