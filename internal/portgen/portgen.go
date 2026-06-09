// Package portgen предоставляет генератор свободных TCP-портов на 127.0.0.1.
package portgen

import (
	"context"
	"errors"
	"fmt"
	"net"
)

type Generator struct{}

// New создаёт Generator, использующий net.Listen("tcp", "127.0.0.1:0").
func New() *Generator {
	return &Generator{}
}

func (g *Generator) RandomPort(ctx context.Context) (int, error) {
	lc := net.ListenConfig{}
	listener, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("get random port: %w", err)
	}
	defer func() { _ = listener.Close() }()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("unexpected address type")
	}
	return addr.Port, nil
}
