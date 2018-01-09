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

// Package mem provides reference-counted buffer and ways to work with memory
// as either string or []byte without copying.
package mem

import (
	"reflect"
	"unsafe"
)

// Bytes converts string -> []byte without copying
func Bytes(s string) []byte {
	var b []byte
	bp := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	bp.Data = (*reflect.StringHeader)(unsafe.Pointer(&s)).Data
	bp.Cap = len(s)
	bp.Len = len(s)
	return b
}

// String converts []byte -> string without copying
func String(b []byte) string {
	var s string
	sp := (*reflect.StringHeader)(unsafe.Pointer(&s))
	sp.Data = (*reflect.SliceHeader)(unsafe.Pointer(&b)).Data
	sp.Len = len(b)
	return s
}
