#!/usr/bin/env bash
# Runs a shell command inside the pinned Go Docker image against a copy of this repository.
#
#   scripts/docker.sh [--out PATH]... -- COMMAND...
#
# The working tree (without .git) is streamed into the container with tar rather than
# bind-mounted, so it works with any Docker setup (Docker Desktop, Colima, CI) regardless of
# which host folders are shared with the VM. Each --out PATH is copied back to the working
# tree afterwards (replacing the local copy). Module and build caches persist in the Docker
# volumes freeastro-go-sdk-gomod and freeastro-go-sdk-gobuild. Extra `docker run` flags can be
# passed in DOCKER_RUN_ARGS (for example "--network NAME -e PRISM_URL=...").
set -euo pipefail

GO_IMAGE="${GO_IMAGE:-golang:1.26.11}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

outs=()
while [ $# -gt 0 ]; do
  case "$1" in
    --out) outs+=("$2"); shift 2 ;;
    --) shift; break ;;
    *) break ;;
  esac
done
[ $# -gt 0 ] || { echo "usage: scripts/docker.sh [--out PATH]... -- COMMAND..." >&2; exit 2; }

tar_flags=()
if tar --version 2>/dev/null | grep -qi bsdtar; then
  tar_flags=(--no-xattrs --no-mac-metadata)
fi
export COPYFILE_DISABLE=1

inner="set -e; mkdir -p /src; cd /src; tar -x -f - 2>/dev/null; git config --global --add safe.directory /src >/dev/null 2>&1 || true; ( $* ) 1>&2"
if [ ${#outs[@]} -gt 0 ]; then
  inner="$inner; tar -c -f - ${outs[*]}"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# The container's stdout is buffered to a file and only unpacked when the command succeeded,
# so a failed run never replaces the local copies of the --out paths.
tar ${tar_flags[@]+"${tar_flags[@]}"} -c -f - -C "$ROOT" --exclude ./.git --exclude ./.scratch . |
  docker run --rm -i \
    -e GOFLAGS="${GOFLAGS:-}" -e FREEASTRO_IN_DOCKER=1 ${DOCKER_RUN_ARGS:-} \
    -v freeastro-go-sdk-gomod:/go/pkg/mod \
    -v freeastro-go-sdk-gobuild:/root/.cache/go-build \
    "$GO_IMAGE" sh -c "$inner" >"$tmp/out.tar"

if [ ${#outs[@]} -gt 0 ]; then
  mkdir "$tmp/out"
  tar -x -f "$tmp/out.tar" -C "$tmp/out"
  for path in "${outs[@]}"; do
    rm -rf "${ROOT:?}/$path"
    if [ -e "$tmp/out/$path" ]; then
      mkdir -p "$(dirname "$ROOT/$path")"
      mv "$tmp/out/$path" "$ROOT/$path"
    fi
  done
fi
