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

// Package mem allows to work with memory as either string or []byte without copying
package mem

import (
    "reflect"
    "unsafe"
)

// string -> []byte without copying
func Bytes(s string) []byte {
    var b []byte
    bp := (*reflect.SliceHeader)(unsafe.Pointer(&b))
    bp.Data = (*reflect.StringHeader)(unsafe.Pointer(&s)).Data
    bp.Cap = len(s)
    bp.Len = len(s)
    return b
}

// []byte -> string without copying
func String(b []byte) string {
    var s string
    sp := (*reflect.StringHeader)(unsafe.Pointer(&s))
    sp.Data = (*reflect.SliceHeader)(unsafe.Pointer(&b)).Data
    sp.Len = len(b)
    return s
}
