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

// Package virtnettest provides basic tests to be run on virtnet network implementations.
package virtnettest

import (
	"context"
	"io"
	"net"
	"testing"

	"golang.org/x/sync/errgroup"

	"lab.nexedi.com/kirr/go123/exc"
	"lab.nexedi.com/kirr/go123/internal/xtesting"
	"lab.nexedi.com/kirr/go123/xnet"
	"lab.nexedi.com/kirr/go123/xnet/virtnet"
)


type mklistener interface {
	Listen(context.Context, string) (xnet.Listener, error)
}

func xlisten(ctx context.Context, n mklistener, laddr string) xnet.Listener {
	l, err := n.Listen(ctx, laddr)
	exc.Raiseif(err)
	return l
}

func xaccept(ctx context.Context, l xnet.Listener) net.Conn {
	c, err := l.Accept(ctx)
	exc.Raiseif(err)
	return c
}

type dialer interface {
	Dial(context.Context, string) (net.Conn, error)
}

func xdial(n dialer, addr string) net.Conn {
	c, err := n.Dial(context.Background(), addr)
	exc.Raiseif(err)
	return c
}

func xread(r io.Reader) string {
	buf := make([]byte, 4096)
	n, err := r.Read(buf)
	exc.Raiseif(err)
	return string(buf[:n])
}

func xwrite(w io.Writer, data string) {
	_, err := w.Write([]byte(data))
	exc.Raiseif(err)
}

func xwait(w interface { Wait() error }) {
	err := w.Wait()
	exc.Raiseif(err)
}

// TestBasic runs basic tests on a virtnet network implementation.
func TestBasic(t *testing.T, subnet *virtnet.SubNetwork) {
	X := exc.Raiseif
	ctx := context.Background()
	assert := xtesting.Assert(t)

	defer func() {
		err := subnet.Close()
		X(err)
	}()

	xaddr := func(addr string) *virtnet.Addr {
		a, err := virtnet.ParseAddr(subnet.Network(), addr)
		X(err)
		return a
	}

	hα, err := subnet.NewHost(ctx, "α")
	X(err)

	hβ, err := subnet.NewHost(ctx, "β")
	X(err)

	assert.Eq(hα.Network(), subnet.Network())
	assert.Eq(hβ.Network(), subnet.Network())
	assert.Eq(hα.Name(), "α")
	assert.Eq(hβ.Name(), "β")

	_, err = hα.Dial(ctx, ":0")
	assert.Eq(err, &net.OpError{Op: "dial", Net: subnet.Network(), Source: xaddr("α:1"), Addr: xaddr("α:0"), Err: virtnet.ErrConnRefused})

	l1, err := hα.Listen(ctx, "")
	X(err)
	assert.Eq(l1.Addr(), xaddr("α:1"))

	// zero port always stays unused even after autobind
	_, err = hα.Dial(ctx, ":0")
	assert.Eq(err, &net.OpError{Op: "dial", Net: subnet.Network(), Source: xaddr("α:2"), Addr: xaddr("α:0"), Err: virtnet.ErrConnRefused})

	wg := &errgroup.Group{}
	wg.Go(exc.Funcx(func() {
		c1s := xaccept(ctx, l1)
		assert.Eq(c1s.LocalAddr(), xaddr("α:2"))
		assert.Eq(c1s.RemoteAddr(), xaddr("β:1"))

		assert.Eq(xread(c1s), "ping")		// XXX for !pipe could read less
		xwrite(c1s, "pong")

		c2s := xaccept(ctx, l1)
		assert.Eq(c2s.LocalAddr(), xaddr("α:3"))
		assert.Eq(c2s.RemoteAddr(), xaddr("β:2"))

		assert.Eq(xread(c2s), "hello")
		xwrite(c2s, "world")
	}))

	c1c := xdial(hβ, "α:1")
	assert.Eq(c1c.LocalAddr(), xaddr("β:1"))
	assert.Eq(c1c.RemoteAddr(), xaddr("α:2"))

	xwrite(c1c, "ping")
	assert.Eq(xread(c1c), "pong")

	c2c := xdial(hβ, "α:1")
	assert.Eq(c2c.LocalAddr(), xaddr("β:2"))
	assert.Eq(c2c.RemoteAddr(), xaddr("α:3"))

	xwrite(c2c, "hello")
	assert.Eq(xread(c2c), "world")

	xwait(wg)

	l2 := xlisten(ctx, hα, ":0") // autobind again
	assert.Eq(l2.Addr(), xaddr("α:4"))
}
