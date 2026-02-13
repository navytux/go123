// Copyright (C) 2026  Nexedi SA and Contributors.
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
	"fmt"
	"reflect"
	"runtime"
	"testing"
)

func TestGetG(t *testing.T) {
	// verify that getg returns reasonable result by checking g.goid and g.startpc
	n := 1000
	for i := 0; i < n; i++ {
		// NOTE we are spawning ginfo without arguments so that startpc goes
		//      directly to ginfo instead of to closure
		go ginfo()
	}

	ginfo_pc   := reflect.ValueOf(ginfo).Pointer()
	ginfo_func := runtime.FuncForPC(ginfo_pc)
	for i := 0; i < n; i++ {
		tg := <-tginfoq
		if tg.err != nil {
			t.Error(tg.err)
		}
		if !(tg.g_goid == tg.goid && tg.g_startpc == ginfo_pc) {
			g_start_func := runtime.FuncForPC(tg.g_startpc)
			t.Errorf("goroutine %d: have goid=%d  startpc=0x%x (%s);  want goid=%d  startpc=0x%x (%s)",
				tg.goid, tg.g_goid, tg.g_startpc, g_start_func.Name(), tg.goid, ginfo_pc, ginfo_func.Name())
		}
	}
}

var tginfoq = make(chan tGInfo)
type tGInfo struct {
	g_goid    uint64  // g.goid
	g_startpc uintptr // g.startpc
	goid      uint64  // goid retrieved via runtime.Stack
	err       error
}

//go:noinline
func ginfo() {
	var tg tGInfo

	// retrieve g info
	g := getg()
	tg.g_goid    = uint64(g.goid)
	tg.g_startpc = g.startpc

	// retrieve goid printed by runtime.Stack
	stkb := make([]byte, 1024)
	n := runtime.Stack(stkb, false)
	stk := string(stkb[:n])

	// goroutine <goid> [running]:
	// ...
	n, err := fmt.Sscanf(stk, "goroutine %d ", &tg.goid)
	if !(n == 1 && err == nil) {
		tg.err = fmt.Errorf("cannot retrieve goid from traceback\n%s", stk)
	}

	tginfoq <- tg
}
