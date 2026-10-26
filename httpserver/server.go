// Package httpserver arma el http.Server con los timeouts que ningún servicio
// debería olvidar y lo corre con apagado ordenado por SIGINT/SIGTERM (Swarm y
// k8s mandan SIGTERM al actualizar).
//
//	srv := httpserver.New(":8080", router, httpserver.Options{
//	    BeforeListen: func() { banner.Print(...) },
//	})
//	return srv.Run(context.Background())
package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/RomanshkVolkov/gokit/lg"
)

// Options ajusta el servidor. Los ceros toman los defaults de la casa.
type Options struct {
	ReadHeaderTimeout time.Duration // 10s: corta slowloris
	ReadTimeout       time.Duration // 15s; súbelo si recibes uploads grandes
	WriteTimeout      time.Duration // 30s; SSE/streaming lo quitan con http.ResponseController
	IdleTimeout       time.Duration // 60s
	ShutdownTimeout   time.Duration // 15s para terminar peticiones en curso
	MaxHeaderBytes    int           // 1 MiB

	// BeforeListen corre justo antes de escuchar (el banner va aquí).
	BeforeListen func()
	// OnShutdown corre tras cerrar el listener, antes de salir (cerrar DB,
	// parar workers). Recibe el contexto con el plazo de ShutdownTimeout.
	OnShutdown func(ctx context.Context)
}

// Server envuelve http.Server con Run.
type Server struct {
	*http.Server
	opts Options
}

// New crea el servidor con defaults seguros.
func New(addr string, h http.Handler, o Options) *Server {
	def := func(d *time.Duration, v time.Duration) {
		if *d == 0 {
			*d = v
		}
	}
	def(&o.ReadHeaderTimeout, 10*time.Second)
	def(&o.ReadTimeout, 15*time.Second)
	def(&o.WriteTimeout, 30*time.Second)
	def(&o.IdleTimeout, 60*time.Second)
	def(&o.ShutdownTimeout, 15*time.Second)
	if o.MaxHeaderBytes == 0 {
		o.MaxHeaderBytes = 1 << 20
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: o.ReadHeaderTimeout,
		ReadTimeout:       o.ReadTimeout,
		WriteTimeout:      o.WriteTimeout,
		IdleTimeout:       o.IdleTimeout,
		MaxHeaderBytes:    o.MaxHeaderBytes,
		ErrorLog:          nil, // usa slog.Default (lg.Setup lo fija)
	}
	tune(srv)
	return &Server{Server: srv, opts: o}
}

// Run escucha hasta que llega SIGINT/SIGTERM o ctx se cancela, y entonces
// termina las peticiones en curso dentro de ShutdownTimeout. Devuelve nil en
// un apagado limpio.
func (s *Server) Run(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errs := make(chan error, 1)
	go func() {
		if s.opts.BeforeListen != nil {
			s.opts.BeforeListen()
		}
		errs <- s.ListenAndServe()
	}()

	select {
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("escuchar en %s: %w", s.Addr, err)
		}
		return nil
	case <-ctx.Done():
		lg.Info("cerrando: termino las peticiones en curso", "causa", causeOf(ctx))
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.opts.ShutdownTimeout)
	defer cancel()
	err := s.Shutdown(shutdownCtx)
	if s.opts.OnShutdown != nil {
		s.opts.OnShutdown(shutdownCtx)
	}
	if err != nil {
		return fmt.Errorf("apagado: %w", err)
	}
	return nil
}

func causeOf(ctx context.Context) string {
	if c := context.Cause(ctx); c != nil {
		return c.Error()
	}
	return "contexto cancelado"
}

// HealthCheck pide GET http://127.0.0.1<addr><path> y falla si no responde 200.
// Existe para el subcomando `api health`: la imagen distroless no trae curl ni
// wget, así el HEALTHCHECK de Docker es CMD ["/api", "health"].
func HealthCheck(addr, path string, timeout time.Duration) error {
	if path == "" {
		path = "/health"
	}
	if timeout == 0 {
		timeout = 3 * time.Second
	}
	if addr == "" {
		addr = ":8080"
	}
	if addr[0] == ':' {
		addr = "127.0.0.1" + addr
	}
	cli := http.Client{Timeout: timeout}
	res, err := cli.Get("http://" + addr + path)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("health respondió %d", res.StatusCode)
	}
	return nil
}
