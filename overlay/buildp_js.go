//go:build js && wasm

package cfg

import "syscall/js"

// shipwright overlay: a DEFAULT -p for builds that run inside a browser tab.
//
// This file does not exist upstream. overlay.json maps it onto a path in
// cmd/go/internal/cfg that Go does not ship, which -overlay is happy to do —
// "a build will run as if the disk file path exists". Adding a file rather
// than replacing cfg.go is the whole point: cfg.go is 700 lines of churn we
// would have to rebase every release to change one initializer, and the one
// initializer is all we want.
//
// What we want it for: cfg.go sets
//
//	BuildP = runtime.GOMAXPROCS(0) // -p flag
//
// and on js/wasm that is 1, because runtime.NumCPU() is 1, because the host
// really does have one JS thread. That is the correct answer to "how many
// threads may this program's scheduler use" and the wrong answer to "how many
// child processes may the go command run at once", which is what -p means.
// Those stopped being the same question when syscall.StartProcess started
// dispatching compile/link/asm/vet to Workers (see the offThreadTools
// whitelist in syscall_js.go): the children now execute on other cores, so
// several genuinely overlap. With -p left at 1 nothing asks them to, and every
// in-tab `go build` is serial unless a human types -p N by hand.
//
// Two other places could have carried this and were rejected:
//
//   - runtime.NumCPU()/GOMAXPROCS on js/wasm. Too broad by far. It would
//     change goroutine scheduling in every js/wasm binary ever built with this
//     toolchain, to describe a parallelism that the Go scheduler still cannot
//     use — there is still exactly one thread running Go code per instance.
//     The thing that became parallel is processes, not threads.
//   - the page, passing -p on the one `go build` it invokes. Least invasive
//     and least useful: it does nothing for someone typing `go build` at the
//     shipyard terminal, which is the case that actually matters, and a
//     GOFLAGS=-p=N in the seeded environment would be silently lost the moment
//     anyone set GOFLAGS themselves.
//
// Ordering works out without any help: package-level initializers run before
// init functions, so this overwrites cfg.go's GOMAXPROCS value; and package
// work imports cfg, so cfg is fully initialized before AddBuildFlags captures
// BuildP as the -p flag's default. An explicit -p on the command line still
// wins, which is what the flag default means.
func init() {
	BuildP = browserBuildP()
}

// buildPCap bounds the default however many cores the browser claims.
//
// Measured with probe-parallel.html on an 8-core host where Brave reports
// navigator.hardwareConcurrency 6, building a hello-world against the seeded
// std — 96 child compiles, a fresh GOCACHE per run so nothing is a cache hit,
// the tab kept visible (a backgrounded tab throttles its timers and times
// fiction), and every run producing the same 2500671-byte output:
//
//	-p 1   100.2s, 101.4s   1.00x   (the old default: NumCPU()==1)
//	-p 2    76.3s           1.31x
//	-p 3    73.4s           1.37x
//	-p 4    71.9s, 74.4s    1.39x
//	-p 6    72.1s, 73.9s, 73.9s   1.39x
//	-p 8    72.3s once, then HUNG the tab on 2 of 3 attempts
//
// and, with this file in place and no -p on the command line at all:
//
//	default 73.1s, 76.0s, 77.8s   peak 4 concurrent workers
//
// Two things in that shape decide the cap.
//
// The gain is over by 3 or 4. It stops well short of the 4x the worker count
// suggests because cmd/go still drives the action graph from the page thread,
// std's dependency chains are long and thin, and every file a worker touches
// crosses fsbridge back to a jsfs that only the page thread owns — 55us a
// call, 513us for a stat+open+read+close cycle. That owner is one thread, and
// it is the same thread cmd/go is running on, so past a handful of workers the
// extra ones only queue deeper against it. Nothing above 4 bought anything
// measurable.
//
// And the tail is not merely flat, it is unsafe. At -p 8 the page thread went
// to 100% and never came back: no reply to a CDP evaluate, no screenshot, the
// probe's own 300s watchdog never fired because the timer that would have
// fired it needed that thread. It survived one run in three, and one of the two
// hangs was on an otherwise idle host, so this is not simply a busy machine. A
// separate run of the whole sweep while the host WAS loaded (load average 15,
// swapping) put -p 6 at 238.6s — 3.3x SLOWER than -p 1 — which the
// quiet-machine numbers above never hint at. Whatever the mechanism, the
// failure is a starved page thread, and the way to not hit it is to not ask
// for it: 4 is below anything that has ever misbehaved here, and gives up
// nothing.
const buildPCap = 4

// browserBuildP reports how many child toolchain processes this tab should run
// at once, or 1 when it cannot run any of them off the page thread.
func browserBuildP() int {
	// The gate has to match syscall.offThread exactly, because that is what
	// actually decides where a child lands. Off-thread children need a
	// SharedArrayBuffer for the fsbridge channel, and browsers only hand one to
	// a cross-origin-isolated page; without isolation every child runs ON the
	// page thread, one at a time. A -p above 1 would then be actively harmful —
	// cmd/go would start N action goroutines that each block the single thread
	// in turn, adding scheduling churn to a build that cannot overlap anyway.
	g := js.Global()
	if !g.Get("crossOriginIsolated").Truthy() || !g.Get("fsbridge").Truthy() {
		return 1
	}
	if proc := g.Get("proc"); !proc.Truthy() || !proc.Get("spawnWorker").Truthy() {
		return 1
	}

	// navigator.hardwareConcurrency is the browser's own answer and the only
	// one available here. It is not the host's core count: privacy-hardened
	// browsers round it down or clamp it (Brave reports 6 on this 8-core box),
	// which for our purposes is a feature — a browser that wants to be treated
	// as smaller gets treated as smaller.
	hc := 0
	if v := g.Get("navigator").Get("hardwareConcurrency"); v.Type() == js.TypeNumber {
		hc = v.Int()
	}
	if hc <= 0 {
		// A browser that will not say gets the one extra worker that pays best:
		// -p 2 already reached 1.31x of the 1.39x that -p 4 tops out at, and it
		// is hard to imagine a machine running a browser that cannot afford one
		// more thread.
		return 2
	}

	// Leave the page thread a core of its own. It is not a spare: it runs
	// cmd/go's orchestration AND serves every worker's file I/O over fsbridge,
	// so oversubscribing it does not slow down one worker, it slows down all of
	// them at once. Costing nothing here, since the cap binds first on anything
	// with 5 cores or more.
	n := hc - 1
	if n < 1 {
		return 1
	}
	if n > buildPCap {
		return buildPCap
	}
	return n
}
