// Copyright (C) 2019  Nexedi SA and Contributors.
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

package xio

import (
	"context"
	"testing"
)

// xIO is test Reader/Writer/Closer/...
type xIO struct{}

func (_ *xIO) Read(ctx context.Context, dst []byte) (int, error) {
	for i := range dst {
		dst[i] = 0
	}
	return len(dst), nil
}

func (_ *xIO) Write(ctx context.Context, src []byte) (int, error) {
	return len(src), nil
}

func (_ *xIO) Close() error {
	return nil
}

// tIO is test io.Reader/io.Writer/...
type tIO struct{}

func (_ *tIO) Read(dst []byte) (int, error) {
	for i := range dst {
		dst[i] = 0
	}
	return len(dst), nil
}

func (_ *tIO) Write(src []byte) (int, error) {
	return len(src), nil
}

func (_ *tIO) Close() error {
	return nil
}


// ok1 asserts that v is true.
func ok1(v bool) {
	if !v {
		panic("not ok")
	}
}

// Verify xio.X <-> io.X conversion
func TestConvert(t *testing.T) {
	x := new(xIO)
	i := new(tIO)
	bg := context.Background()

	// WithCtx(BindCtx(X)) = X
	ok1( WithCtxR(BindCtxR(x, bg)) == x )

	ok1( WithCtxW(BindCtxW(x, bg)) == x )

	ok1( WithCtxR (BindCtxRW(x, bg)) == x )
	ok1( WithCtxW (BindCtxRW(x, bg)) == x )
	ok1( WithCtxRW(BindCtxRW(x, bg)) == x )

	ok1( WithCtxR (BindCtxRC(x, bg)) == x )
	ok1( WithCtxRC(BindCtxRC(x, bg)) == x )

	ok1( WithCtxW (BindCtxWC(x, bg)) == x )
	ok1( WithCtxWC(BindCtxWC(x, bg)) == x )

	ok1( WithCtxR  (BindCtxRWC(x, bg)) == x )
	ok1( WithCtxW  (BindCtxRWC(x, bg)) == x )
	ok1( WithCtxRW (BindCtxRWC(x, bg)) == x )
	ok1( WithCtxRC (BindCtxRWC(x, bg)) == x )
	ok1( WithCtxWC (BindCtxRWC(x, bg)) == x )
	ok1( WithCtxRWC(BindCtxRWC(x, bg)) == x )


	// BindCtx(WithCtx(X), bg) = X
	ok1( BindCtxR(WithCtxR(i), bg) == i )

	ok1( BindCtxW(WithCtxW(i), bg) == i )

	ok1( BindCtxR (WithCtxRW(i), bg) == i )
	ok1( BindCtxW (WithCtxRW(i), bg) == i )
	ok1( BindCtxRW(WithCtxRW(i), bg) == i )

	ok1( BindCtxR (WithCtxRC(i), bg) == i )
	ok1( BindCtxRC(WithCtxRC(i), bg) == i )

	ok1( BindCtxW (WithCtxWC(i), bg) == i )
	ok1( BindCtxWC(WithCtxWC(i), bg) == i )

	ok1( BindCtxR  (WithCtxRWC(i), bg) == i )
	ok1( BindCtxW  (WithCtxRWC(i), bg) == i )
	ok1( BindCtxRW (WithCtxRWC(i), bg) == i )
	ok1( BindCtxRC (WithCtxRWC(i), bg) == i )
	ok1( BindCtxWC (WithCtxRWC(i), bg) == i )
	ok1( BindCtxRWC(WithCtxRWC(i), bg) == i )
}
