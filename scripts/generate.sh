#!/usr/bin/env bash
# Regenerates the SDK from the vendored OpenAPI document (spec/freeastro.json).
#
#   scripts/generate.sh                        rewrite the generated files
#   scripts/generate.sh --check                fail if the generated files are out of date
#   scripts/generate.sh --spec FILE --target DIR
#                                              generate from another spec into DIR (used by
#                                              scripts/test-fixture.sh)
#
# Three steps, all pinned:
#   1. internal/cmd/specprep applies two small deterministic rewrites to a copy of the spec
#      (see its package comment: validation-only unions dropped, untyped consts typed);
#   2. oapi-codegen (version in internal/tools/go.mod, config in internal/codegen/) writes the
#      types and low-level client to api/api.gen.go;
#   3. internal/cmd/genresources writes the idiomatic resource layer (resources_gen.go) and
#      the operation table (operations_gen.go).
#
# Runs inside the pinned Go Docker image (scripts/docker.sh) so Go need not be installed.
# Set FREEASTRO_NATIVE=1 to use the local Go toolchain instead (CI does, with the same version).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GENERATED=(api/api.gen.go resources_gen.go operations_gen.go)

check=0
spec="spec/freeastro.json"
target=""
args=("$@")
while [ $# -gt 0 ]; do
  case "$1" in
    --check) check=1; shift ;;
    --spec) spec="$2"; shift 2 ;;
    --target) target="$2"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

if [ -z "${FREEASTRO_IN_DOCKER:-}" ] && [ "${FREEASTRO_NATIVE:-}" != 1 ]; then
  if [ "$check" = 1 ] || [ -n "$target" ]; then
    exec "$ROOT/scripts/docker.sh" -- scripts/generate.sh ${args[@]+"${args[@]}"}
  fi
  outs=()
  for f in "${GENERATED[@]}"; do outs+=(--out "$f"); done
  exec "$ROOT/scripts/docker.sh" "${outs[@]}" -- scripts/generate.sh ${args[@]+"${args[@]}"}
fi

abs() { case "$1" in /*) echo "$1" ;; *) echo "$ROOT/$1" ;; esac; }

generate_into() {
  local spec_file="$1" out="$2" prepared
  mkdir -p "$out/api"
  prepared="$(mktemp -d)/spec.json"
  (cd "$ROOT" && go run ./internal/cmd/specprep "$spec_file" "$prepared")
  (cd "$ROOT/internal/tools" &&
    go tool oapi-codegen -config "$ROOT/internal/codegen/oapi-codegen.yaml" -o "$out/api/api.gen.go" "$prepared")
  (cd "$ROOT" && go run ./internal/cmd/genresources -spec "$spec_file" -api "$out/api/api.gen.go" -out "$out")
}

spec="$(abs "$spec")"

if [ "$check" = 1 ]; then
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  generate_into "$spec" "$tmp"
  stale=()
  for f in "${GENERATED[@]}"; do
    cmp -s "$tmp/$f" "$ROOT/$f" || stale+=("$f")
  done
  if [ ${#stale[@]} -gt 0 ]; then
    echo "Generated files are out of date: ${stale[*]}. Run scripts/generate.sh." >&2
    for f in "${stale[@]}"; do diff -u "$ROOT/$f" "$tmp/$f" | head -40 >&2 || true; done
    exit 1
  fi
  echo "Generated files are up to date."
  exit 0
fi

generate_into "$spec" "$(abs "${target:-.}")"
