// Copyright (C) 2018-2025  Nexedi SA and Contributors.
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

// Package tracetest_test demonstrates how to use package tracetest.
//
// It also serves as set of testcases for tracetest itself.
package tracetest_test

//go:generate gotrace gen .

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"lab.nexedi.com/kirr/go123/tracing"
	"lab.nexedi.com/kirr/go123/tracing/tracetest"
)

// hi and hello are functions that emit "(Hi|Hello), <who>" and can be traced.

//trace:event traceHi(who string)
//trace:event traceHello(who string)
func hi(who string) {
	traceHi(who)
	fmt.Println("Hi,", who)
}
func hello(who string) {
	traceHello(who)
	fmt.Println("Hello,", who)
}

// we use tracing to attach probes to hi and hello, and emit corresponding
// eventHi and eventHello to tracetest.T from there.
type eventHi string
type eventHello string
func setupTracing(t *tracetest.T) *tracing.ProbeGroup {
	pg := &tracing.ProbeGroup{}
	tracing.Setup(func() {
		traceHi_Attach(pg, func(who string) {
			t.RxEvent(eventHi(who))
		})
		traceHello_Attach(pg, func(who string) {
			t.RxEvent(eventHello(who))
		})
	})
	// NOTE pg.Done must be invoked by caller when setup tracing is no longer needed.
	return pg
}

// routeEvent tells to which stream an event should go.
// Here, in example, we use the convention that who comes as "<threadID>·..."
// and we route the event to stream that corresponds to threadID.
func routeEvent(event interface{}) (stream string) {
	who := ""
	switch ev := event.(type) {
	default:
		panic(fmt.Sprintf("unexpected event type %T", event))
	case eventHi:
		who = string(ev)
	case eventHello:
		who = string(ev)
	}

	i := strings.Index(who, "·")
	if i == -1 {
		panic(fmt.Sprintf("who does not have threadID: %q", who))
	}

	return strings.ToLower(who[:i])
}

// verify calls tracetest.Verify on f with first preparing tracing setup and events delivery.
// It also verifies that tracetest detects errors as expected.
func verify(t *testing.T, f func(t *tracetest.T), targvExtra ...string) {
	t.Helper()
	verifyInSubprocess(t, func (t *testing.T) {
		tracetest.Verify(t, func(t *tracetest.T) {
			// setup tracing to deliver trace events to t.
			pg := setupTracing(t)
			defer pg.Done()
			// tell t to which stream an event should go.
			t.SetEventRouter(routeEvent)

			// run test code
			f(t)
		})
	}, targvExtra...)
}


// Test2ThreadsOK demonstrates verifying 2 threads that execute independently.
// There is no concurrency problem here.
func Test2ThreadsOK(t *testing.T) {
	verify(t, func(t *tracetest.T) {
		var wg sync.WaitGroup
		defer wg.Wait()
		wg.Add(2)

		go func() { // thread1
			defer wg.Done()
			hi("T1·A")
			hello("T1·B")
		}()

		go func() { // thread2
			defer wg.Done()
			hello("T2·C")
			hi("T2·D")
		}()

		// assert that events come as expected
		// NOTE in checks t2 vs t1 order does not matter
		t.Expect("t2", eventHello("T2·C"))
		t.Expect("t2", eventHi("T2·D"))
		t.Expect("t1", eventHi("T1·A"))
		t.Expect("t1", eventHello("T1·B"))
	})
}

// TestDeadlock demonstrates deadlock detection.
// XXX also test for wrong decomposition   XXX or is it also covered by this test as well?
func TestDeadlock(t *testing.T) {
	verify(t, func(t *tracetest.T) {
		var wg sync.WaitGroup
		defer wg.Wait()
		wg.Add(1)

		go func() { // thread1
			defer wg.Done()
			hi("T1·A")
		}()

		// the checker expects something on stream "t2", but there is
		// no event sent there -> deadlock.
		t.Expect("t2", eventHi("zzz"))
	}, "-tracetest.deadtime=0.5s")
}

// TestRace demonstrates detection of logical race.
func TestRace(t *testing.T) {
	verify(t, func(t *tracetest.T) {
		var wg sync.WaitGroup
		defer wg.Wait()
		wg.Add(2)

		// 2 threads should synchronize with each other and do step A before B.
		// They do not properly synchronize though, and just happen to
		// usually emit events in expected order due to sleep in T2.
		// Tracetest detects that.
		go func() { // thread1
			defer wg.Done()
			hi("x·A")
		}()

		go func() { // thread2
			defer wg.Done()
			time.Sleep(100*time.Millisecond)
			hi("x·B")
		}()

		t.Expect("x", eventHi("x·A"))
		t.Expect("x", eventHi("x·B"))
	})
}


// other tests (mainly to verify tracetest itself)

// TestDeadlockExtra demonstrates deadlock detection when there is extra event
// not consumed by main checker.
func TestDeadlockExtra(t *testing.T) {
	verify(t, func(t *tracetest.T) {
		var wg sync.WaitGroup
		defer wg.Wait()
		wg.Add(1)

		go func() { // thread 1
			defer wg.Done()
			hi("T1·A")
			hi("T1·Extra")
		}()

		t.Expect("t1", eventHi("T1·A"))
	}, "-tracetest.deadtime=0.5s")
}

// TestExpectType demonstrates Expect asserting with "unexpected event type".
func TestExpectType(t *testing.T) {
	verify(t, func(t *tracetest.T) {
		var wg sync.WaitGroup
		defer wg.Wait()
		wg.Add(1)

		go func() { // thread 1
			defer wg.Done()
			hi("T1·A")
		}()

		t.Expect("t1", eventHello("T1·A"))
	})
}

// TestExpectValue demonstrates Expect asserting with "unexpected event value".
func TestExpectValue(t *testing.T) {
	verify(t, func(t *tracetest.T) {
		var wg sync.WaitGroup
		defer wg.Wait()
		wg.Add(1)

		go func() { // thread 1
			defer wg.Done()
			hi("T1·A")
		}()

		t.Expect("t1", eventHi("T1·B"))
	})
}



// ----------------------------------------

// verifyInSubprocess runs f in subprocess and verifies that its output matches testExpectMap[t.Name].
func verifyInSubprocess(t *testing.T, f func(t *testing.T), targvExtra ...string) {
	t.Helper()
	if os.Getenv("TRACETEST_EX_VERIFY_IN_SUBPROCESS") == "1" {
		f(t)
		return
	}

	// spawn the test in subprocess and verify its output
	expectOK, ok := testExpectMap[t.Name()]
	if !ok {
		panic(fmt.Sprintf("testExpectMap[%q] not defined", t.Name()))
	}
	outputOK := regexp.QuoteMeta(expectOK.output)
	// empty line -> kind of "<BLANKLINE>"
	for {
		__ := strings.ReplaceAll(outputOK, "\n\n", "\n\\s*\n")
		if __ == outputOK {
			break
		}
		outputOK = __
	}
	outputOK = strings.ReplaceAll(outputOK, "<TIME>", ".+s")
	outputOK = strings.ReplaceAll(outputOK, "<LINE>", "[0-9]+")
	outputRe := regexp.MustCompile(outputOK)
	argv := []string{"-test.run="+t.Name()}
	argv = append(argv, targvExtra...)
	cmd := exec.Command(os.Args[0], argv...)
	cmd.Env = append(os.Environ(), "TRACETEST_EX_VERIFY_IN_SUBPROCESS=1")
	bout, err := cmd.CombinedOutput() // NOTE `go test` itself combines everything to stdout only
	out := string(bout)
	ecode := 0
	if testing.Verbose() {
		t.Logf("stdout:\n%s\n", out)
	}
	if err != nil {
		e, ok := err.(*exec.ExitError)
		if !ok {
			// e.g. could not respawn at all
			t.Fatal(err)
		}
		ecode = e.ExitCode()
	}

	bad := ""
	badf := func(format string, argv ...interface{}) {
		bad += fmt.Sprintf(format+"\n", argv...)
	}

	if ecode != expectOK.exitCode {
		badf("exit code: %d  ; expected: %d", ecode, expectOK.exitCode)
	}

	if !outputRe.MatchString(out) {
		badf("unexpected output:\n%s\nwant: ~\n%s\n", out, expectOK.output)
	}

	if bad != "" {
		t.Fatal(bad)
	}
}

// testExpect describes what result to expect from a test.
type testExpect struct {
	exitCode int
	output   string
}
// testExpectMap maps <test name> -> testExpect.
var testExpectMap = map[string]testExpect{
	"Test2ThreadsOK": {0, ""},

	"TestDeadlock":   {1,
`--- FAIL: TestDeadlock (<TIME>)
    example_test.go:157: t2: recv: deadlock waiting for *tracetest_test.eventHi
    example_test.go:157: test shutdown: #streams: 2,  #(pending events): 1
        t1	<- tracetest_test.eventHi T1·A
        # t2

    tracetest.go:<LINE>: chan.go:<LINE>: t1: send: deadlock
`},

	"TestRace":       {1,
`    --- FAIL: TestRace/delay@0(=x:0) (<TIME>)
        example_test.go:183: x: expect: tracetest_test.eventHi:
            want: x·A
            have: x·B
            diff:
            -"x·A"
            +"x·B"
`},

	"TestDeadlockExtra": {1,
`Hi, T1·A
--- FAIL: TestDeadlockExtra (<TIME>)
    tracetest.go:<LINE>: test shutdown: #streams: 1,  #(pending events): 1
        t1	<- tracetest_test.eventHi T1·Extra

    tracetest.go:<LINE>: chan.go:<LINE>: t1: send: deadlock
`},

	"TestExpectType": {1,
`--- FAIL: TestExpectType (<TIME>)
    example_test.go:221: t1: expect: tracetest_test.eventHello:  got tracetest_test.eventHi T1·A
    example_test.go:221: test shutdown: #streams: 1,  #(pending events): 0
        # t1

    tracetest.go:<LINE>: chan.go:<LINE>: t1: send: unexpected event type
`},

	"TestExpectValue": {1,
`--- FAIL: TestExpectValue (<TIME>)
    example_test.go:237: t1: expect: tracetest_test.eventHi:
        want: T1·B
        have: T1·A
        diff:
        -"T1·B"
        +"T1·A"

    example_test.go:237: test shutdown: #streams: 1,  #(pending events): 0
        # t1

    tracetest.go:<LINE>: chan.go:<LINE>: t1: send: unexpected event data
`},
}
