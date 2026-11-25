#!/bin/sh
# Docker の golang イメージで go を動かす（ホストに Go を入れない）。
#   scripts/go.sh test ./...
#   scripts/go.sh build -o bin/ ./cmd/...
set -eu
cd "$(dirname "$0")/.."
CACHE=${DYNAFW_GOCACHE:-$HOME/.cache/recdyna-go}
mkdir -p "$CACHE/build" "$CACHE/mod"
exec docker run --rm -u "$(id -u):$(id -g)" \
    -e GOCACHE=/cache/build -e GOMODCACHE=/cache/mod -e GOPATH=/tmp/gopath -e CGO_ENABLED=0 \
    ${GOOS:+-e "GOOS=$GOOS"} ${GOARCH:+-e "GOARCH=$GOARCH"} \
    -v "$CACHE:/cache" -v "$PWD:/src" -w /src golang:1.26-bookworm go "$@"
