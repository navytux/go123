// Copyright (C) 2016-2026  Nexedi SA and Contributors.
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
// stop-the-world that should probably be in public xruntime, but I'm (yet)
// hesitating to expose the API to public.

import (
	"math"
	"runtime/debug"
	"sync"
	_ "unsafe"
)


// DoWithStoppedWorld runs f with the world stopped.
//
// The goroutine, that runs f, remains the only one who is running, with others
// goroutines stopped at safe GC points.
// Inside f it requires careful programming as many things that normally work lead to
// fatal errors when the world is stopped - for example using timers would be
// invalid, but adjusting plain values in memory is ok.
//
// f must be marked with go:nosplit .
//
// It is not required to mark f with neither go:nowritebarrier nor go:writebarrierrec.
// The compiler does not allow those directives for user code anyway.
func DoWithStoppedWorld(f func()) {
	// disable GC during f run to make sure that GC's concurrent Mark phase will
	// not be running because it activates runtime write barrier and WB is not safe
	// to use during STW.
	doWithDisabledGC(func() {
		doWithStoppedWorld(f)
	})
}


var nogcMu sync.Mutex

// doWithDisabledGC runs f with guarantee that GC is completely disabled during the run.
func doWithDisabledGC(f func()) {
	// lock so that there is no mix of doWithDisabledGC concurrently
	nogcMu.Lock()
	defer nogcMu.Unlock()

	memlimit  := setMemoryLimit(math.MaxInt64)
	gcpercent := debug.SetGCPercent(-1)
	defer func() {
		gcpercent = debug.SetGCPercent(gcpercent)
		if gcpercent != -1 {
			panic("doWithDisabledGC: GCPercent mutated during do")
		}
	}()
	defer func() {
		memlimit = setMemoryLimit(memlimit)
		if memlimit != math.MaxInt64 {
			panic("doWithDisabledGC: MemoryLimit mutated during do")
		}
	}()

	assertWriteBarrierDisabled() // because GC is disabled
	f()
}


//go:linkname runtime_writeBarrier runtime.writeBarrier
var runtime_writeBarrier struct {
	enabled bool
	// ...
}

//go:nosplit
func assertWriteBarrierDisabled() {
	// assert that writeBarrier is disabled
	// We mostly assert this under STW because else else any executed code that changes
	// a pointer will crash calling runtime.gcWriteBarrier because ptr change in the
	// following go code
	//
	//	var ptr *int
	//	ptr = ptrval
	//
	// is translated to something like this by the compiler:
	//
	//		; AX = ptrval
	// 		CMPL    runtime.writeBarrier(SB), $0
	// 		JEQ     store
	// 		MOVQ    ptr(SB), CX
	// 		CALL    runtime.gcWriteBarrier2(SB)
	// 		MOVQ    AX, (R11)
	// 		MOVQ    CX, 8(R11)
	//     store:	MOVQ    AX, ptr(SB)
	//
	// by verifying that writeBarrier.enabled=0 we make sure that codepath that calls
	// runtime.gcWriteBarrier will not be activated and effectively it will be only
	// the last store instruction.
	//
	// User code cannot use go:nowritebarrier and by doing this check we make sure that
	// runtime.gcWriteBarrier2() codepath won't be activated.
	if runtime_writeBarrier.enabled {
		panic("BUG: write barrier enabled")
	}
}
