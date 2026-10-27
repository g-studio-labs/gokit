// Package httpserver builds the http.Server with the timeouts no service
// should forget and runs it with graceful shutdown on SIGINT/SIGTERM (Swarm
// and k8s send SIGTERM on update).
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

	"github.com/g-studio-labs/gokit/lg"
)

// Options tunes the server. Zero values take the house defaults.
type Options struct {
	ReadHeaderTimeout time.Duration // 10s: stops slowloris
	ReadTimeout       time.Duration // 15s; raise it if you receive large uploads
	WriteTimeout      time.Duration // 30s; SSE/streaming remove it with http.ResponseController
	IdleTimeout       time.Duration // 60s
	ShutdownTimeout   time.Duration // 15s to finish in-flight requests
	MaxHeaderBytes    int           // 1 MiB

	// BeforeListen runs right before listening (the banner goes here).
	BeforeListen func()
	// OnShutdown runs after closing the listener, before exiting (close DB,
	// stop workers). It receives the context with the ShutdownTimeout deadline.
	OnShutdown func(ctx context.Context)
}

// Server wraps http.Server with Run.
type Server struct {
	*http.Server
	opts Options
}

// New creates the server with safe defaults.
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
		ErrorLog:          nil, // uses slog.Default (lg.Setup sets it)
	}
	tune(srv)
	return &Server{Server: srv, opts: o}
}

// Run listens until SIGINT/SIGTERM arrives or ctx is canceled, and then
// finishes in-flight requests within ShutdownTimeout. It returns nil on a
// clean shutdown.
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
			return fmt.Errorf("listen on %s: %w", s.Addr, err)
		}
		return nil
	case <-ctx.Done():
		lg.Info("shutting down: finishing in-flight requests", "cause", causeOf(ctx))
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.opts.ShutdownTimeout)
	defer cancel()
	err := s.Shutdown(shutdownCtx)
	if s.opts.OnShutdown != nil {
		s.opts.OnShutdown(shutdownCtx)
	}
	if err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}

func causeOf(ctx context.Context) string {
	if c := context.Cause(ctx); c != nil {
		return c.Error()
	}
	return "context canceled"
}

// HealthCheck requests GET http://127.0.0.1<addr><path> and fails if it does not
// respond 200. It exists for the `api health` subcommand: the distroless image
// has no curl or wget, so the Docker HEALTHCHECK is CMD ["/api", "health"].
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
		return fmt.Errorf("health responded %d", res.StatusCode)
	}
	return nil
}
