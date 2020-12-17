// Copyright (C) 2018-2020  Nexedi SA and Contributors.
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

package virtnet_test

import (
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"

	"lab.nexedi.com/kirr/go123/exc"
	"lab.nexedi.com/kirr/go123/internal/xtesting"
	"lab.nexedi.com/kirr/go123/xnet"
	"lab.nexedi.com/kirr/go123/xnet/pipenet"
	. "lab.nexedi.com/kirr/go123/xnet/virtnet"

	"github.com/pkg/errors"
)

// testNet is testing network environment.
//
// It consists of a subnetwork backed by pipenet with 2 hosts: hα and hβ. On
// both hosts a listener is started at "" (i.e. it will have ":1" address).
// There is a connection established in between α:2-β:2.
type testNet struct {
	testing.TB

	net      *SubNetwork
	hα, hβ   *Host
	lα, lβ   xnet.Listener
	cαβ, cβα net.Conn
}

// newTestNet creates new testing network environment.
func newTestNet(t0 testing.TB) *testNet {
	t := &testNet{TB: t0}
	t.Helper()

	var err error
	t.net = pipenet.AsVirtNet(pipenet.New("t"))
	t.hα, err = t.net.NewHost(context.Background(), "α")
	if err != nil {
		t.Fatal(err)
	}
	t.hβ, err = t.net.NewHost(context.Background(), "β")
	if err != nil {
		t.Fatal(err)
	}
	t.lα, err = t.hα.Listen(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	t.lβ, err = t.hβ.Listen(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}

	// preestablish α:2-β:2 connection
	wg := &errgroup.Group{}
	defer func() {
		err := wg.Wait()
		if err != nil {
			t.Fatal(err)
		}
	}()

	wg.Go(func() error {
		c, err := t.lβ.Accept(context.Background())
		if err != nil {
			return err
		}
		t.cβα = c
		return nil
	})

	c, err := t.hα.Dial(context.Background(), "β:1")
	if err != nil {
		t.Fatal(err)
	}
	t.cαβ = c

	return t
}

// xneterr constructs net.OpError for testNet network.
//
// if addr is of form "α:1" - only .Addr is set.
// if addr is of form "α:1->β:1" - both .Source and .Addr are set.
func xneterr(op, addr string, err error) *net.OpError {
	addrv := strings.Split(addr, "->")
	if len(addrv) > 2 {
		exc.Raisef("xneterr: invalid addr %q", addr)
	}

	operr := &net.OpError{
		Op:  op,
		Net: "pipet", // matches newTestNet
		Err: err,
	}

	for i, addr := range addrv {
		a, e := ParseAddr("pipet", addr)
		exc.Raiseif(e)

		if i == len(addrv)-1 {
			operr.Addr = a
		} else {
			operr.Source = a
		}
	}

	return operr
}

// xobject lookups testNet object by name.
func (t *testNet) xobject(name string) io.Closer {
	switch name {
	case "subnet":	return t.net
	case "hα":	return t.hα
	case "hβ":	return t.hβ
	case "lα":	return t.lα
	case "lβ":	return t.lβ
	case "cαβ":	return t.cαβ
	case "cβα":	return t.cβα
	}

	exc.Raisef("invalid object: %q", name)
	panic(0)
}

type testFlag int
const serialOnly testFlag = 1

// testClose verifies object.Close vs test func.
//
// object to close is specified by name, e.g. "hβ". test func should try to do
// an action and verify it gets expected error given object is closed.
//
// two scenarios are verified:
//
//	- serial case: first close, then test, and
//	- concurrent case: close is run in parallel to test.
//
// if concurrent case is not applicable for test (e.g. it tries to run a
// function that does not block, like e.g. NewHost in pipenet case), it can be
// disabled via passing optional serialOnly flag.
func testClose(t0 testing.TB, object string, test func(*testNet), flagv ...testFlag) {
	t0.Helper()

	// serial case
	t := newTestNet(t0)
	obj := t.xobject(object)

	err := obj.Close()
	if err != nil {
		t.Fatal(err)
	}
	test(t)

	if len(flagv) > 0 && flagv[0] == serialOnly {
		return
	}

	// concurrent case
	t = newTestNet(t0)
	obj = t.xobject(object)

	wg := &errgroup.Group{}
	wg.Go(func() error {
		tdelay()
		return obj.Close()
	})

	test(t)
	err = wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
}

// tdelay delays a bit.
//
// needed e.g. to test Close interaction with waiting read or write
// (we cannot easily sync and make sure e.g. read is started and became asleep)
func tdelay() {
	time.Sleep(1 * time.Millisecond)
}

// TestClose verifies that for all virtnet objects Close properly interrupt /
// errors all corresponding operations.
func TestClose(t *testing.T) {
	bg := context.Background()
	assert := xtesting.Assert(t)

	//          Subnet Host listener conn
	// NewHost     x
	// Dial        x     x      x
	// Listen      x     x
	// Accept      x     x      x
	// Read/Write  x     x             x

	// ---- NewHost ----

	// subnet.NewHost vs subnet.Close
	testClose(t, "subnet", func(t *testNet) {
		h, err := t.net.NewHost(bg, "γ")
		assert.Eq(h, (*Host)(nil))
		assert.Eq(errors.Cause(err), ErrNetDown)
		assert.Eq(err.Error(), "virtnet \"pipet\": new host \"γ\": network is down")
	}, serialOnly)

	// ---- Dial ----

	// host.Dial vs subnet.Close
	testClose(t, "subnet", func(t *testNet) {
		c, err := t.hα.Dial(bg, "β:1")
		assert.Eq(c, nil)
		assert.Eq(err, xneterr("dial", "α:3->β:1", ErrNetDown))
	})

	// host1.Dial vs host1.Close
	testClose(t, "hα", func(t *testNet) {
		c, err := t.hα.Dial(bg, "β:1")
		assert.Eq(c, nil)
		assert.Eq(err, xneterr("dial", "α:3->β:1", ErrHostDown))
	})

	// host1.Dial vs host2.Close
	testClose(t, "hβ", func(t *testNet) {
		c, err := t.hα.Dial(bg, "β:1")
		assert.Eq(c, nil)
		assert.Eq(err, xneterr("dial", "α:3->β:1", ErrConnRefused))
	})

	// host1.Dial vs host2.listener.Close
	testClose(t, "lβ", func(t *testNet) {
		c, err := t.hα.Dial(bg, "β:1")
		assert.Eq(c, nil)
		assert.Eq(err, xneterr("dial", "α:3->β:1", ErrConnRefused))
	})

	// ---- Listen ----

	// host.Listen vs subnet.Close
	testClose(t, "subnet", func(t *testNet) {
		l, err := t.hα.Listen(bg, "")
		assert.Eq(l, nil)
		assert.Eq(err, xneterr("listen", "α:0", ErrNetDown))
	}, serialOnly)

	// host.Listen vs host.Close
	testClose(t, "hα", func(t *testNet) {
		l, err := t.hα.Listen(bg, "")
		assert.Eq(l, nil)
		assert.Eq(err, xneterr("listen", "α:0", ErrHostDown))
	}, serialOnly)

	// ---- Accept ----

	// listener.Accept vs subnet.Close
	testClose(t, "subnet", func(t *testNet) {
		c, err := t.lα.Accept(bg)
		assert.Eq(c, nil)
		assert.Eq(err, xneterr("accept", "α:1", ErrNetDown))
	})

	// listener.Accept vs host.Close
	testClose(t, "hα", func(t *testNet) {
		c, err := t.lα.Accept(bg)
		assert.Eq(c, nil)
		assert.Eq(err, xneterr("accept", "α:1", ErrHostDown))
	})

	// listener.Accept vs listener.Close
	testClose(t, "lα", func(t *testNet) {
		c, err := t.lα.Accept(bg)
		assert.Eq(c, nil)
		assert.Eq(err, xneterr("accept", "α:1", ErrSockDown))
	})

	// ---- Read/Write ----

	buf := []byte("hello world!")

	// conn.{Read,Write} vs subnet.Close
	testClose(t, "subnet", func(t *testNet) {
		n, err := t.cαβ.Read(buf)
		assert.Eq(n, 0)
		// err can be also EOF because subnet.Close closes cβα too and
		// depending on scheduling we might first get EOF on our end.
		if err != io.EOF {
			assert.Eq(err, xneterr("read", "β:2->α:2", ErrNetDown))
		}
	})
	testClose(t, "subnet", func(t *testNet) {
		n, err := t.cαβ.Write(buf)
		assert.Eq(n, 0)
		assert.Eq(err, xneterr("write", "α:2->β:2", ErrNetDown))
	})

	// conn1.{Read,Write} vs host1.Close
	testClose(t, "hα", func(t *testNet) {
		n, err := t.cαβ.Read(buf)
		assert.Eq(n, 0)
		assert.Eq(err, xneterr("read", "β:2->α:2", ErrHostDown))
	})
	testClose(t, "hα", func(t *testNet) {
		n, err := t.cαβ.Write(buf)
		assert.Eq(n, 0)
		assert.Eq(err, xneterr("write", "α:2->β:2", ErrHostDown))
	})

	// conn1.{Read,Write} vs host2.Close
	testClose(t, "hβ", func(t *testNet) {
		n, err := t.cαβ.Read(buf)
		assert.Eq(n, 0)
		assert.Eq(err, io.EOF)
	})
	testClose(t, "hβ", func(t *testNet) {
		n, err := t.cαβ.Write(buf)
		assert.Eq(n, 0)
		assert.Eq(err, xneterr("write", "α:2->β:2", io.ErrClosedPipe))
	})

	// conn1.{Read,Write} vs conn1.Close
	testClose(t, "cαβ", func(t *testNet) {
		n, err := t.cαβ.Read(buf)
		assert.Eq(n, 0)
		assert.Eq(err, xneterr("read", "β:2->α:2", ErrSockDown))
	})
	testClose(t, "cαβ", func(t *testNet) {
		n, err := t.cαβ.Write(buf)
		assert.Eq(n, 0)
		assert.Eq(err, xneterr("write", "α:2->β:2", ErrSockDown))
	})

	// conn1.{Read,Write} vs conn2.Close
	testClose(t, "cβα", func(t *testNet) {
		n, err := t.cαβ.Read(buf)
		assert.Eq(n, 0)
		assert.Eq(err, io.EOF)
	})
	testClose(t, "cβα", func(t *testNet) {
		n, err := t.cαβ.Write(buf)
		assert.Eq(n, 0)
		assert.Eq(err, xneterr("write", "α:2->β:2", io.ErrClosedPipe))
	})
}

// TestVNetDown verifies that engine shutdown error signal is properly handled.
func TestVNetDown(t0 *testing.T) {
	assert := xtesting.Assert(t0)

	t := newTestNet(t0)
	errSomeProblem := errors.New("some problem")
	SubnetShutdown(t.net, errSomeProblem) // notifier.VNetDown does this

	// SubNetwork.Close = shutdown(nil) and all that interactions were
	// verified in TestClose. Here lets check only that we get proper Close error.
	err := t.net.Close()
	assert.Eq(errors.Cause(err), errSomeProblem)
	assert.Eq(err.Error(), "virtnet \"pipet\": close: some problem")
}
