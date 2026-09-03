# shipwright — the Go toolchain in a browser tab

Ships in a [bottle](https://github.com/0magnet/bottle) need someone to
build them. shipwright is that someone: the Go compiler, building Go
programs, inside the browser tab they will run in.

The real Go toolchain — `cmd/go` driving `cmd/compile`, `cmd/asm` and
`cmd/link`, built for js/wasm with a stock Go toolchain — running inside a
browser tab against [bottle](https://github.com/0magnet/bottle)'s in-memory
filesystem. An **unmodified `go build`** compiles the fmt/os/runtime closure
from standard-library *source* and links a working wasm binary, entirely
client-side: after the page's assets load, the server is never consulted
again.

    $ go build -o /work/probe.wasm .      # in the tab
    → cmd/go spawns compile/asm/link as child wasm processes, ~90 of them,
      over one shared in-memory disk, and writes a 2.5 MB a.wasm

## Run it

    ./build.sh                 # builds the wasm tools + harvests std source
    go run ./serve/main.go     # :8931, serves the page + the /goproxy passthrough
    # open http://127.0.0.1:8931/            — edit a program, build & run
    #      http://127.0.0.1:8931/probe-gobuild.html — the real `go build`
    #      http://127.0.0.1:8931/probe-gonet.html   — `go build` of a fetched module

`python3 -m http.server 8931` also serves the first two demos; the network
demo needs `serve/main.go` because it must proxy `/goproxy` (see below).

Headless proof (each marker line is a pass signal):

    chromium --headless --no-sandbox --virtual-time-budget=600000 \
      --enable-logging=stderr http://127.0.0.1:8931/probe-gonet.html \
      2>&1 | grep SHIPWRIGHT-NET-MARKER

## Three demos

- **`index.html`** — the minimal shape: one `compile.wasm`, one `link.wasm`,
  a fixed set of prebuilt std archives. Source → `main.a` → `a.wasm` → runs.
  Three wasm processes, one jsfs. This is the toolchain with the orchestration
  taken out — easiest to read.
- **`probe-gobuild.html`** — the whole thing: the real **`cmd/go`** runs
  `go build`. It reads a `go.mod`, computes the build graph, and execs the
  compiler, assembler and linker itself — as child wasm processes — compiling
  the standard library from the seeded source. No prebuilt archives; a real
  build, the way it runs on a disk.
- **`probe-gonet.html`** — the same `go build`, but of a program that imports
  an **external module** (`github.com/pkg/errors`). Its source is not seeded:
  cmd/go fetches it from proxy.golang.org over the same-origin `/goproxy`
  passthrough, extracts the zip into the module cache, and compiles it. This
  is `go install pkg@version` territory — a dependency pulled from the network
  and built, in the tab.

## How it works

The toolchain is pure Go and cross-compiles, so `GOOS=js GOARCH=wasm go build
cmd/compile` (and `cmd/link`, `cmd/asm`, `cmd/go`) just works — those are the
`*.wasm` binaries. What a browser tab lacks is the *operating system* cmd/go
orchestrates against: processes, pipes, a working GOROOT, file locks. bottle
supplies the filesystem and network; a small **GOROOT overlay** (in
`overlay/`, applied with `go build -overlay`) supplies the rest, replacing a
handful of js/wasm standard-library files with versions that call into bottle:

- **`syscall_js.go`** — `StartProcess`/`Wait4` over `globalThis.proc.spawn`:
  a child process *is* another wasm instance sharing this tab's jsfs and vnet.
  A child's stdio fds are pipe ends wired to proc's sinks; `Wait4` blocks on
  the child's exit promise. `WaitStatus` carries the exit code.
- **`fs_js.go` + `pipe_wasm.go`** — real `os.Pipe`, backed by jsfs pipe fds
  (refcounted, so a fd retained for a child still delivers EOF on exit). This
  is how compile's stdout reaches cmd/go's reader.
- **`lp_wasm.go`** — a working `exec.LookPath` (the stock js/wasm one always
  errors), so cmd/go finds the tools it parks under `$GOROOT/pkg/tool`.
- **`filelock_other.go`** — no-op advisory locks; one tab is single-threaded,
  so cmd/go's lockedfile layer has nothing to contend with.
- **`args.go`** — lowers cmd/go's `ExecArgLengthLimit` so it writes long
  compiler invocations to an `@response-file`. wasm_exec packs argv+env into
  one 8 KB window and throws past it; the response file keeps the exec'd argv
  tiny. This is the stock long-args mechanism, tuned to the wasm ceiling — no
  wasm_exec patch, no linker change.

`stdsrc.sh` harvests the standard-library source closure the demo programs
pull in (via `go list -deps`, js/wasm constraints applied) plus the
assembler's `pkg/include` headers, keyed by their path under the tab's
`/goroot`. The harness seeds those, the tools, and a `go.mod`+`main.go` into
jsfs, then runs `go-proc.wasm`. `jsfs.js` and `proc.js` are vendored from
bottle.

**The network path.** cmd/go's module downloads go through `net/http`, which
on js/wasm is backed by the browser's Fetch API. A tab can't call
proxy.golang.org directly — CORS forbids it — but it *can* call its own
origin, so `serve/main.go` reverse-proxies `/goproxy/` to proxy.golang.org and
the tab runs with `GOPROXY=<origin>/goproxy`. Same-origin fetch, no CORS, and
cmd/go is none the wiser. `GOSUMDB=off` since the checksum database is a second
egress the passthrough doesn't (yet) cover. On a host that already fronts a
proxy — or over skywire's skysocks/dmsg for the serverless version — the same
`GOPROXY` trick applies.

## What works / what doesn't

Works: `go build` of a pure-Go program — compiling std from source,
assembling, linking, running the result, all in the tab; **external module
dependencies fetched from the network** over the `/goproxy` passthrough and
compiled; the minimal compile→link→run demo; `go version` / `go env`.

Doesn't, yet — the honest gap list:

1. **Wider std closure.** `stdsrc.sh` seeds the closure the demos need; a
   program (or a fetched dependency) importing, say, `net/http` needs that
   package's source seeded too. Seeding all of `$GOROOT/src` removes the limit
   at the cost of a bigger bundle (cacheable in IndexedDB via jsfs.persist).
2. **Checksum database.** Builds run with `GOSUMDB=off`; verifying downloads
   against sum.golang.org is a second egress the passthrough could forward the
   same way `/goproxy` does.
3. **cgo**: never — it needs a C toolchain. Pure Go only. Cross-compiling
   pure Go to any GOOS/GOARCH from inside the tab should work as-is: the
   toolchain has always been a cross-compiler.

Gaps that used to be here and are now closed: file locking, process spawning,
a source GOROOT, and network egress — the toolchain now fetches, compiles,
and links, all in the tab.

## Files

    build.sh           rebuild everything from a stock Go install
    stdsrc.sh          harvest the std source closure (called by build.sh)
    harvest.sh         prebuilt std archives for index.html (called by build.sh)
    serve/main.go      static server + the /goproxy module-proxy passthrough
    overlay/           the GOROOT overlay — 6 std files + generated overlay.json
    jsfs.js, proc.js   vendored from github.com/0magnet/bottle
    wasm_exec.js       copied from the stock Go install
    hello/main.go      the default program for index.html
    netdemo/main.go    a program with an external dep, for probe-gonet.html
    index.html         compile → link → run
    driver.js          seed + compile + link + run, for index.html
    probe-gobuild.html the real cmd/go running `go build` in the tab
    probe-gonet.html   `go build` of a module fetched over /goproxy
    probe-go.html      a minimal check that cmd/go boots (version / env)
    PROC-DESIGN.md     the process layer's design notes

Built artifacts (`*.wasm`, `pkg/`, `stdsrc.json`, the importcfgs,
`overlay/overlay.json`) are regenerated by `build.sh` and not committed.
