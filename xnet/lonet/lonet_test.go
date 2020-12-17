// Copyright (C) 2018-2020  Nexedi SA and Contributors.
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

package lonet

import (
	"context"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/sync/errgroup"

	"lab.nexedi.com/kirr/go123/exc"
	"lab.nexedi.com/kirr/go123/internal/xtesting"
	"lab.nexedi.com/kirr/go123/xerr"
	"lab.nexedi.com/kirr/go123/xnet/internal/virtnettest"
	"lab.nexedi.com/kirr/go123/xnet/virtnet"
)

func TestLonetGoGo(t *testing.T) {
	subnet, err := Join(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}

	// XXX TestBasic shutdows subnet, but /tmp/lonet/<name> is left alive.
	virtnettest.TestBasic(t, subnet)
}

func TestLonetPyPy(t *testing.T) {
	needPy(t)
	err := pytest("-k", "test_lonet_py_basic", "lonet_test.py")
	if err != nil {
		t.Fatal(err)
	}
}

func TestLonetGoPy(t *testing.T) {
	needPy(t)
	assert := xtesting.Assert(t)

	subnet, err := Join(bg, ""); X(err)
	defer func() {
		err := subnet.Close(); X(err)
	}()

	xaddr := func(addr string) *virtnet.Addr {
		a, err := virtnet.ParseAddr(subnet.Network(), addr); X(err)
		return a
	}

	hα, err := subnet.NewHost(bg, "α"); X(err)
	lα, err := hα.Listen(bg, ":1"); X(err)

	wg := &errgroup.Group{}
	wg.Go(exc.Funcx(func() {
		c1, err := lα.Accept(bg); X(err)
		assert.Eq(c1.LocalAddr(), xaddr("α:2"))
		assert.Eq(c1.RemoteAddr(), xaddr("β:2"))

		_, err = c1.Write([]byte("hello py")); X(err)
		buf := make([]byte, 1024)
		n, err := c1.Read(buf)
		buf = buf[:n]
		if want := "hello go"; string(buf) != want {
			exc.Raisef("go<-py: got %q; want %q", buf, want)
		}

		err = c1.Close(); X(err)

		c2, err := hα.Dial(bg, "β:1"); X(err)
		assert.Eq(c2.LocalAddr(), xaddr("α:2"))
		assert.Eq(c2.RemoteAddr(), xaddr("β:2"))

		buf = make([]byte, 1024)
		n, err = c2.Read(buf)
		buf = buf[:n]
		if want := "hello2 go"; string(buf) != want {
			exc.Raisef("go<-py 2: got %q; want %q", buf, want)
		}
		_, err = c2.Write([]byte("hello2 py")); X(err)

		err = c2.Close(); X(err)
	}))


	lonetwork := strings.TrimPrefix(subnet.Network(), "lonet")
	err = pytest("-k", "test_lonet_py_go", "--network", lonetwork, "lonet_test.py")
	X(err)

	err = wg.Wait(); X(err)
}



var havePy = false
var workRoot string

// needPy skips test if python is not available
func needPy(t *testing.T) {
	if havePy {
		return
	}
	t.Skipf("skipping: python/pygolang/pytest are not available")
}

func TestMain(m *testing.M) {
	// check whether we have python + infrastructure for tests
	cmd := exec.Command("python", "-c", "import golang, pytest")
	err := cmd.Run()
	if err == nil {
		havePy = true
	}

	// setup workroot for all tests
	workRoot, err = ioutil.TempDir("", "t-lonet")
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

// pytest runs py.test with argv arguments.
func pytest(argv ...string) (err error) {
	defer xerr.Contextf(&err, "pytest %s", argv)

	cmd := exec.Command("python", "-m", "pytest",
		// ex. with `--registry-dbpath /tmp/1.db` and existing /tmp/1.db,
		// pytest tries to set cachedir=/ , fails and prints warning.
		// Just disable the cache.
		"-p", "no:cacheprovider")
	if testing.Verbose() {
		cmd.Args = append(cmd.Args, "-v", "-s", "--log-file=/dev/stderr")
	} else {
		cmd.Args = append(cmd.Args, "-q", "-q")
	}
	cmd.Args = append(cmd.Args, argv...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
