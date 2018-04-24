// Copyright (C) 2015-2018  Nexedi SA and Contributors.
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

// Package exc provides exception-style error handling for Go.
//
// Raise and Catch allows to raise and catch exceptions.
//
// By default the error caught is the same error that was raised. However with
// Context functions can arrange for context related to what they are doing to
// be added to raised error as prefix, for example
//
//	func doSomething(path string) {
//		defer exc.Context(func() interface{} {
//			return fmt.Sprintf("doing something %s", path)
//		})()
//
//
// Lacking such Context annotations Addcallingcontext allows to add function
// names up to the exception point as the calling context. However this way
// only actions without corresponding arguments (path in the above example) can
// be shown, and there have to be direct 1-1 relation between program and
// operational structures.
//
// Runx allows to run a function which raises exception, and return exception
// as regular error, if any. Similarly XRun allows to run a function which
// returns regular error, and raise exception if error is not nil.
//
// Last but not least it has to be taken into account that exceptions
// complicate control flow and are directly applicable only to serial programs.
// Their use is thus justified in only limited number of cases and by default
// one should always first strongly consider using explicit error returns
// programming style which is canonical in Go.
package exc

import (
	"fmt"
	"runtime"
	"strings"

	"lab.nexedi.com/kirr/go123/my"
	"lab.nexedi.com/kirr/go123/xruntime"
)

// Error is the type which is raised by Raise(arg).
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

// Aserror turns any value into Error.
//
// if v is already Error - it stays the same,
// otherwise new Error is created.
func Aserror(v interface{}) *Error {
	if e, ok := v.(*Error); ok {
		return e
	}
	return &Error{v, nil}
}

// Raise raise error to upper level.
//
// See Catch which receives raised error.
func Raise(arg interface{}) {
	panic(Aserror(arg))
}

// Raisef raises formatted string.
func Raisef(format string, a ...interface{}) {
	panic(Aserror(fmt.Sprintf(format, a...)))
}

// Raiseif raises if err != nil.
//
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

// _errcatch checks recovered value to be of type *Error.
//
// if there is non-Error error - repanic it,
// otherwise return Error either nil (no panic), or actual value.
func _errcatch(r interface{}) *Error {
	e, _ := r.(*Error)
	if e == nil && r != nil {
		panic(r)
	}
	return e
}

// Catch catches error and calls f(e) if it was caught.
//
// Must be called under defer.
func Catch(f func(e *Error)) {
	e := _errcatch(recover())
	if e == nil {
		return
	}

	f(e)
}

// Onunwind installs error filter to be applied on error unwinding.
//
// It hooks into unwinding process with f() call. Returned error is reraised.
// see also: Context()
//
// Must be called under defer.
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

// Context provides error context to be added on unwinding.
//
// f is called if error unwinding is happening and its
// result is added to raised error as "prefix" context.
//
// Must be called under defer.
func Context(f func() interface{}) {
	e := _errcatch(recover())
	if e == nil {
		return
	}

	arg := f()
	panic(Addcontext(e, arg))
}

// Addcontext adds "prefix" context to error.
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

// Addcallingcontext adds calling context to error.
//
// Add calling function frames as error context up-to topfunc not including.
//
// See also: Addcontext()
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

// Runx runs a function which raises exception, and return exception as regular error, if any.
//
// the error, if non-nil, will be returned with added calling context - see
// Addcallingcontext for details.
//
// See also: Funcx.
func Runx(xf func()) (err error) {
	return Funcx(xf)()
}

// Funcx converts a function raising exception, to function returning regular error.
//
// Returned function calls xf and converts exception, if any, to error.
//
// See also: Runx.
func Funcx(xf func()) func() error {
	return func() (err error) {
		here := my.FuncName()
		defer Catch(func(e *Error) {
			err = Addcallingcontext(here, e)
		})

		xf()
		return
	}
}

// XRun runs a function which returns regular error, and raise exception if error is not nil.
//
// See also: XFunc.
func XRun(f func() error) {
	XFunc(f)()
}

// XFunc converts a function returning regular error, to function raising exception.
//
// Returned function calls f and raises appropriate exception if error is not nil.
//
// See also: XRun.
func XFunc(f func() error) func() {
	return func() {
		err := f()
		Raiseif(err)
	}
}
