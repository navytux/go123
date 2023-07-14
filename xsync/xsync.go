// Copyright (C) 2019-2021  Nexedi SA and Contributors.
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

// Package xsync complements standard package sync.
//
//   - `WorkGroup` allows to spawn group of goroutines working on a common task.
//
// Functionality provided by xsync package is also provided by Pygolang(*) in its
// standard package sync.
//
// (*) https://pypi.org/project/pygolang
package xsync

import (
	"context"
	"sync"
)

// WorkGroup represents group of goroutines working on a common task.
//
// Use .Go() to spawn goroutines, and .Wait() to wait for all of them to
// complete, for example:
//
//	wg := xsync.NewWorkGroup(ctx)
//	wg.Go(f1)
//	wg.Go(f2)
//	err := wg.Wait()
//
// Every spawned function accepts context related to the whole work and derived
// from ctx used to initialize WorkGroup, for example:
//
//	func f1(ctx context.Context) error {
//	    ...
//	}
//
// Whenever a function returns error, the work context is canceled indicating
// to other spawned goroutines that they have to cancel their work. .Wait()
// waits for all spawned goroutines to complete and returns error, if any, from
// the first failed subtask.
//
// NOTE if spawned function panics, the panic is currently _not_ propagated to .Wait().
//
// WorkGroup is modelled after https://godoc.org/golang.org/x/sync/errgroup but
// is not equal to it.
type WorkGroup struct {
	ctx    context.Context // workers are spawned under ctx
	cancel func()          // aborts ctx
	waitg  sync.WaitGroup  // wait group for workers
	mu     sync.Mutex
	err    error           // error of the first failed worker
}

// NewWorkGroup creates new WorkGroup working under ctx.
//
// See WorkGroup documentation for details.
func NewWorkGroup(ctx context.Context) *WorkGroup {
	g := &WorkGroup{}
	g.ctx, g.cancel = context.WithCancel(ctx)
	return g
}

// Go spawns new worker under workgroup.
//
// See WorkGroup documentation for details.
func (g *WorkGroup) Go(f func(context.Context) error) {
	g.waitg.Add(1)
	go func() {
		defer g.waitg.Done()

		err := f(g.ctx)
		if err == nil {
			return
		}

		g.mu.Lock()
		defer g.mu.Unlock()

		if g.err == nil {
			// this goroutine is the first failed task
			g.err = err
			g.cancel()
		}
	}()
}

// Wait waits for all spawned workers to complete.
//
// It returns the error, if any, from the first failed worker.
// See WorkGroup documentation for details.
func (g *WorkGroup) Wait() error {
	g.waitg.Wait()
	g.cancel()
	return g.err
}
