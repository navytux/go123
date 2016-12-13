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

// Git-backup | Exception-style errors
package main

import (
    "fmt"
    "runtime"
    "strings"
)

// error type which is raised by raise(arg)
type Error struct {
    arg  interface{}
    link *Error // chain of linked Error(s) - see e.g. errcontext()
}

func (e *Error) Error() string {
    msgv := []string{}
    msg := ""
    for e != nil {
        // TODO(go1.7) -> runtime.Frame  (see xtraceback())
        if f, ok := e.arg.(Frame); ok {
            //msg = f.Function
            //msg = fmt.Sprintf("%s (%s:%d)", f.Function, f.File, f.Line)
            msg = strings.TrimPrefix(f.Name(), _errorpkgdot) // XXX -> better prettyfunc
        } else {
            msg = fmt.Sprint(e.arg)
        }
        msgv = append(msgv, msg)
        e = e.link
    }

    return strings.Join(msgv, ": ")
}

// turn any value into Error
// if v is already Error - it stays the same
// otherwise new Error is created
func aserror(v interface{}) *Error {
    if e, ok := v.(*Error); ok {
        return e
    }
    return &Error{v, nil}
}

// raise error to upper level
func raise(arg interface{}) {
    panic(aserror(arg))
}

// raise formatted string
func raisef(format string, a ...interface{}) {
    panic(aserror(fmt.Sprintf(format, a...)))
}

// raise if err != nil
// NOTE err can be != nil even if typed obj = nil:
//   var obj *T;
//   err = obj
//   err != nil     is true
func raiseif(err error) {
    //if err != nil && !reflect.ValueOf(err).IsNil() {
    if err != nil {
        panic(aserror(err))
    }
}

// checks recovered value to be of *Error
// if there is non-Error error - repanic it
// otherwise return Error either nil (no panic), or actual value
func _errcatch(r interface{}) *Error {
    e, _ := r.(*Error)
    if e == nil && r != nil {
        panic(r)
    }
    return e
}

// catch error and call f(e) if it was caught.
// must be called under defer
func errcatch(f func(e *Error)) {
    e := _errcatch(recover())
    if e == nil {
        return
    }

    f(e)
}

// be notified when error unwinding is being happening.
// hook into unwinding process with f() call. Returned error is reraised.
// see also: errcontext()
// must be called under defer
func erronunwind(f func(e *Error) *Error) {
    // cannot do errcatch(...)
    // as recover() works only in first-level called functions
    e := _errcatch(recover())
    if e == nil {
        return
    }

    e = f(e)
    panic(e)
}

// provide error context to automatically add on unwinding.
// f is called if error unwinding is happening.
// call result is added to raised error as "prefix" context
// must be called under defer
func errcontext(f func() interface{}) {
    e := _errcatch(recover())
    if e == nil {
        return
    }

    arg := f()
    panic(erraddcontext(e, arg))
}

// add "prefix" context to error
func erraddcontext(e *Error, arg interface{}) *Error {
    return &Error{arg, e}
}

func _myfuncname(nskip int) string {
    pcv := [1]uintptr{}
    runtime.Callers(nskip, pcv[:])
    f := runtime.FuncForPC(pcv[0])
    if f == nil {
        return ""
    }
    return f.Name()
}

// get name of currently running function (caller of myfuncname())
// name is fully qualified package/name.function(.x)
func myfuncname() string {
    return _myfuncname(3)
}

// get name of currently running function's package
// package is fully qualified package/name
func mypkgname() string {
    myfunc := _myfuncname(3)
    if myfunc == "" {
        return ""
    }
    // NOTE dots in package name are after last slash are escaped by go as %2e
    // this way the first '.' after last '/' is delimiter between package and function
    //
    // lab.nexedi.com/kirr/git-backup/package%2ename.Function
    // lab.nexedi.com/kirr/git-backup/pkg2.qqq/name%2ezzz.Function
    islash := strings.LastIndexByte(myfunc, '/')
    iafterslash := islash + 1   // NOTE if '/' not found iafterslash = 0
    idot := strings.IndexByte(myfunc[iafterslash:], '.')
    if idot == -1 {
        panic(fmt.Errorf("funcname %q is not fully qualified", myfunc))
    }
    return myfunc[:iafterslash+idot]
}

// TODO(go1.7) goes away in favour of runtime.Frame
type Frame struct {
    *runtime.Func
    pc  uintptr
}

// get current calling traceback as []Frame
// nskip meaning: the same as in runtime.Callers()
// TODO(go1.7) []Frame -> []runtime.Frame
func xtraceback(nskip int) []Frame {
    // all callers
    var pcv = []uintptr{0}
    for {
        pcv = make([]uintptr, 2*len(pcv))
        n := runtime.Callers(nskip+1, pcv)
        if n < len(pcv) {
            pcv = pcv[:n]
            break
        }
    }

    // pcv -> frames
/*
    framev := make([]runtime.Frame, 0, len(pcv))
    frames := runtime.CallersFrames(pcv)
    for more := true; more; {
        var frame runtime.Frame
        frame, more = frames.Next()
        framev = append(framev, frame)
    }
*/
    framev := make([]Frame, 0, len(pcv))
    for _, pc := range pcv {
        framev = append(framev, Frame{runtime.FuncForPC(pc), pc})
    }

    return framev
}

var (
    _errorpkgname string // package name under which error.go lives
    _errorpkgdot  string // errorpkg.
    _errorraise   string // errorpkg.raise
)

func init() {
    _errorpkgname = mypkgname()
    _errorpkgdot  = _errorpkgname + "."
    _errorraise   = _errorpkgname + ".raise"
}

// add calling context to error.
// Add calling function names as error context up-to topfunc not including.
// see also: erraddcontext()
func erraddcallingcontext(topfunc string, e *Error) *Error {
    seenraise := false
    for _, f := range xtraceback(2) {
        // do not show anything after raise*()
        if !seenraise && strings.HasPrefix(f.Name(), _errorraise) {
            seenraise = true
            continue
        }
        if !seenraise {
            continue
        }

        // do not go beyond topfunc
        if topfunc != "" && f.Name() == topfunc {
            break
        }

        // skip intermediates
        if strings.HasSuffix(f.Name(), "_") { // XXX -> better skipfunc
            continue
        }

        e = &Error{f, e}
    }

    return e
}

// error merging multiple errors (e.g. after collecting them from several parallel workers)
type Errorv []error

func (ev Errorv) Error() string {
    if len(ev) == 1 {
        return ev[0].Error()
    }

    msg := fmt.Sprintf("%d errors:\n", len(ev))
    for _, e := range ev {
        msg += fmt.Sprintf("\t- %s\n", e)
    }
    return msg
}
