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

**Live:** the offline demos run in your browser at
<https://0magnet.github.io/shipwright/> (published by `.github/workflows/pages.yml`).
The network demo needs a server for its `/goproxy` passthrough, so run that one
locally.

## Run it

    ./build.sh                 # builds the wasm tools + harvests std source
    go run ./serve/main.go     # :8931, serves the page + the /goproxy passthrough
    # open http://127.0.0.1:8931/            — edit a program, build & run
    #      http://127.0.0.1:8931/probe-gobuild.html — the real `go build`
    #      http://127.0.0.1:8931/probe-gonet.html   — `go build` of a fetched module

`serve/main.go` is the only server involved: static files plus the `/goproxy`
passthrough the network demo needs (see below). It's a std-only Go program —
the toolchain that builds shipwright is all it takes to run it.

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

`stdsrc.sh` harvests the standard-library source into `stdsrc.json`, keyed by
path under the tab's `/goroot`, plus the assembler's `pkg/include` headers.
Two modes: the default seeds just the closure the demo programs reach (~5.9 MB,
via `go list -deps`); `./stdsrc.sh all` seeds the whole standard library
(~97 MB) so *any* pure-Go program — or any module fetched over `/goproxy` —
compiles, its imports guaranteed present. The harness seeds that, the tools,
and a `go.mod`+`main.go` into jsfs, then runs `go-proc.wasm`. `jsfs.js` and
`proc.js` are vendored from bottle.

**The network path.** cmd/go's module downloads go through `net/http`, which
on js/wasm is backed by the browser's Fetch API. A tab can't call
proxy.golang.org directly — CORS forbids it — but it *can* call its own
origin, so `serve/main.go` reverse-proxies `/goproxy/` to proxy.golang.org and
the tab runs with `GOPROXY=<origin>/goproxy`. Same-origin fetch, no CORS, and
cmd/go is none the wiser. The same server mirrors the checksum database at
`/goproxy/sumdb/sum.golang.org/` (it answers `/supported` and forwards the
rest to sum.golang.org), so the build runs with the stock `GOSUMDB` and every
download is verified — the second egress, closed the same way as the first. On
a host that already fronts a proxy — or over skywire's skysocks/dmsg for the
serverless version — the same `GOPROXY` trick applies.

## What works

`go build` of any pure-Go program, in the tab: compiling the standard library
from source, assembling, linking, and running the result. External module
dependencies are **fetched from the network** over the `/goproxy` passthrough
and **checksum-verified** through the mirrored sumdb. With `./stdsrc.sh all`
the std closure is unbounded — anything pure-Go compiles. Also the minimal
compile→link→run demo, and `go version` / `go env`.

This scales to a real program: `probe-websh.html` runs
`go install github.com/0magnet/websh/cmd/websh@main` in the tab (after
`./stdsrc.sh all`), fetching websh's whole module graph — a dozen modules,
including u-root's 52 MB zip — over `/goproxy`, verifying every checksum, and
compiling ~150 packages into the same 13 MB `websh.wasm` a host build produces.
(The proxy follows upstream redirects server-side; proxy.golang.org serves big
zips as a 302 to a CDN on another origin, which the tab's fetch couldn't chase
across origins.)

Cross-compiling pure Go to any GOOS/GOARCH from inside the tab should work
as-is — the toolchain has always been a cross-compiler.

Closed over this line of work: file locking, process spawning, a source
GOROOT, network egress, and checksum verification — the toolchain fetches,
verifies, compiles, and links, all in the tab.

## The one real gap: cgo

cgo needs a **C toolchain** — a C compiler and system headers — to build the C
half of a cgo package, and a browser tab has none. So today it's pure Go only
(and upstream has never supported cgo on `GOOS=js` regardless).

Is it *impossible*? No — just a second toolchain's worth of work. clang has a
wasm build, and a wasi-libc gives it headers and a C runtime; parked in jsfs
and driven through the same `proc` layer that runs `compile`/`link`, with `CC`
pointed at it, cgo's C steps could run exactly the way the Go ones do now. The
hard parts are real (a wasm clang + linker that target wasm *and* run on it, a
seeded sysroot, matching Go's cgo ABI), so it's a project, not a patch — but
the same shape as everything already here, not a wall.

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
    probe-websh.html   `go install` of github.com/0magnet/websh in the tab
    probe-go.html      a minimal check that cmd/go boots (version / env)
    PROC-DESIGN.md     the process layer's design notes

Built artifacts (`*.wasm`, `pkg/`, `stdsrc.json`, the importcfgs,
`overlay/overlay.json`) are regenerated by `build.sh` and not committed.
