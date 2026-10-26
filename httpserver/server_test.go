package httpserver

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestRunApagaConContexto(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	listening := make(chan struct{})
	shutdownRan := false
	srv := New(addr, mux, Options{
		BeforeListen: func() { close(listening) },
		OnShutdown:   func(context.Context) { shutdownRan = true },
	})
	if srv.ReadHeaderTimeout != 10*time.Second || srv.MaxHeaderBytes != 1<<20 {
		t.Error("defaults no aplicados")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()
	<-listening

	var hcErr error
	for i := 0; i < 50; i++ {
		if hcErr = HealthCheck(addr, "/health", time.Second); hcErr == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if hcErr != nil {
		t.Fatalf("health: %v", hcErr)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run no terminó")
	}
	if !shutdownRan {
		t.Error("OnShutdown no corrió")
	}
}
