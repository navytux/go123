// Copyright (C) 2017-2019  Nexedi SA and Contributors.
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

// Package xio provides addons to standard package io.
//
//   - Reader, Writer, ReadWriter, etc are io analogs that add support for contexts.
//   - BindCtx*(X, ctx) converts xio.X into io.X that implicitly passes ctx
//     to xio.X and can be used in legacy code.
//   - WithCtx*(X) converts io.X back into xio.X that accepts context.
//     It is the opposite operation for BindCtx, but for arbitrary io.X
//     returned xio.X handles context only on best-effort basis. In
//     particular IO cancellation is not reliably handled for os.File .
//   - Pipe amends io.Pipe and creates synchronous in-memory pipe that
//     supports IO cancellation.
//
// Miscellaneous utilities:
//
//   - CountReader provides InputOffset for a Reader.
package xio

import (
	"context"
	"io"
)

// Reader is like io.Reader but additionally takes context for Read.
type Reader interface {
	Read(ctx context.Context, dst []byte) (n int, err error)
}

// Writer is like io.Writer but additionally takes context for Write.
type Writer interface {
	Write(ctx context.Context, src []byte) (n int, err error)
}

// ReadWriter combines Reader and Writer.
type ReadWriter interface {
	Reader
	Writer
}

// ReadCloser combines Reader and io.Closer.
type ReadCloser interface {
	Reader
	io.Closer
}

// WriteCloser combines Writer and io.Closer.
type WriteCloser interface {
	Writer
	io.Closer
}

// ReadWriteCloser combines Reader, Writer and io.Closer.
type ReadWriteCloser interface {
	Reader
	Writer
	io.Closer
}


// BindCtx*(xio.X, ctx) -> io.X
//
// XXX better just BindCtx(x T, ctx) -> T with all x IO methods without ctx,
// but that needs either generics, or support from reflect to preserve optional
// methods: https://github.com/golang/go/issues/16522.


// BindCtxR binds Reader r and ctx into io.Reader which passes ctx to r on every Read.
func BindCtxR(r Reader, ctx context.Context) io.Reader {
	// BindCtx(WithCtx(X), BG) = X
	if ctx.Done() == nil {
		switch s := r.(type) {
		case *stubCtxR:   return s.r
		case *stubCtxRW:  return s.rw
		case *stubCtxRC:  return s.r
		case *stubCtxRWC: return s.rw
		}
	}

	return &bindCtxR{r, ctx}
}
type bindCtxR struct {r Reader; ctx context.Context}
func (b *bindCtxR) Read(dst []byte) (int, error)	{ return b.r.Read(b.ctx, dst) }

// BindCtxW binds Writer w and ctx into io.Writer which passes ctx to w on every Write.
func BindCtxW(w Writer, ctx context.Context) io.Writer {
	if ctx.Done() == nil {
		switch s := w.(type) {
		case *stubCtxW:   return s.w
		case *stubCtxRW:  return s.rw
		case *stubCtxWC:  return s.w
		case *stubCtxRWC: return s.rw
		}
	}
	return &bindCtxW{w, ctx}
}
type bindCtxW struct {w Writer; ctx context.Context}
func (b *bindCtxW) Write(src []byte) (int, error)	{ return b.w.Write(b.ctx, src)	}

// BindCtxRW binds ReadWriter rw and ctx into io.ReadWriter which passes ctx to
// rw on every Read and Write.
func BindCtxRW(rw ReadWriter, ctx context.Context) io.ReadWriter {
	if ctx.Done() == nil {
		switch s := rw.(type) {
		case *stubCtxRW:  return s.rw
		case *stubCtxRWC: return s.rw
		}
	}
	return &bindCtxRW{rw, ctx}
}
type bindCtxRW struct {rw ReadWriter; ctx context.Context}
func (b *bindCtxRW) Read (dst []byte) (int, error)	{ return b.rw.Read (b.ctx, dst) }
func (b *bindCtxRW) Write(src []byte) (int, error)	{ return b.rw.Write(b.ctx, src) }

// BindCtxRC binds ReadCloser r and ctx into io.ReadCloser which passes ctx to r on every Read.
func BindCtxRC(r ReadCloser, ctx context.Context) io.ReadCloser {
	if ctx.Done() == nil {
		switch s := r.(type) {
		case *stubCtxRC:  return s.r
		case *stubCtxRWC: return s.rw
		}
	}
	return &bindCtxRC{r, ctx}
}
type bindCtxRC struct {r ReadCloser; ctx context.Context}
func (b *bindCtxRC) Read(dst []byte) (int, error)	{ return b.r.Read(b.ctx, dst)	}
func (b *bindCtxRC) Close() error			{ return b.r.Close()		}

// BindCtxWC binds WriteCloser w and ctx into io.WriteCloser which passes ctx to w on every Write.
func BindCtxWC(w WriteCloser, ctx context.Context) io.WriteCloser {
	if ctx.Done() == nil {
		switch s := w.(type) {
		case *stubCtxWC:  return s.w
		case *stubCtxRWC: return s.rw
		}
	}
	return &bindCtxWC{w, ctx}
}
type bindCtxWC struct {w WriteCloser; ctx context.Context}
func (b *bindCtxWC) Write(src []byte) (int, error)	{ return b.w.Write(b.ctx, src)	}
func (b *bindCtxWC) Close() error			{ return b.w.Close()		}

// BindCtxRWC binds ReadWriteCloser rw and ctx into io.ReadWriteCloser
// which passes ctx to rw on every Read and Write.
func BindCtxRWC(rw ReadWriteCloser, ctx context.Context) io.ReadWriteCloser {
	if ctx.Done() == nil {
		switch s := rw.(type) {
		case *stubCtxRWC: return s.rw
		}
	}
	return &bindCtxRWC{rw, ctx}
}
type bindCtxRWC struct {rw ReadWriteCloser; ctx context.Context}
func (b *bindCtxRWC) Read(dst []byte) (int, error)	{ return b.rw.Read(b.ctx, dst)	}
func (b *bindCtxRWC) Write(src []byte) (int, error)	{ return b.rw.Write(b.ctx, src)	}
func (b *bindCtxRWC) Close() error			{ return b.rw.Close()		}


// WithCtx*(io.X) -> xio.X that handles ctx on best-effort basis.
//
// FIXME for arbitrary io.X for now ctx is completely ignored.
// TODO add support for cancellation if io.X provides working .Set{Read/Write}Deadline:
// https://medium.com/@zombiezen/canceling-i-o-in-go-capn-proto-5ae8c09c5b29
// https://github.com/golang/go/issues/20280

// WithCtxR converts io.Reader r into Reader that accepts ctx.
//
// It returns original IO object if r was created via BindCtx*, but in general
// returned Reader will handle context only on best-effort basis.
func WithCtxR(r io.Reader) Reader {
	// WithCtx(BindCtx(X)) = X
	switch b := r.(type) {
	case *bindCtxR:   return b.r
	case *bindCtxRW:  return b.rw
	case *bindCtxRC:  return b.r
	case *bindCtxRWC: return b.rw
	}

	return &stubCtxR{r}
}
type stubCtxR struct {r io.Reader}
func (s *stubCtxR) Read(ctx context.Context, dst []byte) (int, error)	{ return s.r.Read(dst) }

// WithCtxW converts io.Writer w into Writer that accepts ctx.
//
// It returns original IO object if w was created via BindCtx*, but in general
// returned Writer will handle context only on best-effort basis.
func WithCtxW(w io.Writer) Writer {
	switch b := w.(type) {
	case *bindCtxW:   return b.w
	case *bindCtxRW:  return b.rw
	case *bindCtxWC:  return b.w
	case *bindCtxRWC: return b.rw
	}
	return &stubCtxW{w}
}
type stubCtxW struct {w io.Writer}
func (s *stubCtxW) Write(ctx context.Context, src []byte) (int, error)	{ return s.w.Write(src) }

// WithCtxRW converts io.ReadWriter rw into ReadWriter that accepts ctx.
//
// It returns original IO object if rw was created via BindCtx*, but in general
// returned ReadWriter will handle context only on best-effort basis.
func WithCtxRW(rw io.ReadWriter) ReadWriter {
	switch b := rw.(type) {
	case *bindCtxRW:  return b.rw
	case *bindCtxRWC: return b.rw
	}
	return &stubCtxRW{rw}
}
type stubCtxRW struct {rw io.ReadWriter}
func (s *stubCtxRW) Read (ctx context.Context, dst []byte) (int, error)	{ return s.rw.Read (dst) }
func (s *stubCtxRW) Write(ctx context.Context, src []byte) (int, error)	{ return s.rw.Write(src) }

// WithCtxRC converts io.ReadCloser r into ReadCloser that accepts ctx.
//
// It returns original IO object if r was created via BindCtx*, but in general
// returned ReadCloser will handle context only on best-effort basis.
func WithCtxRC(r io.ReadCloser) ReadCloser {
	switch b := r.(type) {
	case *bindCtxRC:  return b.r
	case *bindCtxRWC: return b.rw
	}
	return &stubCtxRC{r}
}
type stubCtxRC struct {r io.ReadCloser}
func (s *stubCtxRC) Read (ctx context.Context, dst []byte) (int, error)	{ return s.r.Read(dst) }
func (s *stubCtxRC) Close() error					{ return s.r.Close() }

// WithCtxWC converts io.WriteCloser w into WriteCloser that accepts ctx.
//
// It returns original IO object if w was created via BindCtx*, but in general
// returned WriteCloser will handle context only on best-effort basis.
func WithCtxWC(w io.WriteCloser) WriteCloser {
	switch b := w.(type) {
	case *bindCtxWC:  return b.w
	case *bindCtxRWC: return b.rw
	}
	return &stubCtxWC{w}
}
type stubCtxWC struct {w io.WriteCloser}
func (s *stubCtxWC) Write(ctx context.Context, src []byte) (int, error)	{ return s.w.Write(src) }
func (s *stubCtxWC) Close() error					{ return s.w.Close() }

// WithCtxRWC converts io.ReadWriteCloser rw into ReadWriteCloser that accepts ctx.
//
// It returns original IO object if rw was created via BindCtx*, but in general
// returned ReadWriteCloser will handle context only on best-effort basis.
func WithCtxRWC(rw io.ReadWriteCloser) ReadWriteCloser {
	switch b := rw.(type) {
	case *bindCtxRWC: return b.rw
	}
	return &stubCtxRWC{rw}
}
type stubCtxRWC struct {rw io.ReadWriteCloser}
func (s *stubCtxRWC) Read (ctx context.Context, dst []byte) (int, error){ return s.rw.Read (dst) }
func (s *stubCtxRWC) Write(ctx context.Context, src []byte) (int, error){ return s.rw.Write(src) }
func (s *stubCtxRWC) Close() error					{ return s.rw.Close() }


// ----------------------------------------


// CountedReader is a Reader that count total bytes read.
type CountedReader struct {
	r     Reader
	nread int64
}

func (cr *CountedReader) Read(ctx context.Context, p []byte) (int, error) {
	n, err := cr.r.Read(ctx, p)
	cr.nread += int64(n)
	return n, err
}

// InputOffset returns the number of bytes read.
func (cr *CountedReader) InputOffset() int64 {
	return cr.nread
}

// CountReader wraps r with CountedReader.
func CountReader(r Reader) *CountedReader {
	return &CountedReader{r, 0}
}
