package store

import (
	"context"
	"testing"
	"time"
)

// TestNewPool_InvalidDSN covers the ParseConfig error branch: a DSN with
// a non-numeric port can't be parsed, so NewPool returns before any
// network I/O. Pure + deterministic (no Docker).
func TestNewPool_InvalidDSN(t *testing.T) {
	_, err := NewPool(context.Background(), PoolConfig{
		DatabaseURL:    "postgres://user:pass@localhost:not-a-port/buildos",
		MaxConns:       4,
		MinConns:       0,
		ConnectTimeout: 2 * time.Second,
	})
	if err == nil {
		t.Fatal("NewPool with invalid DSN = nil error, want parse error")
	}
}

// TestNewPool_UnreachableDB covers the Ping failure branch: the config
// parses and the (lazy) pool is created, but the first Ping against a
// port that nothing listens on is refused, so NewPool closes the pool
// and returns the ping error. Port 1 is reserved and never accepts
// connections, so this fails fast and deterministically (no Docker, no
// timeout wait — ECONNREFUSED is immediate).
func TestNewPool_UnreachableDB(t *testing.T) {
	_, err := NewPool(context.Background(), PoolConfig{
		DatabaseURL:    "postgres://test:test@127.0.0.1:1/buildos?sslmode=disable",
		MaxConns:       4,
		MinConns:       0,
		ConnectTimeout: 2 * time.Second,
	})
	if err == nil {
		t.Fatal("NewPool against unreachable DB = nil error, want ping failure")
	}
}
