#!/bin/sh
# Rebuild the shipwright prototype from a stock Go installation.
#
# Two toolchains are built here:
#   *.wasm       — the stock cmd/compile, cmd/link, cmd/go (zero patches),
#                  for the compile→link→run demo (index.html).
#   *-proc.wasm  — the same tools built with -overlay overlay/overlay.json,
#                  which teaches the js/wasm runtime to spawn processes, pipe
#                  their stdio, run a real GOROOT, take a no-op file lock and
#                  size the build parallelism the tab can actually use.
#                  With those, the *real cmd/go* runs `go build` in the tab
#                  (probe-gobuild.html).
#
# Everything this writes is regenerable and .gitignore'd — the repo is source.
set -eu
cd "$(dirname "$0")"
GOROOT=$(go env GOROOT)

echo "shipwright: building the stock js/wasm tools (compile/link/go)…"
GOOS=js GOARCH=wasm go build -o compile.wasm cmd/compile
GOOS=js GOARCH=wasm go build -o link.wasm cmd/link
GOOS=js GOARCH=wasm go build -o go.wasm cmd/go
cp "$GOROOT/lib/wasm/wasm_exec.js" .

# --- the overlay tools -------------------------------------------------------
# overlay.json maps stock GOROOT files to this repo's overlay/ copies. Its
# paths must be absolute and are machine-specific, so it is generated here
# rather than committed.
echo "shipwright: generating overlay/overlay.json for GOROOT=$GOROOT…"
here=$(pwd)/overlay
cat > overlay/overlay.json <<EOF
{
  "Replace": {
    "$GOROOT/src/syscall/fs_js.go": "$here/fs_js.go",
    "$GOROOT/src/syscall/syscall_js.go": "$here/syscall_js.go",
    "$GOROOT/src/os/pipe_wasm.go": "$here/pipe_wasm.go",
    "$GOROOT/src/cmd/go/internal/lockedfile/internal/filelock/filelock_other.go": "$here/filelock_other.go",
    "$GOROOT/src/os/exec/lp_wasm.go": "$here/lp_wasm.go",
    "$GOROOT/src/cmd/internal/sys/args.go": "$here/args.go",
    "$GOROOT/src/cmd/go/internal/cfg/buildp_js.go": "$here/buildp_js.go"
  }
}
EOF

echo "shipwright: building the overlay tools (go/compile/link/asm/vet -proc.wasm)…"
GOOS=js GOARCH=wasm go build -overlay overlay/overlay.json -o go-proc.wasm      cmd/go
GOOS=js GOARCH=wasm go build -overlay overlay/overlay.json -o compile-proc.wasm cmd/compile
GOOS=js GOARCH=wasm go build -overlay overlay/overlay.json -o link-proc.wasm    cmd/link
GOOS=js GOARCH=wasm go build -overlay overlay/overlay.json -o asm-proc.wasm     cmd/asm
# vet too, so the in-tab cmd/go can run `go test` (which invokes vet).
GOOS=js GOARCH=wasm go build -overlay overlay/overlay.json -o vet-proc.wasm     cmd/vet

# --- std source for the in-tab cmd/go ---------------------------------------
./stdsrc.sh

# --- prebuilt std archives for the compile/link demo ------------------------
( cd hello && GOFLAGS= GOWORK=off GOOS=js GOARCH=wasm go build -o /dev/null . )
( cd hello && GOFLAGS= GOWORK=off GOOS=js GOARCH=wasm \
  go list -export -deps -f '{{if .Export}}{{.ImportPath}}={{.Export}}{{end}}' . ) > deps.txt
./harvest.sh
( echo '{"files":['; find pkg -name '*.a' | sort | sed 's/.*/"&",/' | sed '$ s/,$//'; echo ']}' ) > manifest.json

echo "shipwright: ready — go run ./serve/main.go and open http://127.0.0.1:8931/"
echo "  index.html         compile → link → run a program in the tab"
echo "  probe-gobuild.html the real cmd/go running 'go build' in the tab"
echo "  probe-gonet.html   'go build' of a module fetched over /goproxy"
# jsfs.js, fsbridge.js and proc.js are vendored from github.com/0magnet/bottle;
# refresh with:
#   git clone git@github.com:0magnet/bottle.git && \
#     cp bottle/jsfs.js bottle/fsbridge.js bottle/proc.js .
