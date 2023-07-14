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

// Package tracetest provides infrastructure for testing concurrent systems
// based on synchronous event tracing.
//
// A serial system can be verified by checking that its execution produces
// expected serial stream of events. But concurrent systems cannot be verified
// by exactly this way because events are only partly-ordered with respect to
// each other by causality or so called happens-before relation.
//
// However in a concurrent system one can decompose all events into serial
// streams in which events should be strictly ordered by causality with respect
// to each other. This decomposition in turn allows to verify that in every
// stream events happenned as expected.
//
// Verification of events for all streams can be done by one *sequential*
// process:
//
//   - if events A and B in different streams are unrelated to each other by
//     causality, the sequence of checks models a particular possible flow of
//     time. Notably since events are delivered synchronously and sender is
//     blocked until receiver/checker explicitly confirms event has been
//     processed, by checking either A then B, or B then A allows to check
//     for a particular race-condition.
//
//   - if events A and B in different streams are related to each other by
//     causality (i.e. there is some happens-before relation for them) the
//     sequence of checking should represent that ordering relation.
//
// Basic package usage is as follows:
//
//	func TestSomething(t *testing.T) {
//	    tracetest.Verify(t, func(t *tracetest.T) {
//	        // setup tracing so that events of test system are collected and
//	        // synchronously delivered to t.RxEvent. This can be done with e.g.
//	        // package lab.nexedi.com/kirr/go123/tracing or by other similar means.
//	        ...
//
//	        // tell t to which stream an event should go.
//	        t.SetEventRouter(...)
//
//	        // run the system and verify it produces expected events
//
//	        // <code to start the system>
//	        t.Expect("<stream₁>", eventOk₁)
//	        t.Expect("<stream₂>", eventOk₂)
//	        ...
//
//	        // <code to further control/affect the system>
//	        t.Expect("<stream₃>", eventOk₃)
//	        t.Expect("<stream₄>", eventOk₄)
//	        ...
//	    })
//	}
//
// See example_test.go for more details.
package tracetest

// Note on detection of races
//
// Verify injects delays to empirically detect race conditions and if a test
// incorrectly decomposed its system into serial streams: consider unrelated to
// each other events A and B are incorrectly routed to the same channel. It
// could be so happening that the order of checks on the test side is almost
// always correct and so the error is not visible. However
//
//	if we add delays to delivery of either A or B
//	and test both combinations
//
// we will for sure detect the error as, if A and B are indeed
// unrelated, one of the delay combination will result in events
// delivered to test in different to what it expects order.
//
// the time for delay could be taken as follows:
//
//	- run the test without delay; collect δt between events on particular stream
//	- take delay = max(δt)·10
//
// to make sure there is indeed no different orderings possible on the
// stream, rerun the test N(event-on-stream) times, and during i'th run
// delay i'th event.
//
// See also on this topic:
// http://www.1024cores.net/home/relacy-race-detector
// http://www.1024cores.net/home/relacy-race-detector/rrd-introduction

import (
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kylelemons/godebug/pretty"

	"lab.nexedi.com/kirr/go123/xruntime"
)


// _testing_TB is alias for testing.TB that is non-public when embedded into a struct.
type _testing_TB = testing.TB

// T is similar to testing.T and represents tracetest test environment.
//
// It is passed by Verify and Run to tested function.
//
// Besides testing.TB it provides
//
//	.RxEvent	-- to where events should be synchronously delivered by the test
//	.SetEventRouter	-- to tell T to which stream an event should go
//	.Expect		-- to assert expectation of an event on a stream
type T struct {
	_testing_TB

	mu             sync.Mutex
	streamTab      map[/*stream*/string]*_chan // where events on stream are delivered; set to nil on test shutdown
	routeEvent     func(event interface{}) (stream string)
	tracev         []eventTrace // record of events as they happen
	delayInjectTab map[/*stream*/string]*delayInjectState

	nakq []nak    // naks queued to be sent after Fatal
	logq []string // queued log messages prepared in fatalfInNonMain
}

// eventTrace keeps information about one event T received via RxEvent.
type eventTrace struct {
	t      time.Time   // time of receive; monotonic
	stream string
	event  interface{}
}

// delayInjectState is used by delay-injector to find out for which event on a
// stream a delay should be injected.
type delayInjectState struct {
	seqno   int           // current sequence number of event on this stream.
	delayAt int           // event with `seqno == delayAt` will be delayed
	delayT  time.Duration // by delayT time.
}


// Run runs f under tracetest environment.
//
// It is similar to Verify but f is ran only once.
// Run does not check for race conditions.
func Run(t testing.TB, f func(t *T)) {
	run(t, f, nil)
}

// run serves Run and Verify: it creates T that wraps t, and runs f under T.
func run(t testing.TB, f func(t *T), delayInjectTab map[string]*delayInjectState) *T {
	tT := &T{
		_testing_TB:    t,
		streamTab:      make(map[string]*_chan),
		delayInjectTab: delayInjectTab,
	}

	// verify in the end that no events are left unchecked / unconsumed,
	// e.g. sent to RxEvent, but not received. Nak them if they are and fail.
	//
	// NOTE this complements T.Fatal and friends, because a test might
	// think it completes successfully, but leaves unconsumed events behind it.
	defer func() {
		nnak := tT.closeStreamTab()
		if nnak != 0 {
			tT.Fail()
		}
		// log messages queued by fatalfInNonMain
		for _, msg := range tT.logq {
			// TODO try to log without hereby file:line, because msg
			// already has file:line corresponding to logged event source location.
			tT.Log(msg)
		}
	}()

	f(tT)
	return tT
}

// Verify verifies a test system.
//
// It runs f under T environment, catching race conditions, deadlocks and
// unexpected events. f is rerun several times and should not alter its
// behaviour from run to run.
func Verify(t *testing.T, f func(t *T)) {
	// run f once. This produces initial trace of events.
	tT0 := run(t, f, nil)

	// now, if f succeeds, verify f with injected delays.
	if tT0.Failed() {
		return
	}

	trace0 := tT0.tracev
	if len(trace0) < 2 {
		return
	}
	streams0 := streamsOfTrace(trace0)

	// sort trace0 by time just in case - events might come from multiple
	// CPUs simultaneously, and so for close events they might be added to
	// tracev not in time order.
	sort.Slice(trace0, func(i, j int) bool {
		return trace0[i].t.Before(trace0[j].t)
	})

	// find out max(δt) in between events
	var δtMax time.Duration
	for i := 1; i < len(trace0); i++ {
		δt := trace0[i].t.Sub(trace0[i-1].t)
		if δt > δtMax {
			δtMax = δt
		}
	}

	// retest f with 10·δtMax delay injected at i'th event
	delayT    := 10*δtMax            // TODO make sure it < deadTime
	delayTmin := 10*time.Millisecond // make sure delayT ≥ 10ms
	if delayT < delayTmin {
		delayT = delayTmin
	}
	for i := 0; i < len(trace0); i++ {
		// stream and on-stream sequence number for i'th global event
		stream := trace0[i].stream
		istream := -1
		for j := 0; j <= i; j++ {
			if trace0[j].stream == stream {
				istream++
			}
		}

		t.Run(fmt.Sprintf("delay@%d(=%s:%d)", i, stream, istream), func(t *testing.T) {
			tT := run(t, f, map[string]*delayInjectState{
				stream: &delayInjectState{
					delayAt: istream,
					delayT:  delayT,
				},
			})

			// verify that streams are the same from run to run
			if tT.Failed() {
				return
			}
			streams := streamsOfTrace(tT.tracev)
			if !reflect.DeepEqual(streams, streams0) {
				tT.Fatalf("streams are not the same as in the first run:\n"+
					  "first: %s\nnow:   %s\ndiff:\n%s\n\n",
					  streams0, streams, pretty.Compare(streams0, streams))
			}
		})
	}
}


// T overrides FailNow/Fatal/Fatalf to also cancel all in-progress sends.
func (t *T) FailNow() {
	t.Helper()
	_ = t.closeStreamTab()
	t._testing_TB.FailNow()
}

func (t *T) Fatal(argv ...interface{}) {
	t.Helper()
	t.Log(argv...)
	t.FailNow()
}

func (t *T) Fatalf(format string, argv ...interface{}) {
	t.Helper()
	t.Logf(format, argv...)
	t.FailNow()
}

// closeStreamTab prints details about pending events on streamTab, naks them
// and closes all channels. It returns the number of naked messages.
func (t *T) closeStreamTab() (nnak int) {
	t.Helper()

	// mark streamTab no longer operational
	t.mu.Lock()
	defer t.mu.Unlock()
	streamTab := t.streamTab
	t.streamTab = nil

	if streamTab == nil {
		return // already closed
	}

	// print details about pending events and all streams
	type sendInfo struct{ch *_chan; msg *_Msg}
	var sendv  []sendInfo // sends are pending here
	var quietv []*_chan   // this channels are quiet

	// order channels by name
	var streams []string
	for __ := range streamTab {
		streams = append(streams, __)
	}
	sort.Slice(streams, func(i, j int) bool {
		return strings.Compare(streams[i], streams[j]) < 0
	})

	for _, stream := range streams {
		ch := streamTab[stream]
		quiet := true

		// check whether someone is sending on channels without blocking.
	loop:	// loop because there might be several concurrent pending sends to particular channel.
		for {
			select {
			case msg := <-ch.msgq:
				sendv = append(sendv, sendInfo{ch, msg})
				quiet = false
			default:
				break loop
			}
		}
		// include ch.unsentv into pending as well (we want to show
		// such events as pending even if corresponding send deadlocked).
		for _, msg := range ch.unsentv {
			sendv = append(sendv, sendInfo{ch, msg})
			quiet = false
		}

		if quiet {
			quietv = append(quietv, ch)
		}
	}

	pending := fmt.Sprintf("test shutdown: #streams: %d,  #(pending events): %d\n", len(streams), len(sendv))
	for _, __ := range sendv {
		pending += fmt.Sprintf("%s\t<- %T %v\n", __.ch.name, __.msg.Event, __.msg.Event)
	}
	for _, ch := range quietv {
		pending += fmt.Sprintf("# %s\n", ch.name)
	}

	// log the details and nak senders that we received from.
	// nak them only after details printout, so that our text comes first,
	// and their "panics" don't get intermixed with it.
	t.Log(pending)
	for _, __ := range t.nakq {
		__.msg.nak(__.why)
		nnak++
	}
	t.nakq = nil
	for _, __ := range sendv {
		__.msg.nak("canceled (test failed)")
		nnak++
	}
	// in any case close channel where future Sends may arrive so that they will "panic" too.
	for _, ch := range streamTab {
		ch.Close()
	}

	return nnak
}

// streamsOfTrace returns sorted list of all streams present in a trace.
func streamsOfTrace(tracev []eventTrace) []string {
	streams := make(map[string]struct{})
	for _, t := range tracev {
		streams[t.stream] = struct{}{}
	}
	streamv := []string{}
	for stream := range streams {
		streamv = append(streamv, stream)
	}
	sort.Strings(streamv)
	return streamv
}


// ---- events delivery + Expect ----


// SetEventRouter tells t to which stream an event should go.
//
// It should be called not more than once.
// Before SetEventRouter is called, all events go to "default" stream.
func (t *T) SetEventRouter(routeEvent func(event interface{}) (stream string)) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.routeEvent != nil {
		panic("double call to SetEventRouter")
	}
	t.routeEvent = routeEvent
}


// chanForStream returns channel corresponding to stream.
// must be called under mu.
func (t *T) chanForStream(stream string) *_chan {
	if t.streamTab == nil {
		return nil // t is no longer operational after e.g. deadlock
	}

	ch, ok := t.streamTab[stream]
	if !ok {
		ch = t.newChan(stream)
		t.streamTab[stream] = ch
	}
	return ch
}

// RxEvent should be synchronously called from test system when an event occurs.
//
// The sequential process of the test system where event originated should be
// paused until RxEvent returns. This requirement can be usually met via
// inserting t.RxEvent() call into the code that produces the event.
func (t *T) RxEvent(event interface{}) {
	t0 := time.Now()
	stream := ""
	t.mu.Lock()
	if t.routeEvent != nil {
		stream = t.routeEvent(event)
	}
	if stream == "" {
		stream = "default"
	}
	t.tracev = append(t.tracev, eventTrace{t0, stream, event})
	ch := t.chanForStream(stream)

	var delay time.Duration
	d, ok := t.delayInjectTab[stream]
	if ok {
		if d.seqno == d.delayAt {
			delay = d.delayT
		}
		d.seqno++
	}

	t.mu.Unlock()

	if ch == nil {
		t.fatalfInNonMain("%s: (pre)send: canceled (test failed)", stream)
	}

	if delay != 0 {
		time.Sleep(delay)
	}

	ch.Send(event)
}

// xget1 gets 1 event in place and checks it has expected type
//
// if checks do not pass - fatal testing error is raised
func (t *T) xget1(stream string, eventp interface{}) *_Msg {
	t.Helper()

	t.mu.Lock()
	ch := t.chanForStream(stream)
	t.mu.Unlock()

	if ch == nil {
		t.Fatalf("%s: recv: canceled (test failed)", stream)
	}

	return ch.RecvInto(eventp)
}

// Expect receives next event on stream and verifies it to be equal to eventOK.
//
// If check is successful ACK is sent back to event producer.
// If check does not pass - fatal testing error is raised.
func (t *T) Expect(stream string, eventOK interface{}) {
	t.Helper()
	msg := t.expect1(stream, eventOK)
	msg.Ack()
}

// TODO ExpectNoACK? (then it would be possible to receive events from 2
// streams; have those 2 processes paused and inspect their state. After
// inspection unpause both)

// TODO Recv? (to receive an event for which we don't know type or value yet)

// TODO Select? (e.g. Select("a", "b") to fetch from either "a" or "b")

// expect1 receives next event on stream and verifies it to be equal to eventOK (both type and value).
//
// if checks do not pass - fatal testing error is raised.
func (t *T) expect1(stream string, eventExpect interface{}) *_Msg {
	t.Helper()

	reventExpect := reflect.ValueOf(eventExpect)

	reventp := reflect.New(reventExpect.Type())
	msg := t.xget1(stream, reventp.Interface())
	revent := reventp.Elem()

	if !reflect.DeepEqual(revent.Interface(), reventExpect.Interface()) {
		t.queuenak(msg, "unexpected event data")
		t.Fatalf("%s: expect: %s:\nwant: %v\nhave: %v\ndiff:\n%s\n\n",
			stream,
			reventExpect.Type(), reventExpect, revent,
			pretty.Compare(reventExpect.Interface(), revent.Interface()))
	}

	return msg
}

// fatalfInNonMain should be called for fatal cases in non-main goroutines instead of panic.
//
// we don't panic because it will stop the process and prevent the main
// goroutine to print detailed reason for e.g. deadlock or other error.
func (t *T) fatalfInNonMain(format string, argv ...interface{}) {
	t.Helper()

	if !strings.HasSuffix(format, "\n") {
		format += "\n"
	}
	msg := fmt.Sprintf(format, argv...)
	msg += fmt.Sprintf("%s\n", debug.Stack())

	// manually include file:line so that message is logged with correct
	// location when emitted via logq.
	// XXX t.Helper() not taken into account
	f := xruntime.Traceback(2)[0] // XXX we need only first caller, not full traceback
	msg = fmt.Sprintf("%s:%d: %s", filepath.Base(f.File), f.Line, msg)

	// serialize fatal log+traceback printout, so that such printouts from
	// multiple goroutines do not get intermixed.
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.streamTab == nil {
		// t is over -> log directly.
		// make sure to prefix log message the same way as would be
		// done when messages are logged via .logq .
		t.logFromTracetest_go(msg)
	} else {
		// remember msg to be logged when t is done so that non-main
		// log output always come after main printout. The messages
		// won't be intermixed because t.Log is serialized internally.
		t.logq = append(t.logq, msg)
	}

	t.Fail()
	runtime.Goexit()
}

// logFromTracetest_go calls t.Log without wrapping it with t.Helper().
//
// as the result the message is prefixed with tracetest.go:<LINE>, not the
// location of fatalfInNonMain caller.
func (t *T) logFromTracetest_go(msg string) {
	t.Log(msg)
}
