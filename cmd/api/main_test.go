package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingListener struct{ err error }

func (listener failingListener) Accept() (net.Conn, error) { return nil, listener.err }
func (failingListener) Close() error                       { return nil }
func (failingListener) Addr() net.Addr                     { return &net.TCPAddr{} }

type pipeListener struct {
	connections chan net.Conn
	closed      chan struct{}
	closeOnce   sync.Once
}

func newPipeListener() *pipeListener {
	return &pipeListener{connections: make(chan net.Conn, 1), closed: make(chan struct{})}
}

func (listener *pipeListener) Accept() (net.Conn, error) {
	select {
	case connection := <-listener.connections:
		return connection, nil
	case <-listener.closed:
		return nil, net.ErrClosed
	}
}

func (listener *pipeListener) Close() error {
	listener.closeOnce.Do(func() { close(listener.closed) })
	return nil
}

func (*pipeListener) Addr() net.Addr { return &net.TCPAddr{} }

func TestServeReturnsListenerFailure(t *testing.T) {
	want := errors.New("listener failed")
	err := serve(context.Background(), &http.Server{}, failingListener{err: want}, time.Second)
	require.Error(t, err)
	assert.ErrorIs(t, err, want)
}

func TestServeStopsOnCancellation(t *testing.T) {
	listener := newPipeListener()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.NoError(t, serve(ctx, &http.Server{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})}, listener, time.Second))
}

func TestServeBoundsShutdown(t *testing.T) {
	listener := newPipeListener()
	started := make(chan struct{})
	release := make(chan struct{})
	server := &http.Server{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { close(started); <-release })}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- serve(ctx, server, listener, 20*time.Millisecond) }()
	serverConnection, clientConnection := net.Pipe()
	listener.connections <- serverConnection
	go func() {
		defer func() { _ = clientConnection.Close() }()
		_, _ = clientConnection.Write([]byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"))
		buffer := make([]byte, 1)
		_, _ = clientConnection.Read(buffer)
	}()
	<-started
	cancel()
	err := <-result
	close(release)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}
