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

package lonet

import (
	"context"
	"fmt"
	"testing"

	"lab.nexedi.com/kirr/go123/exc"
	"lab.nexedi.com/kirr/go123/xnet/virtnet"
)

// registryTester is handy utility to test sqliteRegistry
type registryTester struct {
	*testing.T
	r *sqliteRegistry
}

// Query checks that result of Query(hostname) is as expected.
//
// if expect is error - it checks that Query returns error with cause == expect.
// otherwise expect must be string and it will check that Query
// succeeds and returns osladdr == expect.
func (t *registryTester) Query(hostname string, expect interface{}) {
	t.Helper()
	r := t.r

	osladdr, err := r.Query(context.Background(), hostname)
	if ewant, iserr := expect.(error); iserr {
		// error expected
		// XXX construct full registry error around ewant + reflect.DeepCompare?
		e, ok := err.(*virtnet.RegistryError)
		if !(ok && e.Err == ewant && osladdr == "") {
			t.Fatalf("%s: query %q:\nwant: \"\", %v\nhave: %q, %v",
				r.uri, hostname, ewant, osladdr, err)
		}
	} else {
		// !error expected
		laddr := expect.(string)
		if !(osladdr == laddr && err == nil) {
			t.Fatalf("%s: query %q:\nwant: %q, nil\nhave: %q, %v",
				r.uri, hostname, laddr, osladdr, err)
		}
	}
}

// Announce checks that result of Announce(hostname, osladdr) is as expected.
//
// if len(errv) == 1 - it checks that Announce returns error with cause == errv[0].
// otherwise it will check that Announce succeeds and returns nil error.
func (t *registryTester) Announce(hostname, osladdr string, errv ...error) {
	t.Helper()
	r := t.r

	err := r.Announce(context.Background(), hostname, osladdr)
	var ewant error
	if len(errv) > 0 {
		ewant = errv[0]
		if len(errv) > 1 {
			panic("only 1 error allowed in announce check")
		}
	}
	if ewant != nil {
		// error expected
		// XXX construct full registry error around ewant + reflect.DeepCompare?
		e, ok := err.(*virtnet.RegistryError)
		if (!ok && e.Err == ewant) {
			t.Fatalf("%s: announce %q %q:\nwant %v\nhave: %v",
				r.uri, hostname, osladdr, ewant, err)
		}
	} else {
		// !error expected
		if err != nil {
			t.Fatalf("%s: announce %q %q: %s", r.uri, hostname, osladdr, err)
		}
	}
}

// handy shortcuts for registry errors, ...
var ø     = virtnet.ErrNoHost
var DUP   = virtnet.ErrHostDup
var RDOWN = virtnet.ErrRegistryDown

var X  = exc.Raiseif
var bg = context.Background()


func TestRegistrySQLite(t *testing.T) {
	work := xworkdir(t)
	dbpath := work + "/1.db"

	r1, err := openRegistrySQLite(bg, dbpath, "aaa")
	X(err)

	t1 := &registryTester{t, r1}
	t1.Query("α", ø)
	t1.Announce("α", "alpha:1234")
	t1.Announce("α", "alpha:1234", DUP)
	t1.Announce("α", "alpha:1235", DUP)
	t1.Query("α", "alpha:1234")
	t1.Query("β", ø)

	r2, err := openRegistrySQLite(bg, dbpath, "aaa")
	X(err)

	t2 := &registryTester{t, r2}
	t2.Query("α", "alpha:1234")
	t2.Query("β", ø)
	t2.Announce("β", "beta:zzz")
	t2.Query("β", "beta:zzz")

	t1.Query("β", "beta:zzz")

	X(r1.Close())

	t1.Query("α", RDOWN)
	t1.Query("β", RDOWN)
	t1.Announce("γ", "gamma:qqq", RDOWN)
	t1.Query("γ", RDOWN)

	t2.Query("α", "alpha:1234")

	X(r2.Close())

	t2.Query("α", RDOWN)


	// verify network mismatch detection works
	r3, err := openRegistrySQLite(bg, dbpath, "bbb")
	if !(r3 == nil && err != nil) {
		t.Fatalf("network mismatch: not detected")
	}
	errWant := fmt.Sprintf(`%s: open []: setup "bbb": network name mismatch: want "bbb"; have "aaa"`, dbpath)
	if err.Error() != errWant {
		t.Fatalf("network mismatch: error:\nhave: %q\nwant: %q", err.Error(), errWant)
	}
}


// verify that go and python implementations of sqlite registry understand each other.
func TestRegistrySQLitePyGo(t *testing.T) {
	needPy(t)

	work := xworkdir(t)
	dbpath := work + "/1.db"

	r1, err := openRegistrySQLite(bg, dbpath, "ccc")
	X(err)

	t1 := &registryTester{t, r1}
	t1.Query("α", ø)
	t1.Announce("α", "alpha:1234")
	t1.Announce("α", "alpha:1234", DUP)
	t1.Announce("α", "alpha:1235", DUP)
	t1.Query("α", "alpha:1234")
	t1.Query("β", ø)

	// in python: check/modify the registry
	err = pytest("-k", "test_registry_pygo", "--registry-dbpath", dbpath, "lonet_test.py")
	X(err)

	// back in go: python must have set β + α should stay the same
	t1.Query("β", "beta:py")
	t1.Query("α", "alpha:1234")
}
