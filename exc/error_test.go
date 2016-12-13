// Copyright (C) 2015-2016  Nexedi SA and Contributors.
//                          Kirill Smelkov <kirr@nexedi.com>
//
// This program is free software: you can Use, Study, Modify and Redistribute
// it under the terms of the GNU General Public License version 3, or (at your
// option) any later version, as published by the Free Software Foundation.
//
// This program is distributed WITHOUT ANY WARRANTY; without even the implied
// warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
//
// See COPYING file for full licensing terms.

package main

import (
    "errors"
    "strings"
    "testing"
)

func do_raise1() {
    raise(1)
}

func TestErrRaiseCatch(t *testing.T) {
    defer errcatch(func(e *Error) {
        if !(e.arg == 1 && e.link == nil) {
            t.Fatalf("error caught but unexpected: %#v  ; want {1, nil}", e)
        }
    })
    do_raise1()
    t.Fatal("error not caught")
}

// verify err chain has .arg(s) as expected
func verifyErrChain(t *testing.T, e *Error, argv ...interface{}) {
    i := 0
    for ; e != nil; i, e = i+1, e.link {
        if i >= len(argv) {
            t.Fatal("too long error chain")
        }
        if e.arg != argv[i] {
            t.Fatalf("error caught but unexpected %vth arg: %v  ; want %v", i, e.arg, argv[i])
        }
    }
    if i < len(argv) {
        t.Fatal("too small error chain")
    }
}

func do_onunwind1(t *testing.T) {
    defer erronunwind(func(e *Error) *Error {
        t.Fatal("on unwind called without raise")
        return nil
    })
}

func do_onunwind2() {
    defer erronunwind(func(e *Error) *Error {
        return &Error{2, e}
    })
    do_raise1()
}

func TestErrOnUnwind(t *testing.T) {
    defer errcatch(func(e *Error) {
        verifyErrChain(t, e, 2, 1)
    })
    do_onunwind1(t)
    do_onunwind2()
    t.Fatal("error not caught")
}

func do_context1(t *testing.T) {
    defer errcontext(func() interface{} {
        t.Fatal("on context called without raise")
        return nil
    })
}

func do_context2() {
    defer errcontext(func() interface{} {
        return 3
    })
    do_raise1()
}

func TestErrContext(t *testing.T) {
    defer errcatch(func(e *Error) {
        verifyErrChain(t, e, 3, 1)
    })
    do_context1(t)
    do_context2()
    t.Fatal("error not caught")
}

func TestMyFuncName(t *testing.T) {
    myfunc := myfuncname()
    // go test changes full package name (putting filesystem of the tree into ti)
    // thus we check only for suffix
    wantsuffix := ".TestMyFuncName"
    if !strings.HasSuffix(myfunc, wantsuffix) {
        t.Errorf("myfuncname() -> %v  ; want *%v", myfunc, wantsuffix)
    }
}

func do_raise11() {
    do_raise1()
}

func do_raise3if() {
    raiseif(errors.New("3"))
}

func do_raise3if1() {
    do_raise3if()
}

func do_raise4f() {
    raisef("%d", 4)
}

func do_raise4f1() {
    do_raise4f()
}


func TestErrAddCallingContext(t *testing.T) {
    var tests = []struct{ f func(); wanterrcontext string } {
        {do_raise11,    "do_raise11: do_raise1: 1"},
        {do_raise3if1,  "do_raise3if1: do_raise3if: 3"},
        {do_raise4f1,   "do_raise4f1: do_raise4f: 4"},
    }

    for _, tt := range tests {
        func() {
            myfunc := myfuncname()
            defer errcatch(func(e *Error) {
                e = erraddcallingcontext(myfunc, e)
                msg := e.Error()
                if msg != tt.wanterrcontext {
                    t.Fatalf("err + calling context: %q  ; want %q", msg, tt.wanterrcontext)
                }
            })
            tt.f()
            t.Fatal("error not caught")
        }()
    }

}
