#!/usr/bin/env bash
# Buduje libaec (CCSDS, wymagane przez ECMWF open data) i ecCodes z CMake do lab4/.eccodes
set -euo pipefail

ECCODES_VERSION="${ECCODES_VERSION:-2.50.1}"
LIBAEC_VERSION="${LIBAEC_VERSION:-1.1.4}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PREFIX="$ROOT/.eccodes"
WORK="$ROOT/.build"
JOBS="$(nproc 2>/dev/null || echo 4)"

mkdir -p "$WORK" "$PREFIX"
cd "$WORK"

fetch() { # url archive
  [ -f "$2" ] || curl -fL --retry 3 -o "$2" "$1"
}

# --- libaec ---
fetch "https://github.com/MathisRosenhauer/libaec/archive/refs/tags/v${LIBAEC_VERSION}.tar.gz" "libaec.tar.gz"
rm -rf "libaec-src" && mkdir "libaec-src" && tar xzf libaec.tar.gz -C libaec-src --strip-components=1
cmake -S libaec-src -B libaec-build -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX="$PREFIX" -DBUILD_TESTING=OFF
cmake --build libaec-build -j"$JOBS"
cmake --install libaec-build

# --- ecCodes ---
fetch "https://github.com/ecmwf/eccodes/archive/refs/tags/${ECCODES_VERSION}.tar.gz" "eccodes.tar.gz"
rm -rf "eccodes-src" && mkdir "eccodes-src" && tar xzf eccodes.tar.gz -C eccodes-src --strip-components=1
cmake -S eccodes-src -B eccodes-build \
  -DCMAKE_BUILD_TYPE=Release \
  -DCMAKE_INSTALL_PREFIX="$PREFIX" \
  -DCMAKE_PREFIX_PATH="$PREFIX" \
  -DENABLE_FORTRAN=OFF -DENABLE_NETCDF=OFF -DENABLE_PYTHON=OFF \
  -DENABLE_MEMFS=OFF -DENABLE_EXAMPLES=OFF -DENABLE_TESTS=OFF \
  -DENABLE_AEC=ON
cmake --build eccodes-build -j"$JOBS"
cmake --install eccodes-build

echo "OK: ecCodes zainstalowany w $PREFIX"
echo "Ustaw: export ECCODES_DIR=$PREFIX  (lub użyj scripts/env.sh)"
