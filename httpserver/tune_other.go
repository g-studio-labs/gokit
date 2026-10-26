//go:build !go1.27

package httpserver

import "net/http"

func tune(*http.Server) {}
