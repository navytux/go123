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
