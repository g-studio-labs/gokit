//go:build go1.27

package httpserver

import "net/http"

// Go 1.27 añadió MaxHeaderValueCount: cuántos valores acepta por header
// (defensa contra cabeceras repetidas a lo loco). Se fija aquí para que el
// módulo siga compilando con 1.26.
func tune(s *http.Server) { s.MaxHeaderValueCount = 100 }
