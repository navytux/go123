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

package mem

import (
	"reflect"
	"testing"
)

// check that String() and Bytes() create correct objects which alias original object memory
func TestStringBytes(t *testing.T) {
	s := "Hello"
	b := []byte(s)

	s1 := String(b)
	b1 := Bytes(s1)
	if s1 != s			{ t.Error("string -> []byte -> String != Identity") }
	if !reflect.DeepEqual(b1, b)	{ t.Error("[]byte -> String -> Bytes != Identity") }
	b[0] = 'I'
	if s != "Hello"			{ t.Error("string -> []byte not copied") }
	if s1 != "Iello"		{ t.Error("[]byte -> String not aliased") }
	if !reflect.DeepEqual(b1, b)	{ t.Error("string -> Bytes  not aliased") }
}
