// Copyright (C) 2017-2021  Nexedi SA and Contributors.
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

package xnet
// network tracing

import (
	"context"
	"net"
	"sync/atomic"
)

// NetTrace wraps underlying networker with IO tracing layer.
//
// Tracing is done via calling trace func right after corresponding networking
// event happenned.  No synchronization for notification is performed - if one
// is required tracing func must implement such synchronization itself.
//
// only initiation events are traced:
//
// 1. Tx only (no Rx):
//   - because Write, contrary to Read, never writes partial data on non-error
//   - because in case of pipenet tracing writes only is enough to get whole network exchange picture
//
// 2. Dial only (no Accept)
//   - for similar reasons.
//
// WARNING NetTrace functionality is currently very draft.
func NetTrace(inner Networker, tracerx TraceReceiver) *Tracer {
	return &Tracer{inner, tracerx, 1}
}

// TraceReceiver is the interface that needs to be implemented by network trace receivers.
type TraceReceiver interface {
	TraceNetDial(*TraceDial)
	TraceNetConnect(*TraceConnect)
	TraceNetListen(*TraceListen)
	TraceNetTx(*TraceTx)
	// XXX +TraceNetClose?
}

// TraceDial is event corresponding to network dial start.
type TraceDial struct {
	// XXX also put networker?
	Dialer, Addr string
}

// TraceConnect is event corresponding to established network connection.
type TraceConnect struct {
	// XXX also put networker?
	Src, Dst net.Addr
	Dialed   string
}

// TraceListen is event corresponding to network listening.
type TraceListen struct {
	// XXX also put networker?
	Laddr net.Addr
}

// TraceTx is event corresponding to network transmission.
type TraceTx struct {
	// XXX also put network somehow?
	Src, Dst net.Addr
	Pkt      []byte
}

// Tracer wraps underlying Networker to emit events on networking operations.
//
// Create it via NetTrace.
type Tracer struct {
	inner Networker
	rx    TraceReceiver
	on    int32 // atomic (tracing can be enabled/disabled at runtime)
}

// TraceOn tells the tracer to (re)enable delivery of trace events.
func (t *Tracer) TraceOn() {
	atomic.StoreInt32(&t.on, 1)
}

// TraceOff tells tracer to disable delivery of trace events.
func (t *Tracer) TraceOff() {
	atomic.StoreInt32(&t.on, 0)
}

func (t *Tracer) enabled() bool {
	return (atomic.LoadInt32(&t.on) != 0)
}

// Network implements Networker.
func (t *Tracer) Network() string {
	return t.inner.Network() // XXX + "+trace" ?
}

// Name implements Networker.
func (t *Tracer) Name() string {
	return t.inner.Name()
}

// Close implements Networker.
func (t *Tracer) Close() error {
	// XXX +trace?
	return t.inner.Close()
}

// Dial implements Networker.
func (t *Tracer) Dial(ctx context.Context, addr string) (net.Conn, error) {
	if t.enabled() {
		t.rx.TraceNetDial(&TraceDial{Dialer: t.inner.Name(), Addr: addr})
	}
	c, err := t.inner.Dial(ctx, addr)
	if err != nil {
		return nil, err
	}
	if t.enabled() {
		t.rx.TraceNetConnect(&TraceConnect{Src: c.LocalAddr(), Dst: c.RemoteAddr(), Dialed: addr})
	}
	return &traceConn{t, c}, nil
}

// Listen implements Networker.
func (t *Tracer) Listen(ctx context.Context, laddr string) (Listener, error) {
	// XXX +TraceNetListenPre ?
	l, err := t.inner.Listen(ctx, laddr)
	if err != nil {
		return nil, err
	}
	if t.enabled() {
		t.rx.TraceNetListen(&TraceListen{Laddr: l.Addr()})
	}
	return &netTraceListener{t, l}, nil
}

// netTraceListener wraps net.Listener to wrap accepted connections with traceConn.
type netTraceListener struct {
	t        *Tracer
	Listener
}

func (ntl *netTraceListener) Accept(ctx context.Context) (net.Conn, error) {
	c, err := ntl.Listener.Accept(ctx)
	if err != nil {
		return nil, err
	}
	return &traceConn{ntl.t, c}, nil
}

// traceConn wraps net.Conn and notifies tracer on Writes.
type traceConn struct {
	t        *Tracer
	net.Conn
}

func (tc *traceConn) Write(b []byte) (int, error) {
	// XXX +TraceNetTxPre ?
	n, err := tc.Conn.Write(b)
	if err == nil {
		if tc.t.enabled() {
			tc.t.rx.TraceNetTx(&TraceTx{Src: tc.LocalAddr(), Dst: tc.RemoteAddr(), Pkt: b})
		}
	}
	return n, err
}
