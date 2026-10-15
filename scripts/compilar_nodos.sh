#!/usr/bin/env bash
# Compila los programas para una máquina que no tiene Go instalado (por ejemplo, la VM del Nodo 3).
# El proyecto es Go puro (sin cgo), así que la compilación cruzada funciona directamente.
#
# Uso: ./scripts/compilar_nodos.sh [sistema] [arquitectura]     (por defecto: linux amd64)
#   ej. ./scripts/compilar_nodos.sh windows amd64
# Luego copia la carpeta bin/<sistema>-<arquitectura>/ a la máquina destino (scp, memoria USB, etc.).
set -euo pipefail
cd "$(dirname "$0")/.."

SO="${1:-linux}"
ARQ="${2:-amd64}"
DESTINO="bin/${SO}-${ARQ}"
EXT=""; [ "$SO" = "windows" ] && EXT=".exe"
mkdir -p "$DESTINO"

for prog in persistencia rmc gestor_transacciones cliente metricas verbd; do
  CGO_ENABLED=0 GOOS="$SO" GOARCH="$ARQ" go build -o "${DESTINO}/${prog}${EXT}" "./cmd/${prog}"
done
echo "Binarios para ${SO}/${ARQ} en ${DESTINO}/:"; ls -lh "$DESTINO"
