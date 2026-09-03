#!/bin/sh
# Rebuild the gotab prototype from a stock Go installation.
set -eu
cd "$(dirname "$0")"
GOOS=js GOARCH=wasm go build -o compile.wasm cmd/compile
GOOS=js GOARCH=wasm go build -o link.wasm cmd/link
GOOS=js GOARCH=wasm go build -o go.wasm cmd/go
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" .
( cd hello && GOFLAGS= GOWORK=off GOOS=js GOARCH=wasm go build -o /dev/null . )
( cd hello && GOFLAGS= GOWORK=off GOOS=js GOARCH=wasm \
  go list -export -deps -f '{{if .Export}}{{.ImportPath}}={{.Export}}{{end}}' . ) > deps.txt
./harvest.sh
( echo '{"files":['; find pkg -name '*.a' | sort | sed 's/.*/"&",/' | sed '$ s/,$//'; echo ']}' ) > manifest.json
echo "gotab: ready — python3 -m http.server 8931 and open http://127.0.0.1:8931/"
# jsfs.js is vendored from github.com/0magnet/bottle; refresh with:
#   git clone git@github.com:0magnet/bottle.git && cp bottle/jsfs.js .
