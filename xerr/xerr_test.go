// Copyright (C) 2016-2017  Nexedi SA and Contributors.
//                          Kirill Smelkov <kirr@nexedi.com>
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

	check(nil, "")

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

func TestContext(t *testing.T) {
	test := func(e error) (err error) {
		defer Context(&err, "test ctx")
		return e
	}

	testf := func(e error) (err error) {
		defer Contextf(&err, "testf ctx %d %q", 123, "hello")
		return e
	}

	if test(nil) != nil {
		t.Error("Context(nil) -> !nil")
	}
	if testf(nil) != nil {
		t.Error("Contextf(nil) -> !nil")
	}

	err := errors.New("an error")

	want := "test ctx: an error"
	if e := test(err); !(e != nil && e.Error() == want) {
		t.Errorf("Context(%v) -> %v  ; want %v", err, e, want)
	}

	want = `testf ctx 123 "hello": an error`
	if e := testf(err); !(e != nil && e.Error() == want) {
		t.Errorf("Contextf(%v) -> %v  ; want %v", err, e, want)
	}
}
