#!/usr/bin/env bash
# Checks the generation pipeline against synthetic specs that use features the production
# spec does not (yet): typed responses replacing untyped ones, ErrorResponse bodies on 4xx/5xx,
# 201 and 204 responses, vendor +json types, image/svg+xml, image/png, text/markdown,
# application/pdf, JSON-or-Markdown responses and non-JSON request bodies.
#
#   scripts/test-fixture.sh
#
# For each fixture, the SDK is generated into a temporary module together with the
# hand-written sources, coverage_test.go and the fixture's own test, then vetted and tested.
# Runs in Docker like scripts/generate.sh unless FREEASTRO_NATIVE=1.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [ -z "${FREEASTRO_IN_DOCKER:-}" ] && [ "${FREEASTRO_NATIVE:-}" != 1 ]; then
  exec "$ROOT/scripts/docker.sh" -- scripts/test-fixture.sh
fi

FIXTURES=(
  "tests/fixtures/synthetic-spec.json:testdata/fixtures/synthetic_test.go"
  "testdata/fixtures/extra-spec.json:testdata/fixtures/extra_test.go"
)

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

for entry in "${FIXTURES[@]}"; do
  spec="${entry%%:*}"
  test_file="${entry#*:}"
  module="$work/$(basename "$spec" .json)"
  echo "== $spec"
  mkdir -p "$module/api" "$module/spec"
  cp "$ROOT/go.mod" "$ROOT/go.sum" "$ROOT/coverage_test.go" "$ROOT/$test_file" "$module/"
  for f in "$ROOT"/*.go; do
    case "$f" in *_gen.go | *_test.go) continue ;; esac
    cp "$f" "$module/"
  done
  cp "$ROOT/api/doc.go" "$module/api/"
  cp "$ROOT/$spec" "$module/spec/freeastro.json"
  "$ROOT/scripts/generate.sh" --spec "$ROOT/$spec" --target "$module"
  (cd "$module" && go vet ./... && go test -count=1 ./...)
done
