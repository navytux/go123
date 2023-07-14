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

package tracetest
// synchronous channels.

import (
	"errors"
	"flag"
	"fmt"
	"reflect"
	"time"
)

var (
	chatty   = flag.Bool("tracetest.v", false, "verbose: print events as they are sent on trace channels")
	deadTime = flag.Duration("tracetest.deadtime", 3*time.Second, "time after which no events activity is considered to be a deadlock")
)

// _Msg represents message with 1 event sent over _chan.
//
// The goroutine which sent the message will wait for Ack before continue.
type _Msg struct {
	Event interface{}
	ack   chan<- error // nil on Ack; !nil on nak
}

// _chan provides synchronous channel associated with a stream.
//
// It comes with additional property that send blocks until receiving side
// explicitly acknowledges message was received and processed.
//
// New channels must be created via T.newChan.
//
// It is safe to use _chan from multiple goroutines simultaneously.
type _chan struct {
	t    *T            // created for stream <.name> under <.t>
	name string        // name of the channel/stream
	msgq chan *_Msg
	down chan struct{} // becomes ready when closed

	// messages that were not sent due to e.g. detected deadlock.
	// T includes these in final printout for pending events
	// protected by t.mu
	unsentv []*_Msg
}

// Send sends event to a consumer and waits for ack.
// if main testing goroutine detects any problem Send panics.
func (ch *_chan) Send(event interface{}) {
	t := ch.t
	if *chatty {
		fmt.Printf("%s <- %T %v\n", ch.name, event, event)
	}
	ack := make(chan error, 1)
	msg := &_Msg{event, ack}
	unsentWhy := ""
	select {
	case ch.msgq <- msg:
		err := <-ack
		if err != nil {
			t.fatalfInNonMain("%s: send: %s", ch.name, err)
		}
		return

	case <-ch.down:
		unsentWhy = "channel was closed"

	case <-time.After(*deadTime):
		unsentWhy = "deadlock"
	}

	// remember event as still "send-pending"
	t.mu.Lock()
	ch.unsentv = append(ch.unsentv, msg)
	t.mu.Unlock()

	t.fatalfInNonMain("%s: send: %s", ch.name, unsentWhy)
}

// Close closes the sending side of the channel.
func (ch *_chan) Close() {
	close(ch.down) // note - not .msgq
}

// Recv receives message from a producer.
//
// The consumer, after dealing with the message, must send back an ack.
// Must be called from main testing thread.
func (ch *_chan) Recv() *_Msg {
	t := ch.t; t.Helper()
	msg := ch.recv()
	if msg == nil {
		t.Fatalf("%s: recv: deadlock\n", ch.name)
	}
	return msg
}

// RecvInto receives message from a producer, verifies that event type is the
// same as type of *event, and saves received event there.
//
// Must be called from main testing thread.
func (ch *_chan) RecvInto(eventp interface{}) *_Msg {
	t := ch.t; t.Helper()
	msg := ch.recv()
	if msg == nil {
		t.Fatalf("%s: recv: deadlock waiting for %T\n", ch.name, eventp)
	}

	reventp := reflect.ValueOf(eventp)
	if reventp.Type().Elem() != reflect.TypeOf(msg.Event) {
		t.queuenak(msg, "unexpected event type")
		t.Fatalf("%s: expect: %s:  got %T %v", ch.name, reventp.Elem().Type(), msg.Event, msg.Event)
	}

	// *eventp = msg.Event
	reventp.Elem().Set(reflect.ValueOf(msg.Event))

	return msg
}

func (ch *_chan) recv() *_Msg {
	select {
	case msg := <-ch.msgq:
		return msg // ok

	case <-time.After(*deadTime):
		return nil // deadlock
	}
}


// Ack acknowledges the event was processed and unblocks producer goroutine.
func (m *_Msg) Ack() {
	m.ack <- nil
}

// nak tells sender that event verification failed and why.
// it is called only by tracetest internals.
func (m *_Msg) nak(why string) {
	m.ack <- errors.New(why)
}

// nak represents scheduled call to `msg.nak(why)`.
type nak struct {
	msg *_Msg
	why string
}

// queuenak schedules call to `msg.nak(why)`.
func (t *T) queuenak(msg *_Msg, why string) {
	t.nakq = append(t.nakq, nak{msg, why})
}

// newChan creates new _chan channel.
func (t *T) newChan(name string) *_chan {
	// NOTE T ensures not to create channels with duplicate names.
	return &_chan{t: t, name: name, msgq: make(chan *_Msg), down: make(chan struct{})}
}
