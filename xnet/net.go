// Copyright (C) 2017-2020  Nexedi SA and Contributors.
//                          Kirill Smelkov <kirr@nexedi.com>
//
// This program is free software: you can Use, Study, Modify and Redistribute
// it under the terms of the GNU General Public License version 3, or (at your
// option) any later version, as published by the Free Software Foundation.
//
// You can also Link and Combine this program with other software covered by
// the terms of any of the Free Software licenses or any of the Open Source
// Initiative approved licenses and Convey the resulting work. Corresponding
// source of such a combination shall include the source code for all other
// software used.
//
// This program is distributed WITHOUT ANY WARRANTY; without even the implied
// warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
//
// See COPYING file for full licensing terms.
// See https://www.nexedi.com/licensing for rationale and options.

// Package xnet provides addons to std package net.
package xnet

import (
	"context"
	"fmt"
	"net"
	"os"

	"crypto/tls"

	"lab.nexedi.com/kirr/go123/xsync"
)

// Networker is interface representing access-point to a streaming network.
type Networker interface {
	// Network returns name of the network.
	Network() string

	// Name returns name of the access-point on the network.
	//
	// Example of name is local hostname if networker provides access to
	// OS-level dial/listen.
	Name() string

	// Dial connects to addr on underlying network.
	//
	// See net.Dial for semantic details.
	Dial(ctx context.Context, addr string) (net.Conn, error)

	// Listen starts listening on local address laddr on underlying network access-point.
	//
	// See net.Listen for semantic details.
	Listen(ctx context.Context, laddr string) (Listener, error)
}

// Listener amends net.Listener for Accept to handle cancellation.
type Listener interface {
	Accept(ctx context.Context) (net.Conn, error)

	// same as in net.Listener
	Close() error
	Addr() net.Addr
}


var hostname string
func init() {
	host, err := os.Hostname()
	if err != nil {
		panic(fmt.Errorf("cannot detect hostname: %s", err))
	}
	hostname = host
}

// NetPlain creates Networker corresponding to regular network accessors from std package net.
//
// network is "tcp", "tcp4", "tcp6", "unix", etc...
func NetPlain(network string) Networker {
	return &netPlain{network, hostname}
}

type netPlain struct {
	network, hostname string
}

func (n *netPlain) Network() string {
	return n.network
}

func (n *netPlain) Name() string {
	return n.hostname
}

func (n *netPlain) Dial(ctx context.Context, addr string) (net.Conn, error) {
	d := net.Dialer{}
	return d.DialContext(ctx, n.network, addr)
}

func (n *netPlain) Listen(ctx context.Context, laddr string) (Listener, error) {
	lc := net.ListenConfig{}
	rawl, err := lc.Listen(ctx, n.network, laddr)
	if err != nil {
		return nil, err
	}
	return newListenerCtx(rawl), nil
}

// NetTLS wraps underlying networker with TLS layer according to config.
//
// The config must be valid:
//
//	- for tls.Client -- for Dial to work,
//	- for tls.Server -- for Listen to work.
func NetTLS(inner Networker, config *tls.Config) Networker {
	return &netTLS{inner, config}
}

type netTLS struct {
	inner  Networker
	config *tls.Config
}

func (n *netTLS) Network() string {
	return n.inner.Network() + "+tls"
}

func (n *netTLS) Name() string {
	return n.inner.Name()
}

func (n *netTLS) Dial(ctx context.Context, addr string) (net.Conn, error) {
	c, err := n.inner.Dial(ctx, addr)
	if err != nil {
		return nil, err
	}
	return tls.Client(c, n.config), nil
}

func (n *netTLS) Listen(ctx context.Context, laddr string) (Listener, error) {
	l, err := n.inner.Listen(ctx, laddr)
	if err != nil {
		return nil, err
	}
	return &listenerTLS{l, n}, nil
}

// listenerTLS implements Listener for netTLS.
type listenerTLS struct {
	innerl Listener
	net    *netTLS
}

func (l *listenerTLS) Close() error {
	return l.innerl.Close()
}

func (l *listenerTLS) Addr() net.Addr {
	return l.innerl.Addr()
}

func (l *listenerTLS) Accept(ctx context.Context) (net.Conn, error) {
	conn, err := l.innerl.Accept(ctx)
	if err != nil {
		return nil, err
	}
	return tls.Server(conn, l.net.config), nil
}


// ----------------------------------------

// listenerCtx provides Listener given net.Listener.
type listenerCtx struct {
	rawl        net.Listener     // underlying listener
	serveWG     *xsync.WorkGroup // Accept loop is run under serveWG
	serveCancel func()           // Close calls serveCancel to request Accept loop shutdown
	acceptq     chan accepted    // Accept results go -> acceptq
}

// accepted represents Accept result.
type accepted struct {
	conn net.Conn
	err  error
}

func newListenerCtx(rawl net.Listener) *listenerCtx {
	l := &listenerCtx{rawl: rawl, acceptq: make(chan accepted)}
	ctx, cancel := context.WithCancel(context.Background())
	l.serveWG = xsync.NewWorkGroup(ctx)
	l.serveCancel = cancel
	l.serveWG.Go(l.serve)
	return l
}

func (l *listenerCtx) serve(ctx context.Context) error {
	for {
		// raw Accept. This should not stuck overliving ctx as Close closes rawl
		conn, err := l.rawl.Accept()

		// send result to Accept, but don't try to send if we are closed
		ctxErr := ctx.Err()
		if ctxErr == nil {
			select {
			case <-ctx.Done():
				// closed
				ctxErr = ctx.Err()

			case l.acceptq <- accepted{conn, err}:
				// ok
			}
		}
		// shutdown if we are closed
		if ctxErr != nil {
			if conn != nil {
				conn.Close() // ignore err
			}
			return ctxErr
		}
	}
}

func (l *listenerCtx) Close() error {
	l.serveCancel()
	err := l.rawl.Close()
	_ = l.serveWG.Wait() // ignore err - it is always "canceled"
	return err
}

func (l *listenerCtx) Accept(ctx context.Context) (_ net.Conn, err error) {
	err = ctx.Err()

	// don't try to pull from acceptq if ctx is already canceled
	if err == nil {
		select {
		case <-ctx.Done():
			err = ctx.Err()

		case a := <-l.acceptq:
			return a.conn, a.err
		}
	}

	// here it is always due to ctx cancel
	laddr := l.rawl.Addr()
	return nil, &net.OpError{
		Op:     "accept",
		Net:    laddr.Network(),
		Source: nil,
		Addr:   laddr,
		Err:    err,
	}
}

func (l *listenerCtx) Addr() net.Addr {
	return l.rawl.Addr()
}
