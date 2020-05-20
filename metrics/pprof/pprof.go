// Package pprof is separated out from metrics to isolate the 'init' functionality of pprof, so that it is
// included when used by binaries, but not if other drand packages get used or integrated into clients that
// don't expect the pprof side effect to have taken effect.
package pprof

import (
	"net/http"

	pprof "net/http/pprof" // adds default pprof endpoint at /debug/pprof
)

// WithProfile provides an http mux setup to serve pprof endpoints. it should be mounted at /debug/pprof
func WithProfile() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", pprof.Index)
	mux.HandleFunc("/cmdline", pprof.Cmdline)
	mux.HandleFunc("/profile", pprof.Profile)
	mux.HandleFunc("/symbol", pprof.Symbol)
	mux.HandleFunc("/trace", pprof.Trace)

	return mux
}
