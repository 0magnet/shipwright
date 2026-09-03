// serve hosts the shipwright page and the network egress the tab's cmd/go
// needs. cmd/go's downloads go through net/http, which on js/wasm is the
// browser Fetch API: a tab can call its own origin but not proxy.golang.org
// or sum.golang.org (CORS). serve stands in for both, same-origin:
//
//   /goproxy/                       → the Go module proxy (proxy.golang.org)
//   /goproxy/sumdb/sum.golang.org/  → the checksum database (sum.golang.org),
//                                     mirrored per the proxy protocol so the
//                                     tab can run with GOSUMDB on
//
// The tab builds with GOPROXY=<origin>/goproxy and the stock GOSUMDB, and both
// the fetch and its checksum verification cross same-origin. This is the "few
// lines on any host server" the README's network gap asked for.
//
//	go run ./serve            # :8931, / = static, /goproxy = proxy + sumdb
//	go run ./serve -addr :9000 -upstream https://goproxy.cn
package main

import (
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// passthrough builds a reverse proxy to target that rewrites Host so target's
// TLS and routing see their own name rather than ours.
func passthrough(raw string) (*httputil.ReverseProxy, *url.URL) {
	target, err := url.Parse(raw)
	if err != nil {
		log.Fatalf("bad upstream %q: %v", raw, err)
	}
	rp := httputil.NewSingleHostReverseProxy(target)
	director := rp.Director
	rp.Director = func(r *http.Request) {
		director(r)
		r.Host = target.Host
	}
	return rp, target
}

func main() {
	addr := flag.String("addr", ":8931", "listen address")
	dir := flag.String("dir", ".", "directory to serve")
	upstream := flag.String("upstream", "https://proxy.golang.org", "Go module proxy to pass through to")
	sumdb := flag.String("sumdb", "https://sum.golang.org", "checksum database to mirror")
	flag.Parse()

	modRP, mod := passthrough(*upstream)
	sumRP, _ := passthrough(*sumdb)

	mux := http.NewServeMux()

	// The proxy sumdb-mirror protocol: /supported answered here says "yes, ask
	// me for sumdb data too"; the rest is forwarded to sum.golang.org with the
	// /goproxy/sumdb/<host> prefix stripped, leaving /lookup, /tile, /latest.
	const sumPrefix = "/goproxy/sumdb/sum.golang.org"
	mux.HandleFunc(sumPrefix+"/supported", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle(sumPrefix+"/", http.StripPrefix(sumPrefix, sumRP))

	// Everything else under /goproxy is the module proxy. StripPrefix leaves
	// the leading slash, so /goproxy/mod/@v/list becomes /mod/@v/list — the
	// module-proxy protocol path upstream expects.
	mux.Handle("/goproxy/", http.StripPrefix("/goproxy", modRP))

	mux.Handle("/", http.FileServer(http.Dir(*dir)))

	log.Printf("shipwright: http://localhost%s  (/goproxy → %s, sumdb → %s)", *addr, mod, *sumdb)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
