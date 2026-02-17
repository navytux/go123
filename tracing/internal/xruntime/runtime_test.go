// Copyright (C) 2017-2026  Nexedi SA and Contributors.
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

package xruntime

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStartStopTheWorld(t *testing.T) {
	var stop int32
	defer atomic.StoreInt32(&stop, 1)
	ready := make(chan int)

	// g2
	go func() {
		// make sure the thread running this goroutine is different from thread for main g.
		// this way we can be sure there are 2 OS threads in action and communicating via busywait should work.
		runtime.LockOSThread()
		ready <- 0

		for atomic.LoadInt32(&stop) == 0 {
			atomic.AddInt32(&tstw.x, 1)

			// XXX as of go19 tight loops are not preemptible (golang.org/issues/10958)
			//     -> explicitly make sure we do not miss STW request.
			runtime.Gosched()
		}
	}()


	// wait for spawned goroutine to jump into its own thread
	<-ready

	// verify g and g2 are indeed running in parallel
	check_g_g2_running := func(bad string) {
		xprev := atomic.LoadInt32(&tstw.x)
		xnext := xprev
		nδ := 0
		tstart := time.Now()
		for nδ < 100 && time.Now().Sub(tstart) < time.Second {
			xnext = atomic.LoadInt32(&tstw.x)
			if xnext != xprev {
				nδ += 1
				xprev = xnext
			}
		}

		if nδ == 0 {
			t.Fatal(bad)
		}
	}

	check_g_g2_running("g and g2 are not running in parallel")

	// now stop the world and for 1s make sure g2 is not running in parallel with us
	tstw.nδ = 0
	tstw.nrun = 0
	tstw.tstart = time.Now()
	DoWithStoppedWorld(fstw)

	if tstw.nrun != 1 {
		t.Fatalf("DoWithStoppedWorld: ran given func %d times instead of exactly once", tstw.nrun)
	}

	if tstw.nδ != 0 {
		t.Fatalf("g2 modified x at least %d times while the world was stopped", tstw.nδ)
	}

	// make sure g2 is now running again
	check_g_g2_running("g2 did not restarted after StartTheWorld")
}

var tstw struct {
	x int32

	nδ     int
	nrun   int
	tstart time.Time
}

//go:nosplit
func fstw() {
	tstw.nrun += 1
	xprev := atomic.LoadInt32(&tstw.x)
	xnext := xprev
	for time.Now().Sub(tstw.tstart) < time.Second {
		for i := 0; i < 100; i++ {
			xnext = atomic.LoadInt32(&tstw.x)
			if xnext != xprev {
				tstw.nδ += 1
				xprev = xnext
			}
		}
	}
}


// verify that STW entry/exit is safe wrt simultaneous runtime·write .
//
// On go ≥ 1.23 doWithStoppedWorld works via installing hook into runtime.write .
// This used to deadlock or crash in the presence of simultaneous calls to runtime.write
// from other goroutines.
func TestStartStopTheWorld_vs_runtimeWrite(t *testing.T) {
	// problem is reproduced only with GOMAXPROCS ≥ 2
	minprocs := 2
	maxprocs := runtime.GOMAXPROCS(-1)
	if minprocs < maxprocs {
		minprocs = maxprocs
	}
	runtime.GOMAXPROCS(minprocs)
	defer runtime.GOMAXPROCS(maxprocs)

	// surrounding load that invokes runtime.write non-stop
	var wg sync.WaitGroup
	defer wg.Wait()
	var done int32
	defer atomic.StoreInt32(&done, 1)
	ready := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		runtime.LockOSThread()
		close(ready)
		defer println()

		for atomic.LoadInt32(&done) == 0 {
			print(".")
		}

	}()

	<-ready

	// enter/exit STW many times ...
	nstw := 100
	tstw2.nrun = 0
	for i := 0; i < nstw; i++ {
		DoWithStoppedWorld(fstw2)
		runtime.Gosched()
	}

	if tstw2.nrun != nstw {
		t.Fatalf("DoWithStoppedWorld: ran given func %d times instead of %d", tstw2.nrun, nstw)
	}
}

var tstw2 struct {
	nrun int
}

//go:nosplit
func fstw2() {
	tstw2.nrun += 1
}
