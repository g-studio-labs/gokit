package db

import (
	"context"
	"testing"
	"time"
)

func TestOpenFailsFastWithoutDatabase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := Open(ctx, "postgres://nobody:nothing@127.0.0.1:1/no?sslmode=disable&connect_timeout=1", Options{Retries: 1})
	if err == nil {
		t.Fatal("must fail without a database")
	}
}
