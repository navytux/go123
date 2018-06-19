// Copyright (C) 2018  Nexedi SA and Contributors.
//                     Kirill Smelkov <kirr@nexedi.com>
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

// Package xtesting provides addons to std package testing.
//
// The tools provided are mostly useful when doing tests with exceptions.
package xtesting

import (
	"fmt"
	"reflect"
	"testing"

	"lab.nexedi.com/kirr/go123/exc"
)

// Asserter is handy objects to make asserts in tests.
//
// For example:
//
//	assert := xtesting.Assert(t)
//	assert.Eq(a, b)
//	..
//
// Contrary to t.Fatal* and e.g. github.com/stretchr/testify/require.Assert it
// is safe to use Asserter from non-main goroutine.
type Asserter struct {
	t testing.TB
}

// Assert creates Asserter bound to t for reporting.
func Assert(t testing.TB) *Asserter {
	return &Asserter{t}
}

// Eq asserts that a == b and raises exception if not.
func (x *Asserter) Eq(a, b interface{}) {
	x.t.Helper()
	if !reflect.DeepEqual(a, b) {
		fmt.Printf("not equal:\nhave: %v\nwant: %v\n", a, b)
		x.t.Errorf("not equal:\nhave: %v\nwant: %v", a, b)
		exc.Raise(0)
	}
}
