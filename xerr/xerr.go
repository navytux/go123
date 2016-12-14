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

// Package xerr provides addons for error-handling
package xerr

import (
	"fmt"
)

// error merging multiple errors (e.g. after collecting them from several parallel workers)
type Errorv []error

func (errv Errorv) Error() string {
	if len(errv) == 1 {
		return errv[0].Error()
	}

	msg := fmt.Sprintf("%d errors:\n", len(errv))
	for _, e := range errv {
		msg += fmt.Sprintf("\t- %s\n", e)
	}
	return msg
}
