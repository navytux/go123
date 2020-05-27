// Copyright (C) 2017-2020  Nexedi SA and Contributors.
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

// Package xcontext provides addons to std package context.
//
// Merging contexts
//
// Merge could be handy in situations where spawned job needs to be canceled
// whenever any of 2 contexts becomes done. This frequently arises with service
// methods that accept context as argument, and the service itself, on another
// control line, could be instructed to become non-operational. For example:
//
//	func (srv *Service) DoSomething(ctx context.Context) (err error) {
//		defer xerr.Contextf(&err, "%s: do something", srv)
//
//		// srv.serveCtx is context that becomes canceled when srv is
//		// instructed to stop providing service.
//		origCtx := ctx
//		ctx, cancel := xcontext.Merge(ctx, srv.serveCtx)
//		defer cancel()
//
//		err = srv.doJob(ctx)
//		if err != nil {
//			if ctx.Err() != nil && origCtx.Err() == nil {
//				// error due to service shutdown
//				err = ErrServiceDown
//			}
//			return err
//		}
//
//		...
//	}
package xcontext

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// XXX if we could change std context, then Merge could work by simply creating
// cancelCtx and registering it to parent1 and parent2.
//
//	https://github.com/golang/go/issues/36503
//	https://github.com/golang/go/issues/36448
//
// For the reference: here is how it is done in pygolang:
//
//	https://lab.nexedi.com/kirr/pygolang/blob/d3bfb1bf/golang/context.py#L115-130
//	https://lab.nexedi.com/kirr/pygolang/blob/d3bfb1bf/golang/context.py#L228-264


// mergeCtx represents 2 context merged into 1.
type mergeCtx struct {
	parent1, parent2 context.Context

	done     chan struct{}
	doneMark uint32
	doneOnce sync.Once
	doneErr  error

	cancelCh   chan struct{}
	cancelOnce sync.Once
}

// Merge merges 2 contexts into 1.
//
// The result context:
//
//	- is done when parent1 or parent2 is done, or cancel called, whichever happens first,
//	- has deadline = min(parent1.Deadline, parent2.Deadline),
//	- has associated values merged from parent1 and parent2, with parent1 taking precedence.
//
// Canceling this context releases resources associated with it, so code should
// call cancel as soon as the operations running in this Context complete.
func Merge(parent1, parent2 context.Context) (context.Context, context.CancelFunc) {
	mc := &mergeCtx{
		parent1:  parent1,
		parent2:  parent2,
		done:     make(chan struct{}),
		cancelCh: make(chan struct{}),
	}

	// if src ctx is already cancelled - make mc cancelled right after creation
	//
	// this saves goroutine spawn and makes
	//
	//	ctx = Merge(ctx1, ctx2); ctx.Err != nil
	//
	// check possible.
	select {
	case <-parent1.Done():
		mc.finish(parent1.Err())

	case <-parent2.Done():
		mc.finish(parent2.Err())

	default:
		// src ctx not canceled - spawn parent{1,2}.done merger.
		go mc.wait()
	}

	return mc, mc.cancel
}

// finish marks merge ctx as done with specified error.
//
// it is safe to call finish multiple times and from multiple goroutines
// simultaneously - only the first call has the effect.
//
// finish returns the first error - with which ctx was actually marked as done.
func (mc *mergeCtx) finish(err error) error {
	mc.doneOnce.Do(func() {
		mc.doneErr = err
		atomic.StoreUint32(&mc.doneMark, 1)
		close(mc.done)
	})
	return mc.doneErr
}

// wait waits for (.parent1 | .parent2 | .cancelCh) and then marks mergeCtx as done.
func (mc *mergeCtx) wait() {
	var err error
	select {
	case <-mc.parent1.Done():
		err = mc.parent1.Err()

	case <-mc.parent2.Done():
		err = mc.parent2.Err()

	case <-mc.cancelCh:
		err = context.Canceled
	}

	mc.finish(err)
}

// cancel sends signal to wait to shutdown.
//
// cancel is the context.CancelFunc returned for mergeCtx by Merge.
func (mc *mergeCtx) cancel() {
	mc.cancelOnce.Do(func() {
		close(mc.cancelCh)
	})
}

// Done implements context.Context .
func (mc *mergeCtx) Done() <-chan struct{} {
	return mc.done
}

// Err implements context.Context .
func (mc *mergeCtx) Err() error {
	// fast path: if already done
	if atomic.LoadUint32(&mc.doneMark) != 0 {
		return mc.doneErr
	}

	// slow path: poll all sources so that there is no delay for e.g.
	// parent1.Err -> mergeCtx.Err, if user checks mergeCtx.Err directly.
	var err error
	select {
	case <-mc.parent1.Done():
		err = mc.parent1.Err()

	case <-mc.parent2.Done():
		err = mc.parent2.Err()

	case <-mc.cancelCh:
		err = context.Canceled

	default:
		return nil
	}

	return mc.finish(err)
}

// Deadline implements context.Context .
func (mc *mergeCtx) Deadline() (time.Time, bool) {
	d1, ok1 := mc.parent1.Deadline()
	d2, ok2 := mc.parent2.Deadline()
	switch {
	case !ok1:
		return d2, ok2
	case !ok2:
		return d1, ok1
	case d1.Before(d2):
		return d1, true
	default:
		return d2, true
	}
}

// Value implements context.Context .
func (mc *mergeCtx) Value(key interface{}) interface{} {
	v := mc.parent1.Value(key)
	if v != nil {
		return v
	}
	return mc.parent2.Value(key)
}

// ----------------------------------------

// chanCtx wraps channel into context.Context interface.
type chanCtx struct {
	done <-chan struct{}
}

// MergeChan merges context and channel into 1 context.
//
// MergeChan, similarly to Merge, provides resulting context which:
//
//	- is done when parent1 is done or done2 is closed, or cancel called, whichever happens first,
//	- has the same deadline as parent1,
//	- has the same associated values as parent1.
//
// Canceling this context releases resources associated with it, so code should
// call cancel as soon as the operations running in this Context complete.
func MergeChan(parent1 context.Context, done2 <-chan struct{}) (context.Context, context.CancelFunc) {
	return Merge(parent1, chanCtx{done2})
}

// Done implements context.Context .
func (c chanCtx) Done() <-chan struct{} {
	return c.done
}

// Err implements context.Context .
func (c chanCtx) Err() error {
	select {
	case <-c.done:
		return context.Canceled
	default:
		return nil
	}
}

// Deadline implements context.Context .
func (c chanCtx) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

// Value implements context.Context .
func (c chanCtx) Value(key interface{}) interface{} {
	return nil
}
