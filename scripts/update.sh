#!/bin/sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
THIMBLE_VERSION="${THIMBLE_VERSION:-latest}" exec "$SCRIPT_DIR/install.sh"
