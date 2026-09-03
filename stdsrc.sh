#!/bin/sh
# Generate stdsrc.json: the standard-library *source* closure the in-tab cmd/go
# compiles to build a fmt-importing program, keyed by the path it lives at under
# the tab's GOROOT (/goroot). This is what makes probe-gobuild.html a real build
# — cmd/go compiles fmt/os/runtime/… from source, exactly as it would on disk,
# rather than reading prebuilt archives.
#
# By default the closure is whatever `go list -deps` of the demo programs pulls
# in (js/wasm build constraints applied, so only the files that GOOS=js
# GOARCH=wasm actually compiles are seeded): hello/ for the fmt/os/runtime
# base, and netdemo/ so the standard-library packages its external dependency
# imports are present when cmd/go fetches and compiles it in the tab. asm also
# needs the compiler's include headers. Only .Standard packages are seeded —
# the external module itself is fetched at runtime over /goproxy, not seeded.
#
#   ./stdsrc.sh          the demo closure (~5.9 MB) — enough for the demos
#   ./stdsrc.sh all      the whole standard library (~30 MB) — build anything
#
# "all" removes the "imports must lie in the seeded closure" limit: any pure-Go
# program, and any module fetched over /goproxy, compiles because every std
# package it could import is already on disk in the tab.
set -eu
cd "$(dirname "$0")"
GOROOT=$(go env GOROOT)
MODE="${1:-demo}"

echo "shipwright: harvesting std source (this is the in-tab GOROOT) [mode: $MODE]…"

# host source paths, keyed later by their path under /goroot. Two modes:
#   all  — every .go/.s/.h under $GOROOT/src, minus tests and testdata.
#   demo — only the packages the demo programs' build closures reach.
# Both add the pkg/include headers the assembler reads (textflag.h &c).
{
  if [ "$MODE" = all ]; then
    find "$GOROOT/src" \( -name '*.go' -o -name '*.s' -o -name '*.h' \) \
      -not -name '*_test.go' -not -path '*/testdata/*'
  else
    for m in hello netdemo; do
      ( cd "$m" && GOOS=js GOARCH=wasm GOFLAGS=-mod=mod go list -deps -json . ) | \
        jq -r 'select(.Standard==true) | .Dir as $d
               | ((.GoFiles // []) + (.SFiles // []) + (.HFiles // []))[]
               | $d + "/" + .'
    done
  fi
  ls "$GOROOT"/pkg/include/*.h
} | sort -u > .stdsrc.list

# Encode the listed files as {"/goroot/<rel>": "<base64>"} with a small Go
# helper — this is a Go project, so the toolchain that builds it is the only
# thing it needs to build itself. The helper lives in a temp dir: the go tool
# ignores files whose names begin with '.', so it can't sit in this directory
# under a dotted name, and a plain name here would look like stray package src.
gendir=$(mktemp -d)
trap 'rm -rf "$gendir" .stdsrc.list' EXIT
cat > "$gendir/main.go" <<'GO'
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	goroot, listfile, out := os.Args[1], os.Args[2], os.Args[3]
	data, err := os.ReadFile(listfile)
	if err != nil {
		panic(err)
	}
	files := map[string]string{}
	for _, host := range strings.Fields(string(data)) {
		b, err := os.ReadFile(host)
		if err != nil {
			continue // a directory or unreadable entry: skip
		}
		rel, err := filepath.Rel(goroot, host)
		if err != nil {
			continue
		}
		files["/goroot/"+rel] = base64.StdEncoding.EncodeToString(b)
	}
	enc, err := json.Marshal(files)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(out, enc, 0o644); err != nil {
		panic(err)
	}
	fmt.Printf("shipwright: seeded %d std source files into stdsrc.json\n", len(files))
}
GO
go run "$gendir/main.go" "$GOROOT" .stdsrc.list stdsrc.json
