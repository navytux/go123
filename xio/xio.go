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

// Package xio provides addons to standard package io.
package xio

import "io"

// CountedReader is an io.Reader that count total bytes read.
type CountedReader struct {
	r     io.Reader
	nread int64
}

func (cr *CountedReader) Read(p []byte) (int, error) {
	n, err := cr.r.Read(p)
	cr.nread += int64(n)
	return n, err
}

// InputOffset returns the number of bytes read.
func (cr *CountedReader) InputOffset() int64 {
	return cr.nread
}

// CountReader wraps r with CountedReader
func CountReader(r io.Reader) *CountedReader {
	return &CountedReader{r, 0}
}
