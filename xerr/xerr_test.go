// Copyright (C) 2016  Nexedi SA and Contributors.
//                     Kirill Smelkov <kirr@nexedi.com>
//
// This program is free software: you can Use, Study, Modify and Redistribute
// it under the terms of the GNU General Public License version 3, or (at your
// option) any later version, as published by the Free Software Foundation.
//
// This program is distributed WITHOUT ANY WARRANTY; without even the implied
// warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
//
// See COPYING file for full licensing terms.

package xerr

import (
	"errors"
	"reflect"
	"testing"
)

func TestErrorv(t *testing.T) {
	var errv Errorv
	check := func(aserr error, errmsg string) {
		err := errv.Err()
		// cannot use err != aserr as Errorv is not comparable (it is a slice)
		if !reflect.DeepEqual(err, aserr) {
			t.Fatalf("%#v: Err() -> %#v  ; want %#v", errv, err, aserr)
		}
		msg := errv.Error()
		if msg != errmsg {
			t.Fatalf("%#v: Error() -> %q  ; want %q", errv, msg, errmsg)
		}
	}

	err1 := errors.New("err1")
	err2 := errors.New("err2")

	errv.Append(err1)
	check(err1, "err1")

	errv.Appendif(nil)
	check(err1, "err1")

	errv.Appendif(err2)
	check(errv,
`2 errors:
	- err1
	- err2
`)

	errv.Appendf("err3 %q", "hello world")
	check(errv,
`3 errors:
	- err1
	- err2
	- err3 "hello world"
`)
}
