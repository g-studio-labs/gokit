//go:build go1.27

package httpserver

import "net/http"

// Go 1.27 added MaxHeaderValueCount: how many values it accepts per header
// (defense against wildly repeated headers). It is set here so the module
// still builds with 1.26.
func tune(s *http.Server) { s.MaxHeaderValueCount = 100 }
