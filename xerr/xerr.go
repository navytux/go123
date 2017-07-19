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

// Package xerr provides addons for error-handling
package xerr

import (
	"fmt"
)

// error merging multiple errors (e.g. after collecting them from several parallel workers)
type Errorv []error

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

// append err to error vector
func (errv *Errorv) Append(err error) {
	*errv = append(*errv, err)
}

// append err to error vector, but only if err != nil
func (errv *Errorv) Appendif(err error) {
	if err == nil {
		return
	}
	errv.Append(err)
}

// append formatted error string
func (errv *Errorv) Appendf(format string, a ...interface{}) {
	errv.Append(fmt.Errorf(format, a...))
}

// Err returns error in canonical form accumulated in error vector
// - nil	if len(errv)==0
// - errv[0]	if len(errv)==1		// XXX is this good idea?
// - errv	otherwise
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

// Merge merges non-nil errors into one error
// it returns:
// - nil			if all errors are nil
// - single error		if there is only one non-nil error
// - Errorv with non-nil errors	if there is more than one non-nil error
func Merge(errv ...error) error {
	ev := Errorv{}
	for _, err := range errv {
		ev.Appendif(err)
	}
	return ev.Err()
}

// First returns first non-nil error, or nil if there is no errors
func First(errv ...error) error {
	for _, err := range errv {
		if err != nil {
			return err
		}
	}
	return nil
}

// ----------------------------------------

// Context provides error context to be automatically added on error return
// to work as intended it should be called under defer like this:
//
//	func myfunc(...) (..., err error) {
//		defer xerr.Context(&err, "error context")
//		...
//
// it is also possible to use Context directly to add context to an error if it
// is non-nil:
//
//	..., myerr := f()
//	xerr.Context(&myerr, "while doing something")
//
// which is equivalent to
//
//	..., myerr := f()
//	if myerr != nil {
//		myerr = fmt.Errorf("%s: %s", "while doing something", myerr)
//	}
func Context(errp *error, context string) {
	if *errp == nil {
		return
	}
	*errp = fmt.Errorf("%s: %s", context, *errp)
}

// Contextf provides formatted error context to be automatically added on error return
// Contextf is formatted analog of Context. Please see Context for details on how to use.
func Contextf(errp *error, format string, argv ...interface{}) {
	if *errp == nil {
	        return
	}

	format += ": %s"
	argv = append(argv, *errp)
	*errp = fmt.Errorf(format, argv...)
}
