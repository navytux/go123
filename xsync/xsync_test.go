// Copyright (C) 2019-2020  Nexedi SA and Contributors.
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

package xsync

import (
	"context"
	"fmt"
	"reflect"
	"testing"
)

func TestWorkGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	_ = cancel

	var l  []int // l[i] is data state for i'th worker
	var wg *WorkGroup
	// xwait waits for wg to complete, and asserts on returned error and state of l
	xwait := func(eok string, lok ...int) {
		t.Helper()
		err := wg.Wait()
		if eok == "" {
			if err != nil {
				t.Fatalf("xwait: failed: %q", err)
			}
		} else {
			estr := ""
			if err != nil {
				estr = err.Error()
			}
			if estr != eok {
				t.Fatalf("xwait: unexpected errror:\nhave: %q\nwant: %q", estr, eok)
			}
		}
		if !reflect.DeepEqual(l, lok) {
			t.Fatalf("xwait: unexpected l:\nhave: %v\nwant: %v", l, lok)
		}
	}

	// t1=ok, t2=ok
	wg = NewWorkGroup(ctx)
	l  = []int{0, 0}
	for i := 0; i < 2; i++ {
		i := i
		wg.Go(func(ctx context.Context) error {
			l[i] = i+1
			return nil
		})
	}
	xwait("", 1, 2)

	// t1=fail, t2=ok, does not look at ctx
	wg = NewWorkGroup(ctx)
	l  = []int{0, 0}
	for i := 0; i < 2; i++ {
		i := i
		wg.Go(func(ctx context.Context) error {
			l[i] = i+1
			if i == 0 {
				return fmt.Errorf("aaa")
			}
			return nil
		})
	}
	xwait("aaa", 1, 2)

	// t1=fail, t2=wait cancel, fail
	wg = NewWorkGroup(ctx)
	l  = []int{0, 0}
	for i := 0; i < 2; i++ {
		i := i
		wg.Go(func(ctx context.Context) error {
			l[i] = i+1
			if i == 0 {
				return fmt.Errorf("bbb")
			}
			if i == 1 {
				<-ctx.Done()
				return fmt.Errorf("ccc")
			}
			panic("unreachable")
		})
	}
	xwait("bbb", 1, 2)

	// t1=ok,wait cancel  t2=ok,wait cancel
	// cancel parent
	wg = NewWorkGroup(ctx)
	l  = []int{0, 0}
	for i := 0; i < 2; i++ {
		i := i
		wg.Go(func(ctx context.Context) error {
			l[i] = i+1
			<-ctx.Done()
			return nil
		})
	}
	cancel() // parent cancel - must be propagated into workgroup
	xwait("", 1, 2)
}
