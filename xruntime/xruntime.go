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

// Package xruntime provides addons to standard package runtime
package xruntime

import (
	"runtime"
)

// TODO(go1.7) goes away in favour of runtime.Frame
type Frame struct {
	*runtime.Func
	Pc uintptr
}

// get current calling traceback as []Frame
// nskip meaning: the same as in runtime.Callers()
// TODO(go1.7) []Frame -> []runtime.Frame
func Traceback(nskip int) []Frame {
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
