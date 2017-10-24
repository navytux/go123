// Copyright (C) 2015-2017  Nexedi SA and Contributors.
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

// Package xruntime provides addons to standard package runtime
package xruntime

import (
	"runtime"
)

// Traceback returns current calling traceback as []runtime.Frame
// nskip meaning: the same as in runtime.Callers()
func Traceback(nskip int) []runtime.Frame {
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
	framev := make([]runtime.Frame, 0, len(pcv))
	frames := runtime.CallersFrames(pcv)
	for more := true; more; {
		var frame runtime.Frame
		frame, more = frames.Next()
		framev = append(framev, frame)
	}

	return framev
}
