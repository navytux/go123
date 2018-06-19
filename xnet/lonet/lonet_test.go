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
	"io/ioutil"
	"log"
	"os"
	"testing"

	"lab.nexedi.com/kirr/go123/xnet/internal/virtnettest"
)

func TestLonet(t *testing.T) {
	subnet, err := Join(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}

	// XXX TestBasic shutdows subnet, but /tmp/lonet/<name> is left alive.
	virtnettest.TestBasic(t, subnet)
}



var workRoot string

func TestMain(m *testing.M) {
	// setup workroot for all tests
	workRoot, err := ioutil.TempDir("", "t-lonet")
	if err != nil {
		log.Fatal(err)
	}

	exit := m.Run()
	os.RemoveAll(workRoot)
	os.Exit(exit)
}

// xworkdir creates temp directory inside workRoot.
func xworkdir(t testing.TB) string {
	work, err := ioutil.TempDir(workRoot, "")
	if err != nil {
		t.Fatal(err)
	}
	return work
}
