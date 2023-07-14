// Copyright (C) 2017-2019  Nexedi SA and Contributors.
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

//go:build !go1.9
// +build !go1.9

package xmath

// CeilPow2 returns minimal y >= x, such that y = 2^i.
func CeilPow2(x uint64) uint64 {
	if x == 0 {
		return x
	}

	l := uint(0)
	h := uint(63)
	for l < h {
		i := (l + h) / 2
		y := uint64(1) << i

		switch {
		case y < x:
			l = i + 1
		case y > x:
			h = i
		default:
			// y == x
			return y
		}
	}

	return 1 << h
}
