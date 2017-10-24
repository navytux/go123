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

package my

import (
	"strings"
	"testing"
)

// goes first not to be broken by other edits
func TestMyLine(t *testing.T) {
	myline := Line()
	want := 27
	if myline != want {
		t.Errorf("my.Line() -> %v  ; want %v", myline, want)
	}
}


func TestMyFuncName(t *testing.T) {
	myfunc := FuncName()
	// go test changes full package name (putting filesystem of the tree into it)
	// thus we check only for suffix
	wantsuffix := ".TestMyFuncName"
	if !strings.HasSuffix(myfunc, wantsuffix) {
		t.Errorf("my.FuncName() -> %v  ; want *%v", myfunc, wantsuffix)
	}
}

// XXX how to test PkgName? (go test changes full package name - see ^^^)

func TestMyFile(t *testing.T) {
	myfile := File()
	wantsuffix := "my_test.go"
	if !strings.HasSuffix(myfile, wantsuffix) {
		t.Errorf("my.File() -> %v  ; want *%v", myfile, wantsuffix)
	}
}
