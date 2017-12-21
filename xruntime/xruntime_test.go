// Copyright (C) 2015-2017  Nexedi SA and Contributors.
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
	"testing"

	"lab.nexedi.com/kirr/go123/my"
)

func f333(tb1, tb2 *[]runtime.Frame) {
	// NOTE keeping tb1 and tb2 updates on the same line - so that .Line in both frames is the same
	*tb1 = append(*tb1, my.Frame()); *tb2 = Traceback(1)
}

func f222(tb1, tb2 *[]runtime.Frame) {
	*tb1 = append(*tb1, my.Frame()); f333(tb1, tb2)
}

func f111(tb1, tb2 *[]runtime.Frame) {
	*tb1 = append(*tb1, my.Frame()); f222(tb1, tb2)
}

func TestTraceback(t *testing.T) {
	var tb1, tb2 []runtime.Frame
	f111(&tb1, &tb2)

	if len(tb1) != 3 {
		t.Fatalf("len(tb1) = %v  ; must be 3", len(tb1))
	}

	tb1[0], tb1[1], tb1[2] = tb1[2], tb1[1], tb1[0] // reverse

	for i, f1 := range tb1 {
		f2 := tb2[i]

		// pc are different; everything else must be the same
		f1.PC = 0
		f2.PC = 0

		if f1 != f2 {
			t.Errorf("traceback #%v:\nhave %v\nwant %v", i, f2, f1)
		}
	}
}
