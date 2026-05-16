package main

import (
	"log"
	"net/http"
	"os"

	"github.com/IstarVin/hls-proxy/internal/proxy"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	proxyBase := os.Getenv("PROXY_BASE") // e.g. https://my-server.example.com
	// If unset, the handler falls back to http://r.Host (self-configuring).

	mux := http.NewServeMux()
	mux.Handle("/proxy", proxy.Handler(proxyBase))
	mux.Handle("/proxy/", proxy.Handler(proxyBase))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("HLS proxy OK\n"))
	})

	addr := ":" + port
	log.Printf("hls-proxy listening on %s (PROXY_BASE=%q)", addr, proxyBase)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
