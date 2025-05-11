// Copyright (C) 2025  Nexedi SA and Contributors.
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

//go:build go1.23
// +build go1.23

package xruntime

import (
	"runtime/debug"
	"sync"
	"sync/atomic"
	"unsafe"

	_ "go4.org/unsafe/assume-no-moving-gc"
)

//go:linkname runtime_overrideWrite runtime.overrideWrite
//go:linkname runtime_write runtime.write

var runtime_overrideWrite func(fd uintptr, p unsafe.Pointer, n int32) int32
func runtime_write(fd uintptr, p unsafe.Pointer, n int32) int32

// go keeps func as one word, pointing to another struct{func_addr, this} with two words.
// we need to swap that func's top word atomically and so we need to make sure we can really
// do this by asserting that the size of func is the same as the size of one word.
func init() {
	if unsafe.Sizeof(runtime_overrideWrite) != unsafe.Sizeof(unsafe.Pointer(nil)) {
		panic("build bug: sizeof(func) != sizeof(pointer)")
	}
}


var withSTWMu sync.Mutex

func doWithStoppedWorld(f func()) {
	// protect multiple simultaneous doWithStoppedWorld from e.g. clobbering runtime_overrideWrite on restore
	withSTWMu.Lock()
	defer withSTWMu.Unlock()

	STWdone := make(chan struct{})

	// foverrideWrite invokes f when write is called the first time	with special fd value
	var ncall atomic.Int32
	fdhook := uintptr(0);  fdhook -= 1  // -1U
	foverrideWrite := func(fd uintptr, p unsafe.Pointer, n int32) int32 {
		// some call to write happens simultaneously to us when we are either:
		// - entering STW but not yet there, or
		// - after exiting STW but not yet restored overrideWrite to its saved value.
		// wait till STW is over and retry the write
		if fd != fdhook {
			<-STWdone
			return runtime_write(fd, p, n)
		}

		// a call to write with special fd should happen only from under STW triggered by debug.WriteHeapDump below
		if ncall.Add(1) == 1 {
			f()
		}
		return n
	}

	// swap runtime.overrideWrite to foverrideWrite
	pruntime_overrideWrite := (*unsafe.Pointer)(unsafe.Pointer(&runtime_overrideWrite))
	var oldWrite unsafe.Pointer
	var oldWriteObj func() // to keep oldWrite alive while our hook is installed instead
	for {
		oldWrite = atomic.LoadPointer(pruntime_overrideWrite)
		oldWriteObj = *(*func())(unsafe.Pointer(&oldWrite))
		ok := atomic.CompareAndSwapPointer(pruntime_overrideWrite, oldWrite,
			*((*unsafe.Pointer)((unsafe.Pointer(&foverrideWrite)))))
		if ok {
			break
		}
	}
	_ = oldWriteObj

	// debug.WriteHeapDump enters STW and invokes runtime.write many times inside
	// use it as a way to enter STW and hook our code there
	debug.WriteHeapDump(fdhook)

	// restore runtime.overrideWrite
	atomic.StorePointer(pruntime_overrideWrite, oldWrite)

	// STW is over, unpause waiting inflight writes to regular fds
	close(STWdone)
}
