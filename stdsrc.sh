#!/bin/sh
# Generate stdsrc.json: the standard-library *source* closure the in-tab cmd/go
# compiles to build a fmt-importing program, keyed by the path it lives at under
# the tab's GOROOT (/goroot). This is what makes probe-gobuild.html a real build
# — cmd/go compiles fmt/os/runtime/… from source, exactly as it would on disk,
# rather than reading prebuilt archives.
#
# The closure is whatever `go list -deps` of hello/ pulls in (js/wasm build
# constraints applied, so only the files that GOOS=js GOARCH=wasm actually
# compiles are seeded). asm also needs the compiler's include headers.
set -eu
cd "$(dirname "$0")"
GOROOT=$(go env GOROOT)

echo "shipwright: harvesting std source closure (this is the in-tab GOROOT)…"

# host source paths: every .go/.s/.h of every std package in hello's build
# closure, plus the pkg/include headers the assembler reads (textflag.h &c).
{
  ( cd hello && GOOS=js GOARCH=wasm go list -deps -json . ) | \
    jq -r 'select(.Standard==true) | .Dir as $d
           | ((.GoFiles // []) + (.SFiles // []) + (.HFiles // []))[]
           | $d + "/" + .'
  ls "$GOROOT"/pkg/include/*.h
} | sort -u > .stdsrc.list

python3 - "$GOROOT" .stdsrc.list stdsrc.json <<'PY'
import base64, json, os, sys
goroot, listfile, out = sys.argv[1], sys.argv[2], sys.argv[3]
files = {}
for host in open(listfile).read().split():
    if not host or not os.path.isfile(host):
        continue
    # host is under $GOROOT; the tab mounts GOROOT at /goroot.
    rel = os.path.relpath(host, goroot)
    key = "/goroot/" + rel
    with open(host, "rb") as f:
        files[key] = base64.b64encode(f.read()).decode("ascii")
json.dump(files, open(out, "w"))
print("shipwright: seeded %d std source files into stdsrc.json" % len(files))
PY
rm -f .stdsrc.list
