// Copyright (C) 2016-2018  Nexedi SA and Contributors.
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

package xerr

import (
	"errors"
	"io"
	"reflect"
	"testing"

	pkgerrors "github.com/pkg/errors"
)

func TestErrorv(t *testing.T) {
	var errv Errorv

	// check verifies that:
	// 1. errv.Err() == aserr
	// 2. errv.Error() == errmsg
	check := func(aserr error, errmsg string) {
		err := errv.Err()
		// cannot use err != aserr as Errorv is not generally comparable (it is a slice)
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

	// since Errorv is a slice it cannot be generally compared - for
	// example comparing 2 error interfaces that both have dynamic type
	// Errorv will panic. However it is possible to compare Errorv to other
	// types, because interfaces with different dynamic types are always
	// not equal.
	eqcheck := func(a, b error, expect bool) {
		t.Helper()
		eq := (a == b)
		ne := (a != b)
		if eq != expect {
			t.Fatalf("%#v == %#v -> %v; want %v", a, b, eq, expect)
		}
		if ne != !expect {
			t.Fatalf("%#v != %#v -> %v; want %v", a, b, ne, !expect)
		}
	}

	eqcheck(err1, nil, false)
	eqcheck(err1, err1, true)
	eqcheck(errv, nil, false)
	eqcheck(errv, err1, false)
	eqcheck(errv, io.EOF, false)

	// check Errorv == Errorv panics.
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("Errorv == Errorv -> not paniced")
			}
		}()

		eqcheck(errv, errv, true)
	}()
}

func TestMerge(t *testing.T) {
	e := errors.New("e")
	e2 := errors.New("e2")

	testv := []struct {
		in  []error
		out error
	}{
		{nil, nil},
		{[]error{}, nil},
		{[]error{nil}, nil},
		{[]error{nil, nil}, nil},
		{[]error{e}, e},
		{[]error{e, nil}, e},
		{[]error{nil, e}, e},
		{[]error{nil, e, nil}, e},
		{[]error{nil, e, e2}, Errorv{e, e2}},
		{[]error{nil, e2, e}, Errorv{e2, e}},
		{[]error{nil, e2, nil, e}, Errorv{e2, e}},
		{[]error{nil, e2, nil, e, nil}, Errorv{e2, e}},
	}

	for _, tt := range testv {
		err := Merge(tt.in...)
		//if err != tt.out {
		// XXX Errorv is uncomparable because it is []
		if !reflect.DeepEqual(err, tt.out) {
			t.Errorf("Merge(%v) -> %v  ; want %v", tt.in, err, tt.out)
		}
	}
}

func TestFirst(t *testing.T) {
	e := errors.New("e")
	e2 := errors.New("e2")

	testv := []struct {
		in  []error
		out error
	}{
		{nil, nil},
		{[]error{}, nil},
		{[]error{nil}, nil},
		{[]error{nil, nil}, nil},
		{[]error{e}, e},
		{[]error{e, nil}, e},
		{[]error{nil, e}, e},
		{[]error{nil, e, nil}, e},
		{[]error{nil, e, e2}, e},
		{[]error{nil, e2, e}, e2},
	}

	for _, tt := range testv {
		err := First(tt.in...)
		if err != tt.out {
			t.Errorf("First(%v) -> %v  ; want %v", tt.in, err, tt.out)
		}
	}
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

	e := test(err)
	want := "test ctx: an error"
	if !(e != nil && e.Error() == want) {
		t.Errorf("Context(%v) -> %v  ; want %v", err, e, want)
	}
	if ec := pkgerrors.Cause(e); ec != err {
		t.Errorf("Context(%v) -> %v -> cause %v  ; want %v", err, e, ec, err)
	}

	e = testf(err)
	want = `testf ctx 123 "hello": an error`
	if !(e != nil && e.Error() == want) {
		t.Errorf("Contextf(%v) -> %v  ; want %v", err, e, want)
	}
	if ec := pkgerrors.Cause(e); ec != err {
		t.Errorf("Contextf(%v) -> %v -> cause %v  ; want %v", err, e, ec, err)
	}
}
