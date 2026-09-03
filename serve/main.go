// serve hosts the shipwright page and, at /goproxy/, reverse-proxies the Go
// module proxy. The tab's cmd/go fetches modules with GOPROXY pointed here;
// because the request is same-origin, the browser's CORS wall — which blocks
// a tab from calling proxy.golang.org directly — never comes up. This is the
// "few lines on any host server" the README's network gap asks for.
//
//	go run ./serve            # :8931, / = static, /goproxy/ = module proxy
//	go run ./serve -addr :9000 -upstream https://goproxy.cn
package main

import (
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

func main() {
	addr := flag.String("addr", ":8931", "listen address")
	dir := flag.String("dir", ".", "directory to serve")
	upstream := flag.String("upstream", "https://proxy.golang.org", "Go module proxy to pass through to")
	flag.Parse()

	target, err := url.Parse(*upstream)
	if err != nil {
		log.Fatalf("bad -upstream %q: %v", *upstream, err)
	}
	rp := httputil.NewSingleHostReverseProxy(target)
	// Rewrite Host so upstream TLS/routing sees its own name, not ours.
	director := rp.Director
	rp.Director = func(r *http.Request) {
		director(r)
		r.Host = target.Host
	}

	mux := http.NewServeMux()
	// StripPrefix leaves the leading slash, so /goproxy/mod/@v/list becomes
	// /mod/@v/list — exactly the module-proxy protocol path upstream expects.
	mux.Handle("/goproxy/", http.StripPrefix("/goproxy", rp))
	mux.Handle("/", http.FileServer(http.Dir(*dir)))

	log.Printf("shipwright: http://localhost%s  (/goproxy → %s)", *addr, *upstream)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
