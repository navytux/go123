// Copyright (C) 2015-2017  Nexedi SA and Contributors.
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

// Package exc provides exception-style error handling for Go
package exc

import (
	"fmt"
	"runtime"
	"strings"

	"lab.nexedi.com/kirr/go123/my"
	"lab.nexedi.com/kirr/go123/xruntime"
)

// error type which is raised by Raise(arg)
type Error struct {
	arg  interface{}
	link *Error // chain of linked Error(s) - see e.g. Context()
}

func (e *Error) Error() string {
	msgv := []string{}
	msg := ""
	for e != nil {
		if f, ok := e.arg.(runtime.Frame); ok {
			//msg = f.Function
			//msg = fmt.Sprintf("%s (%s:%d)", f.Function, f.File, f.Line)
			msg = strings.TrimPrefix(f.Function, _errorpkgdot) // XXX -> better prettyfunc
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
func Aserror(v interface{}) *Error {
	if e, ok := v.(*Error); ok {
		return e
	}
	return &Error{v, nil}
}

// raise error to upper level
func Raise(arg interface{}) {
	panic(Aserror(arg))
}

// raise formatted string
func Raisef(format string, a ...interface{}) {
	panic(Aserror(fmt.Sprintf(format, a...)))
}

// raise if err != nil
// NOTE err can be != nil even if typed obj = nil:
//   var obj *T;
//   err = obj
//   err != nil     is true
func Raiseif(err error) {
	//if err != nil && !reflect.ValueOf(err).IsNil() {
	if err != nil {
		panic(Aserror(err))
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
func Catch(f func(e *Error)) {
	e := _errcatch(recover())
	if e == nil {
		return
	}

	f(e)
}

// be notified when error unwinding is being happening.
// hook into unwinding process with f() call. Returned error is reraised.
// see also: Context()
// must be called under defer
func Onunwind(f func(e *Error) *Error) {
	// cannot do Catch(...)
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
func Context(f func() interface{}) {
	e := _errcatch(recover())
	if e == nil {
		return
	}

	arg := f()
	panic(Addcontext(e, arg))
}

// add "prefix" context to error
func Addcontext(e *Error, arg interface{}) *Error {
	return &Error{arg, e}
}

var (
	_errorpkgname string // package name under which error.go lives
	_errorpkgdot  string // errorpkg.
	_errorraise   string // errorpkg.Raise
)

func init() {
	_errorpkgname	= my.PkgName()
	_errorpkgdot	= _errorpkgname + "."
	_errorraise	= _errorpkgname + ".Raise"
}

// add calling context to error.
// Add calling function frames as error context up-to topfunc not including.
// see also: Addcontext()
func Addcallingcontext(topfunc string, e *Error) *Error {
	seenraise := false
	for _, f := range xruntime.Traceback(2) {
		// do not show anything after raise*()
		if !seenraise && strings.HasPrefix(f.Function, _errorraise) {
			seenraise = true
			continue
		}
		if !seenraise {
			continue
		}

		// do not go beyond topfunc
		if topfunc != "" && f.Function == topfunc {
			break
		}

		// skip intermediates
		if strings.HasSuffix(f.Function, "_") { // XXX -> better skipfunc
			continue
		}

		e = &Error{f, e}
	}

	return e
}

// run a function which raises exception, and return exception as regular error, if any.
// the error, if non-nil, will be returned with added calling context - see
// Addcallingcontext for details.
func Runx(xf func()) (err error) {
	here := my.FuncName()
	defer Catch(func(e *Error) {
		err = Addcallingcontext(here, e)
	})

	xf()
	return
}

// run a function which returns regular error, and raise exception if error is not nil.
func XRun(f func() error) {
	err := f()
	Raiseif(err)
}
