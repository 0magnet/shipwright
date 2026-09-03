# shipwright — the Go toolchain in a browser tab

Ships in a [bottle](https://github.com/0magnet/bottle) need someone to
build them. shipwright is that someone: the Go compiler, building Go
programs, inside the browser tab they will run in.

The real Go compiler and linker — `cmd/compile` and `cmd/link`, built for
js/wasm with a stock Go toolchain, zero patches — running inside a browser
tab against [bottle](https://github.com/0magnet/bottle)'s in-memory
filesystem. A program is compiled, linked and executed entirely client-side:
after the page's assets load, the server is never consulted again.

    source → compile.wasm → main.a → link.wasm → a.wasm → runs, in the tab

## Run it

    ./build.sh                 # builds the wasm tools + harvests std archives
    python3 -m http.server 8931
    # open http://127.0.0.1:8931/  — edit the program, press build & run

Headless proof (the marker line is the pass signal):

    chromium --headless --no-sandbox --virtual-time-budget=300000 \
      --enable-logging=stderr --screenshot=/tmp/s.png http://127.0.0.1:8931/ \
      2>&1 | grep SHIPWRIGHT-MARKER

## How it works

- `compile.wasm`, `link.wasm`, `go.wasm`: `GOOS=js GOARCH=wasm go build
  cmd/compile` (and cmd/link, cmd/go). They compile unmodified — the Go
  toolchain is pure Go and always has been.
- `harvest.sh`: warms a js/wasm build of `hello/`, then uses
  `go list -export -deps` to pull the 57 std package archives (fmt/os/runtime
  closure, ~22 MB) out of the host build cache into `pkg/`, and writes the
  `importcfg` / `importcfg.link` the tools will read in the tab.
- `driver.js`: seeds bottle's jsfs with the archives, importcfgs and the
  page's editable `main.go`; instantiates `compile.wasm` with
  `argv = [compile -p main -pack -importcfg /importcfg -o /work/main.a
  /src/main.go]` (wasm_exec passes argv/env); then `link.wasm`; then runs the
  freshly linked `/work/a.wasm` as a third instance. Three processes, one
  in-memory disk — exactly the shape bottle exists to provide.
- Tool stdout/stderr land in the page log via `jsfs.stdio`.

`probe-go.html` goes further: the real **`cmd/go` itself boots in the tab** —
`go version` and `go env` run and exit 0. `go list` is where it stops today
(see gaps).

## What works / what doesn't

Works: compile, link, run, any program whose imports lie inside the seeded
archive set; editing and rebuilding without reload; `go version` / `go env`
from the real cmd/go.

Doesn't, yet — the honest gap list, in dependency order:

1. **File locking.** `go list`/`go build` livelock: cmd/go's lockedfile
   layer needs flock-like semantics jsfs doesn't provide, and its fallback
   spin-waits. jsfs growing an advisory-lock table (or honoring
   O_CREATE|O_EXCL atomically with real EEXIST, which it may only partly do)
   is the first unlock.
2. **Processes.** `go build` orchestrates by exec()ing compile/link. The fix
   is a bottle `proc` layer: `syscall.StartProcess` under js/wasm →
   instantiate another wasm module (the toolchain binaries parked in jsfs at
   `$GOROOT/pkg/tool/js_wasm/`) sharing the same jsfs/vnet, wait = the
   instance's exit promise. See PROC-DESIGN.md.
3. **GOROOT.** Full std *source* seeded into jsfs (tens of MB, cacheable via
   jsfs.persist/IndexedDB) so cmd/go can compile std itself, or std archives
   pre-seeded per-release as done here.
4. **Network.** `go install pkg@version` needs GOPROXY egress. Browser CORS
   blocks proxy.golang.org, so: a same-origin `/goproxy/*` passthrough
   (five lines on any host server), or skywire's skysocks/dmsg channels for
   the serverless version.
5. **cgo**: never (needs a C toolchain); pure Go only. Cross-compiling pure
   Go to any GOOS/GOARCH from inside the tab should work as-is once cmd/go
   runs — the toolchain has always been a cross-compiler.

## Files

    bottle/            the OS layer (cloned; jsfs.js is what's loaded here)
    build.sh           rebuild everything from a stock Go install
    harvest.sh         std archive + importcfg harvest (called by build.sh)
    compile.wasm       cmd/compile for js/wasm      (~51 MB)
    link.wasm          cmd/link for js/wasm         (~12 MB)
    go.wasm            cmd/go for js/wasm           (~30 MB)
    pkg/               57 std package archives      (~22 MB)
    importcfg[.link]   package path → archive maps
    hello/main.go      the default program
    index.html         the editable demo
    driver.js          seed + compile + link + run
    probe-go.html      how far the real cmd/go gets in-tab
