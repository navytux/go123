// Copyright (C) 2017  Nexedi SA and Contributors.
//                     Kirill Smelkov <kirr@nexedi.com>
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
	"sync/atomic"
	"testing"
	"time"
)

func TestStartStopTheWorld(t *testing.T) {
	var x, stop int32
	ready := make(chan int)

	// g2
	go func() {
		// make sure the thread running this goroutine is different from thread for main g.
		// this way we can be sure there are 2 OS threads in action and communicating via busywait should work.
		runtime.LockOSThread()
		ready <- 0

		for atomic.LoadInt32(&stop) == 0 {
			atomic.AddInt32(&x, 1)

			// XXX as of go19 tight loops are not preemptible (golang.org/issues/10958)
			//     -> explicitly make sure we do not miss STW request.
			runtime.Gosched()
		}
	}()


	// wait for spawned goroutine to jump into its own thread
	<-ready

	// verify g and g2 are indeed running in parallel
	check_g_g2_running := func(bad string) {
		xprev := atomic.LoadInt32(&x)
		xnext := xprev
		nδ := 0
		tstart := time.Now()
		for nδ < 100 && time.Now().Sub(tstart) < time.Second {
			xnext = atomic.LoadInt32(&x)
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
	StopTheWorld("just for my reason")

	xprev := atomic.LoadInt32(&x)
	xnext := xprev
	nδ := 0
	tstart := time.Now()
	for time.Now().Sub(tstart) < time.Second {
		for i := 0; i < 100; i++ {
			xnext = atomic.LoadInt32(&x)
			if xnext != xprev {
				nδ += 1
				xprev = xnext
			}
		}
	}

	StartTheWorld()

	if nδ != 0 {
		t.Fatalf("g2 modified x at least %d times while the world was stopped", nδ)
	}

	// make sure g2 is now running again
	check_g_g2_running("g2 did not restarted after StartTheWorld")

	atomic.StoreInt32(&stop, 1)
}
