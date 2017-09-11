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

// +build go1.9

// Package xmath provides addons to std math package
package xmath

import (
	"math/bits"
)

// CeilPow2 returns minimal y >= x, such that y = 2^i
func CeilPow2(x uint64) uint64 {
	switch bits.OnesCount64(x) {
	case 0, 1:
		return x // either 0 or 2^i already
	default:
		return 1 << uint(bits.Len64(x))
	}
}

// CeilLog2 returns minimal i: 2^i >= x
func CeilLog2(x uint64) int {
	switch bits.OnesCount64(x) {
	case 0:
		return 0
	case 1:
		return bits.Len64(x) - 1
	default:
		return bits.Len64(x)
	}
}

// FloorLog2 returns maximal i: 2^i <= x
//
// x=0 gives -> -1.
func FloorLog2(x uint64) int {
	switch bits.OnesCount64(x) {
	case 0:
		return -1
	default:
		return bits.Len64(x) - 1
	}
}


// XXX if needed: NextPow2 (y > x, such that y = 2^i) is
//	1 << bits.Len64(x)
