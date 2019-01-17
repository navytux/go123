// Copyright (C) 2015-2019  Nexedi SA and Contributors.
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

// Package my provides easy way to determine current function's name and context.
package my

import (
	"fmt"
	"runtime"
	"strings"
)

func _myframe(nskip int) runtime.Frame {
	pcv := [1]uintptr{}
	n := runtime.Callers(nskip, pcv[:])
	if n != 1 {
		panic("error determining caller")
	}
	frameit := runtime.CallersFrames(pcv[:])
	f, _ := frameit.Next()
	return f
}

// FuncName returns name of currently running function.
//
// i.e. the name of FuncName caller.
//
// name is fully qualified package/name.function(.x)
func FuncName() string {
	f := _myframe(3)
	return f.Function
}

// PkgName returns name of currently running function's package.
//
// package is fully qualified package/name.
func PkgName() string {
	f := _myframe(3)
	myfunc := f.Function
	// NOTE dots in package name are after last slash are escaped by go as %2e
	// this way the first '.' after last '/' is delimiter between package and function
	//
	// lab.nexedi.com/kirr/git-backup/package%2ename.Function
	// lab.nexedi.com/kirr/git-backup/pkg2.qqq/name%2ezzz.Function
	islash := strings.LastIndexByte(myfunc, '/')
	iafterslash := islash + 1 // NOTE if '/' not found iafterslash = 0
	idot := strings.IndexByte(myfunc[iafterslash:], '.')
	if idot == -1 {
		panic(fmt.Errorf("funcname %q is not fully qualified", myfunc))
	}
	return myfunc[:iafterslash+idot]
}

// File returns path of currently running function's file.
func File() string {
	f := _myframe(3)
	return f.File
}

// Line returns currently running function's line.
func Line() int {
	f := _myframe(3)
	return f.Line
}

// Frame returns currently running functions's frame.
func Frame() runtime.Frame {
	return _myframe(3)
}
