#!/usr/bin/env bash
# Plan 11B §3.5: gateway/* must depend only on pkg/* and stdlib.
# Specifically gateway/* MUST NOT import service/*, repository/*, or api/*.
# This guards against accidental reverse-dependency creep (e.g. someone
# importing service/cost.Recorder from gateway/llm to "auto-record" tokens —
# that's a layer violation; tokens should be returned in the response and
# the business layer calls Recorder explicitly).

set -euo pipefail

cd "$(dirname "$0")/.."

violations=$(go list -deps ./internal/gateway/... 2>/dev/null \
    | grep -E "^github\.com/aibao/server/internal/(service|repository|api)/" || true)

if [ -n "$violations" ]; then
    echo "FAIL: gateway/ depends on forbidden packages:" >&2
    echo "$violations" >&2
    exit 1
fi

echo "OK: gateway layering check passed"
