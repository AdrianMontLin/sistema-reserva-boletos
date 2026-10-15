#!/usr/bin/env bash
# Levanta TODO el sistema en un solo equipo (localhost) y corre una carga corta.
# Sirve para comprobar que el proyecto funciona antes de desplegarlo en los 3 nodos.
#
# Uso: ./scripts/demo_local.sh [duracion_segundos]   (por defecto 20)
set -euo pipefail
cd "$(dirname "$0")/.."

DURACION="${1:-20}"
mkdir -p bin logs data
echo ">> Compilando..."
go build -o bin/ ./cmd/...

PIDS=()
limpiar() { for p in "${PIDS[@]:-}"; do kill -INT "$p" 2>/dev/null || true; done; wait 2>/dev/null || true; }
trap limpiar EXIT

echo ">> Levantando Persistencia (9904), RMC (9902) y Gestor de Transacciones (9900)..."
./bin/persistencia -bind tcp://127.0.0.1:9904 -archivo data/demo_bd.json -reiniciar -estado-cada 5s > logs/persistencia.log 2>&1 & PIDS+=($!)
sleep 1
./bin/rmc -bind tcp://127.0.0.1:9902 -persistencia tcp://127.0.0.1:9904 -q > logs/rmc.log 2>&1 & PIDS+=($!)
./bin/gestor_transacciones -bind tcp://127.0.0.1:9900 -rmc tcp://127.0.0.1:9902 -reporte-cada 0 -q -metricas logs/metricas_gt.csv > logs/gt.log 2>&1 & PIDS+=($!)
sleep 1

echo ">> Carga: 3 clientes (2 solo consulta) durante ${DURACION}s"
./bin/cliente -gt tcp://127.0.0.1:9900 -id N1 -clientes 3 -solo-consulta 2 -rpm 60 -pct-consulta 50 \
  -duracion "${DURACION}s" -q -salida logs/resultados

limpiar; trap - EXIT
echo; echo ">> Métricas del Gestor de Transacciones:"; cat logs/gt.log | tail -4
echo; echo ">> Estado final de la base de datos:"; tail -1 logs/persistencia.log
echo; echo "Logs en ./logs  |  Snapshot de la BD: ./data/demo_bd.json  (ver: ./bin/verbd data/demo_bd.json)"
