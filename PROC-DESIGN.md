# bottle `proc`: processes for wasm tabs (design)

The missing third leg. bottle fakes the filesystem (jsfs) and the network
(vnet); cmd/go — and every Unix-shaped orchestrator — also needs fork/exec.
A tab has an exact analog: instantiating another wasm module IS spawning a
process. websh already demonstrates the degenerate case by hand.

## Shape

Page global `proc` (proc.js), plus a Go adapter mirroring vnet's:

    proc.spawn({argv, env, cwd, stdio}) -> {pid, exited: Promise<code>}

- **Binary resolution**: argv[0] resolved against jsfs (PATH walk). The file
  is the wasm bytes: `WebAssembly.instantiate(jsfs bytes)`; a compile cache
  (`WebAssembly.Module` keyed by path+mtime) makes repeat spawns cheap.
- **The child shares jsfs** — that is the point (cmd/go writes $WORK, the
  child compiler reads it). vnet likewise. cwd/env/argv are per-instance via
  wasm_exec's `go.argv`/`go.env` + `process.chdir` before start.
- **stdio**: jsfs.stdio is page-global today; per-process it becomes a table
  keyed by instance, parent's fds inherited by default, pipes = in-memory
  byte queues (vnet already has the pipe primitive).
- **wait/exit**: wasm_exec's `go.exit(code)` resolves the `exited` promise.
  `os/exec.Wait` maps directly; ProcessState carries the code.
- **Go side** (`bottle/proc` package, js build tag): a linkname/monkey layer
  is NOT needed — os/exec on js currently fails in `syscall.StartProcess`
  with ENOSYS. Two options, in order of invasiveness:
  1. **Fork-patch `syscall_js.go`** in a toolchain overlay: implement
     StartProcess/Wait4 over `proc.spawn`. Cleanest result — unmodified
     cmd/go binaries just work — but requires building the toolchain wasm
     binaries against the patched GOROOT (a build.sh detail, not a runtime
     one, and a candidate upstream proposal later).
  2. An exec-handler shim inside the program (works for OUR programs like
     websh applets, not for stock cmd/go).
  Route 1 is the one that makes `go build` orchestrate for real.
- **Threads/blocking**: everything stays async on the one JS thread;
  "concurrent" processes interleave exactly like goroutines already do.

## Order of work

1. jsfs advisory locks (unblocks `go list` even before proc exists).
2. proc.js + spawn/wait + stdio table; websh `exec` applet to exercise it.
3. GOROOT overlay patch for syscall StartProcess/Wait4; rebuild go.wasm;
   `go build ./...` in-tab against seeded std archives.
4. `/goproxy` passthrough (or skysocks channel) → `go install pkg@ver`.

## Status (updated)

Steps reordered by what the empirical probe showed: `go list` hangs on the
toolID `compile -V=full` exec, before any file lock — so **proc is the first
unlock, not file-locking.**

- **Done.** proc.js + the `bottle/proc` Go adapter, shipped in
  [bottle](https://github.com/0magnet/bottle) (`example/proc`). A parent wasm
  spawns a child sharing the page fs, pipes stdin, and reads its stdout and
  exit code. This is step 2 of the original order, standing on its own.
- **Next.** The GOROOT overlay: patch `syscall.StartProcess`/`Wait4`
  (two ENOSYS stubs in `syscall/syscall_js.go`) to call `proc.spawn`, map
  the child's fd 0/1/2 onto proc's per-process stdio, rebuild `go.wasm`. Then
  `go list`/`go build` orchestrate the compiler and linker that already run
  in the tab today.
- **After.** jsfs advisory locks; `/goproxy` passthrough for `go install`.
