#!/usr/bin/env bash
# OS collector fixture tests (no network).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="${ROOT}/sql-optima-os-collector.sh"
FIXTURES="${ROOT}/test/fixtures/proc"
EXPECTED="${ROOT}/test/expected.json"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

pass() {
  echo "PASS: $*"
}

# --- CPU delta unit test (matches Go cpuDeltaPercents) ---
test_cpu_delta() {
  local script_dir
  script_dir="${ROOT}"
  # shellcheck source=/dev/null
  source "${SCRIPT}"

  local out
  out=$(cpu_delta_percents "1000 100 500 8000 200 50 30 10" "1100 110 550 8100 220 55 33 11")
  read -r u s i io <<<"$out"
  local sum
  sum=$(awk -v u="$u" -v s="$s" -v i="$i" -v io="$io" 'BEGIN { printf "%.2f", u+s+i+io }')
  awk -v sum="$sum" -v u="$u" 'BEGIN {
    if (sum < 99 || sum > 101) { exit 1 }
    if (u <= 0) { exit 1 }
  }' || fail "cpu_delta_percents sum=${sum} u=${u}"

  out=$(cpu_delta_percents "10 10 10 80 0 0 0 0" "10 10 10 80 0 0 0 0")
  read -r u s i io <<<"$out"
  [[ "$u" == "0" && "$s" == "0" && "$i" == "100" && "$io" == "0" ]] \
    || fail "cpu_delta zero delta got u=${u} s=${s} i=${i} io=${io}"

  pass "cpu_delta_percents"
}

# --- Full collect against fixtures ---
test_collect_fixtures() {
  [[ -x "${SCRIPT}" ]] || chmod +x "${SCRIPT}"

  local got
  got=$(PROC_ROOT="${FIXTURES}" \
    SQL_OPTIMA_COLLECT_ONLY=1 \
    SQL_OPTIMA_SKIP_CPU_SAMPLE=1 \
    SQL_OPTIMA_BACKEND_URL="http://127.0.0.1:9/ignore" \
    SQL_OPTIMA_API_KEY="test" \
    SQL_OPTIMA_INSTANCE_NAME="test-instance" \
    "${SCRIPT}" -interval 30s 2>/dev/null)

  local keys=(
    hostname instance_name
    total_bytes available_bytes used_bytes free_bytes
    cached_bytes buffers_bytes shared_bytes slab_bytes
    swap_total_bytes swap_used_bytes swap_free_bytes
    dirty_bytes writeback_bytes
    cpu_user_pct cpu_system_pct cpu_idle_pct cpu_iowait_pct
    load_1m load_5m load_15m cpu_cores
    postgres_rss_bytes postgres_vsz_bytes backend_count
  )
  local key gv ev
  for key in "${keys[@]}"; do
    gv=$(echo "$got" | jq -r --arg k "$key" '.[$k]')
    ev=$(jq -r --arg k "$key" '.[$k]' "${EXPECTED}")
    if [[ "$gv" != "$ev" ]]; then
      echo "expected:" >&2
      jq . "${EXPECTED}" >&2
      echo "got:" >&2
      echo "$got" | jq . >&2
      fail "field ${key}: got ${gv}, want ${ev}"
    fi
  done
  pass "collect_metrics fixtures"
}

test_cpu_delta
test_collect_fixtures
echo "All os_collector tests passed."
