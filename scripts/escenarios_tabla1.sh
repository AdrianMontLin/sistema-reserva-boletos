#!/usr/bin/env bash
# Ejecuta los escenarios de prueba de la Tabla 1 desde UN nodo.
#
#   3 clientes: 2 solo consulta + 1 consulta/reserva (50/50)
#   5 clientes: 3 solo consulta + 2 reserva
#   8 clientes: 4 solo consulta + 4 reserva
#
# (En la Entrega 1 no existe la modificación, por eso "reserva y modificación"
#  se ejecuta con reservas.)
#
# Uso: ./scripts/escenarios_tabla1.sh <direccion_GT> [3|5|8|todos] [duracion] [id_origen]
# Ej.: ./scripts/escenarios_tabla1.sh tcp://100.64.0.2:9900 todos 2m N1
#
# Recomendado: reiniciar la Persistencia con -reiniciar entre escenarios para
# que todos partan de la misma base de datos.
set -euo pipefail
cd "$(dirname "$0")/.."

GT="${1:?Indica la dirección del GT, ej: tcp://100.64.0.2:9900}"
ESC="${2:-todos}"
DUR="${3:-2m}"
ID="${4:-N1}"
mkdir -p bin resultados
go build -o bin/ ./cmd/cliente ./cmd/metricas

correr() {
  local n="$1" solo="$2" pct="$3"
  echo; echo "=== Escenario ${n} clientes (${solo} solo consulta) — ${DUR} ==="
  ./bin/cliente -gt "$GT" -id "$ID" -clientes "$n" -solo-consulta "$solo" -rpm 20 -pct-consulta "$pct" \
    -duracion "$DUR" -q -salida "resultados/esc${n}"
}

case "$ESC" in
  3) correr 3 2 50 ;;
  5) correr 5 3 0 ;;
  8) correr 8 4 0 ;;
  todos)
    correr 3 2 50
    read -r -p "Reinicia la Persistencia (-reiniciar) y pulsa Enter para el escenario de 5 clientes... " _
    correr 5 3 0
    read -r -p "Reinicia la Persistencia (-reiniciar) y pulsa Enter para el escenario de 8 clientes... " _
    correr 8 4 0 ;;
  *) echo "Escenario inválido: $ESC (usa 3, 5, 8 o todos)"; exit 1 ;;
esac
echo; echo "CSV en ./resultados — consolidar con: ./bin/metricas resultados/esc*_${ID}.csv"
