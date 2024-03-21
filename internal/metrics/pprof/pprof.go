// Package pprof is separated out from metrics to isolate the 'init' functionality of pprof, so that it is
// included when used by binaries, but not if other drand packages get used or integrated into clients that
// don't expect the pprof side effect to have taken effect.
package pprof

import (
	"net/http"
	"net/http/pprof" // adds default pprof endpoint at /debug/pprof
)

// WithProfile provides an http mux setup to serve pprof endpoints. it should be mounted at /debug/pprof
func WithProfile() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", pprof.Index)
	// sub-path need to handle the whole path for the matching to work
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	return mux
}
