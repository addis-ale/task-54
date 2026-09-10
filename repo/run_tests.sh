#!/usr/bin/env bash

set -uo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOG_DIR="${ROOT_DIR}/test_reports"
mkdir -p "${LOG_DIR}"

TOTAL_TESTS=0
TOTAL_PASSES=0
TOTAL_FAILURES=0
SUITE_FAILURES=0

USE_DOCKER=false

# Attempt Docker build; fall back to native Go if Docker is unavailable.
if command -v docker &> /dev/null; then
  echo "Building test image..."
  TESTER_IMAGE="clinic-test-runner"
  if docker build --target tester -t "${TESTER_IMAGE}" -q "${ROOT_DIR}" > /dev/null 2>&1; then
    echo "Test image ready."
    USE_DOCKER=true
  else
    echo "Docker build failed, falling back to native Go test runner."
  fi
else
  echo "Docker not found, using native Go test runner."
fi

run_suite() {
  local suite_name="$1"
  local pkg_pattern="$2"
  local log_file="${LOG_DIR}/${suite_name}.log"

  echo ""
  echo "============================================================"
  echo "Running ${suite_name} (${pkg_pattern})"
  echo "Log: ${log_file}"
  echo "============================================================"

  if [[ "${USE_DOCKER}" == "true" ]]; then
    if docker run --rm "${TESTER_IMAGE}" "${pkg_pattern}" 2>&1 | tee "${log_file}"; then
      suite_exit=0
    else
      suite_exit=$?
    fi
  else
    if (cd "${ROOT_DIR}" && go test "${pkg_pattern}" -v -count=1) 2>&1 | tee "${log_file}"; then
      suite_exit=0
    else
      suite_exit=$?
    fi
  fi

  local suite_total
  local suite_pass
  local suite_fail

  suite_total=$(grep -cE "^--- (PASS|FAIL):" "${log_file}" || true)
  suite_pass=$(grep -cE "^--- PASS:" "${log_file}" || true)
  suite_fail=$(grep -cE "^--- FAIL:" "${log_file}" || true)

  TOTAL_TESTS=$((TOTAL_TESTS + suite_total))
  TOTAL_PASSES=$((TOTAL_PASSES + suite_pass))
  TOTAL_FAILURES=$((TOTAL_FAILURES + suite_fail))

  if [[ ${suite_exit} -ne 0 ]]; then
    SUITE_FAILURES=$((SUITE_FAILURES + 1))
    echo "[${suite_name}] RESULT: FAILED"
  else
    echo "[${suite_name}] RESULT: PASSED"
  fi

  echo "[${suite_name}] Cases: total=${suite_total}, pass=${suite_pass}, fail=${suite_fail}"
}

echo "Starting Clinic Administration Suite verification run..."
echo "Timestamp: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"

run_suite "unit_tests" "./unit_tests/..."
run_suite "API_tests" "./API_tests/..."

echo ""
echo "======================= FINAL SUMMARY ======================="
echo "Total Tests : ${TOTAL_TESTS}"
echo "Passes      : ${TOTAL_PASSES}"
echo "Failures    : ${TOTAL_FAILURES}"
echo "Suite Errors: ${SUITE_FAILURES}"
echo "============================================================"

if [[ ${SUITE_FAILURES} -ne 0 || ${TOTAL_FAILURES} -ne 0 ]]; then
  echo "Verification run finished with failures."
  exit 1
fi

echo "Verification run finished successfully."
exit 0
