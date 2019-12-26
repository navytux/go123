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

// Package xbufio provides addons to std package bufio.
package xbufio

import (
	"bufio"
	"context"
	"io"

	"lab.nexedi.com/kirr/go123/xio"
)

// Reader is a bufio.Reader that also reports current logical position in input stream.
type Reader struct {
	*bufio.Reader
	cr *xio.CountedReader
}

func NewReader(r io.Reader) *Reader {
	// idempotent(Reader)
	if r, ok := r.(*Reader); ok {
		return r
	}

	// idempotent(xio.CountedReader)
	xr := xio.WithCtxR(r)
	cr, ok := xr.(*xio.CountedReader)
	if !ok {
		cr = xio.CountReader(xr)
	}

	return &Reader{bufio.NewReader(xio.BindCtxR(cr, context.Background())), cr}
}

// InputOffset returns current logical position in input stream.
func (r *Reader) InputOffset() int64 {
	return r.cr.InputOffset() - int64(r.Reader.Buffered())
}
