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

// Package xerr provides addons for error-handling.
//
//
// Error context
//
// Context and Contextf are handy to concisely add context to returned error,
// for example:
//
//	func myfunc(arg1, arg2 string) (..., err error) {
//		defer xerr.Contextf(&error, "doing something (%s, %s)", arg1, arg2)
//		...
//
// which will, if returned error is !nil, wrap it with the following prefix:
//
//	"doing something (%s, %s):" % (arg1, arg2)
//
// The original unwrapped error will be still accessible as the cause of
// returned error.  Please see package github.com/pkg/errors for details on
// this topic.
//
//
// Error vector
//
// Sometimes there are several operations performed and we want to collect
// errors from them all. For this Errorv could be used which is vector of
// errors and at the same time an error itself. After collecting it is possible
// to extract resulting error from the vector in canonical form with
// Errorv.Err. Errorv also provides handy ways to append errors to the vector
// - see Errorv.Append* for details.
//
// For convenience Merge could be used to concisely construct error vector
// from !nil errors and extract its canonical form in one line, for example:
//
//	err1 := op1(...)
//	err2 := op2(...)
//	err3 := op3(...)
//
//	err := xerr.Merge(err1, err2, err3)
//	return err
//
// There is also First counterpart to Merge, which returns only first !nil
// error.
//
// Since Errorv is actually a slice it cannot be generally compared - for example
// comparing 2 error interfaces that both have dynamic type Errorv will panic
// at runtime. However it is possible to compare Errorv to other error types,
// because interfaces with different dynamic types are always not equal. For
// example the following works:
//
//	var err error = Errorv{...} // received as result from a function
//	if err == io.EOF {
//		...
package xerr

import (
	"fmt"

	"github.com/pkg/errors"
)

// Errorv is error vector merging multiple errors (e.g. after collecting them from several parallel workers).
type Errorv []error

// Error returns string representation of error vector.
//
//	- ""			if len(errv)==0
//	- errv[0].Error()	if len(errv)==1
//	- "<n> errors:\n" + string representation of every error on separate line, otherwise.
func (errv Errorv) Error() string {
	switch len(errv) {
	case 0:
		return ""
	case 1:
		return errv[0].Error()
	}

	msg := fmt.Sprintf("%d errors:\n", len(errv))
	for _, e := range errv {
		msg += fmt.Sprintf("\t- %s\n", e)
	}
	return msg
}

// Append appends err to error vector.
func (errv *Errorv) Append(err error) {
	*errv = append(*errv, err)
}

// Appendif appends err to error vector if err != nil.
func (errv *Errorv) Appendif(err error) {
	if err == nil {
		return
	}
	errv.Append(err)
}

// Appendf appends formatted error string.
func (errv *Errorv) Appendf(format string, a ...interface{}) {
	errv.Append(fmt.Errorf(format, a...))
}

// Err returns error in canonical form accumulated in error vector.
//
//	- nil      if len(errv)==0
//	- errv[0]  if len(errv)==1		// XXX is this good idea?
//	- errv     otherwise
func (errv Errorv) Err() error {
	switch len(errv) {
	case 0:
		return nil
	case 1:
		return errv[0]
	default:
		return errv
	}
}

// Merge merges non-nil errors into one error.
//
// it returns:
//
//	- nil                         if all errors are nil
//	- single error                if there is only one non-nil error
//	- Errorv with non-nil errors  if there is more than one non-nil error
func Merge(errv ...error) error {
	ev := Errorv{}
	for _, err := range errv {
		ev.Appendif(err)
	}
	return ev.Err()
}

// First returns first non-nil error, or nil if there is no errors.
func First(errv ...error) error {
	for _, err := range errv {
		if err != nil {
			return err
		}
	}
	return nil
}

// ----------------------------------------

// Context provides error context to be automatically added on error return.
//
// Intended to be used under defer like this:
//
//	func myfunc(...) (..., err error) {
//		defer xerr.Context(&err, "error context")
//		...
//
// It is also possible to use Context directly to add context to an error if it
// is non-nil:
//
//	..., myerr := f()
//	xerr.Context(&myerr, "while doing something")
//
// which is equivalent to
//
//	import "github.com/pkg/errors"
//
//	..., myerr := f()
//	if myerr != nil {
//		myerr = errors.WithMessage(myerr, "while doing something")
//	}
func Context(errp *error, context string) {
	if *errp == nil {
		return
	}
	*errp = errors.WithMessage(*errp, context)
}

// Contextf provides formatted error context to be automatically added on error return.
//
// Contextf is formatted analog of Context. Please see Context for details on how to use.
func Contextf(errp *error, format string, argv ...interface{}) {
	if *errp == nil {
	        return
	}

	*errp = errors.WithMessage(*errp, fmt.Sprintf(format, argv...))
}
