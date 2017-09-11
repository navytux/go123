// Copyright (C) 2017  Nexedi SA and Contributors.
//                     Kirill Smelkov <kirr@nexedi.com>
//
// This program is free software: you can Use, Study, Modify and Redistribute
// it under the terms of the GNU General Public License version 3, or (at your
// option) any later version, as published by the Free Software Foundation.
//
// You can also Link and Combine this program with other software covered by
// the terms of any of the Open Source Initiative approved licenses and Convey
// the resulting work. Corresponding source of such a combination shall include
// the source code for all other software used.
//
// This program is distributed WITHOUT ANY WARRANTY; without even the implied
// warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
//
// See COPYING file for full licensing terms.

package xmath

import (
	"testing"
)

func TestPow2(t *testing.T) {
	testv := []struct {x, xcpow2 uint64; xclog2 int} {
		{0, 0, 0},
		{1, 1, 0},
		{2, 2, 1},
		{3, 4, 2},
		{4, 4, 2},
		{5, 8, 3},
		{5, 8, 3},
		{6, 8, 3},
		{7, 8, 3},
		{8, 8, 3},
		{9, 16, 4},
		{10, 16, 4},
		{11, 16, 4},
		{12, 16, 4},
		{13, 16, 4},
		{14, 16, 4},
		{15, 16, 4},
		{16, 16, 4},
		{1<<62 - 1, 1<<62, 62},
		{1<<62, 1<<62, 62},
		{1<<62+1, 1<<63, 63},
		{1<<63 - 1, 1<<63, 63},
		{1<<63, 1<<63, 63},
	}

	for _, tt := range testv {
		xcpow2 := CeilPow2(tt.x)
		if xcpow2 != tt.xcpow2 {
			t.Errorf("CeilPow2(%v) -> %v  ; want %v", tt.x, xcpow2, tt.xcpow2)
		}

		xclog2 := CeilLog2(tt.xcpow2)
		if xclog2 != tt.xclog2 {
			t.Errorf("CeilLog2(%v) -> %v  ; want %v", tt.xcpow2, xclog2, tt.xclog2)
		}

		xclog2 = CeilLog2(tt.x)
		if xclog2 != tt.xclog2 {
			t.Errorf("CeilLog2(%v) -> %v  ; want %v", tt.x, xclog2, tt.xclog2)
		}

		xflog2 := FloorLog2(tt.xcpow2)
		xflog2Ok := tt.xclog2
		if tt.x == 0 {
			xflog2Ok = -1
		}
		if xflog2 != xflog2Ok {
			t.Errorf("FloorLog2(%v) -> %v  ; want %v", tt.xcpow2, xflog2, xflog2Ok)
		}

		if tt.x != tt.xcpow2 {
			xflog2Ok--
		}
		xflog2 = FloorLog2(tt.x)
		if xflog2 != xflog2Ok {
			t.Errorf("FloorLog2(%v) -> %v  ; want %v", tt.x, xflog2, xflog2Ok)
		}
	}
}
