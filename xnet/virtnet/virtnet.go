// Copyright (C) 2017-2018  Nexedi SA and Contributors.
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

// Package virtnet provides infrastructure for TCP-like virtual networks.
//
// For testing distributed systems it is sometimes handy to imitate network of
// several TCP hosts. It is also handy that ports allocated on Dial/Listen/Accept
// on that hosts be predictable - that would help tests to verify network
// events against expected sequence.
//
// Package virtnet provides infrastructure for using and implementing such
// TCP-like virtual networks.
//
//
// Using virtnet networks
//
// Addresses on a virtnet network are host:port pairs represented by Addr.
// A network conceptually consists of several SubNetworks each being home for
// multiple Hosts. Host is xnet.Networker and so can be worked with similarly
// to regular TCP network access-point with Dial/Listen/Accept. Host's ports
// allocation is predictable: ports of a host are contiguous integer sequence
// starting from 1 that are all initially free, and whenever autobind is
// requested the first free port of the host will be used.
// Virtnet ensures that host names are unique throughout whole network.
//
// To work with a virtnet network, one uses corresponding package for
// particular virtnet network implementation. Such packages provide a way to
// join particular network and after joining give back SubNetwork to user.
// Starting from SubNetwork one can create Hosts and from those exchange data
// throughout whole network.
//
// Please see package lab.nexedi.com/kirr/go123/xnet/pipenet for particular
// well-known virtnet-based network.
//
//
// Implementing virtnet networks
//
// To implement a virtnet-based network one need to implement Engine and Registry.
//
// A virtnet network implementation should provide Engine and Registry
// instances to SubNetwork when creating it. The subnetwork will use provided
// engine and registry for its operations. A virtnet network implementation
// receives instance of Notifier together with SubNetwork when creating it. The
// network implementation should use provided Notifier to notify the subnetwork
// to handle incoming events.
//
// Please see Engine, Registry and Notifier documentation for details.
package virtnet

// TODO Fix virtnet for TCP semantic: there port(accepted) = port(listen), i.e.
//	When we connect www.nexedi.com:80, remote addr of socket will have port 80.
//	Likewise on server side accepted socket will have local port 80.
//	The connection should be thus fully identified by src-dst address pair.

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"

	"lab.nexedi.com/kirr/go123/xcontext"
	"lab.nexedi.com/kirr/go123/xerr"
	"lab.nexedi.com/kirr/go123/xnet"
)

var (
	ErrNetDown         = errors.New("network is down")
	ErrHostDown        = errors.New("host is down")
	ErrSockDown        = errors.New("socket is down")
	ErrAddrAlreadyUsed = errors.New("address already in use")
	ErrAddrNoListen    = errors.New("cannot listen on requested address")
	ErrConnRefused     = errors.New("connection refused")
)

// Addr represents address of a virtnet endpoint.
type Addr struct {
	Net  string // full network name, e.g. "pipeα" or "lonetβ"
	Host string // name of host access point on the network
	Port int    // port on host
}

// SubNetwork represents one subnetwork of a virtnet network.
//
// Multiple Hosts could be created on one subnetwork.
// Multiple subnetworks could be part of a single virtnet network.
//
// Host names are unique through whole virtnet network.
//
// SubNetwork should be created by a particular virtnet network implementation
// via NewSubNetwork.
type SubNetwork struct {
	// full network name, e.g. "pipeα" or "lonetβ"
	network string

	// virtnet network implementation and registry given to us
	engine   Engine
	registry Registry

	// {} hostname -> Host
	hostMu  sync.Mutex
	hostMap map[string]*Host

	down     chan struct{} // closed when no longer operational
	downErr  error
	downOnce sync.Once
}

// Host represents named access point on a virtnet network.
//
// A Host belongs to a SubNetwork.
// It has name and array of sockets indexed by port.
// It implements xnet.Networker.
//
// It is safe to use Host from multiple goroutines simultaneously.
type Host struct {
	subnet *SubNetwork
	name   string

	// [] port -> listener | conn  ; [0] is always nil
	sockMu  sync.Mutex
	socketv []*socket

	down     chan struct{} // closed when no longer operational
	downOnce sync.Once
}

var _ xnet.Networker = (*Host)(nil)

// socket represents one endpoint entry on Host.
//
// it can be either already connected or listening.
type socket struct {
	host *Host // host/port this socket is bound to
	port int

	conn     *conn     // connection endpoint is here if != nil
	listener *listener // listener is waiting here if != nil
}

// conn represents one endpoint of a virtnet connection.
//
// conn is the net.Conn implementation that Host.Dial and listener.Accept return.
type conn struct {
	socket   *socket // local socket
	peerAddr *Addr   // address of the remote side of this connection

	net.Conn

	down      uint32    // 1 after shutdown
	downOnce  sync.Once
	errClose  error     // error we got from closing underlying net.Conn
	closeOnce sync.Once
}

// listener implements net.Listener for Host.Listen .
type listener struct {
	// subnetwork/host/port we are listening on
	socket *socket

	dialq chan dialReq // Dial requests to our port go here

	down      chan struct{} // closed when no longer operational
	downOnce  sync.Once
	closeOnce sync.Once
}

// dialReq represents one dial request to listener from acceptor.
type dialReq struct {
	from *Addr
	conn net.Conn
	resp chan *Accept
}

// notifier implements Notifier for SubNetwork.
//
// it is separate from SubNetwork not to generally expose Notifier as API
// virtnet users (contrary to virtnet network implementers) should use.
type notifier struct {
	subnet *SubNetwork
}


// ----------------------------------------

// NewSubNetwork creates new SubNetwork with given name.
//
// It should be used by virtnet network implementations who should provide it
// with Engine and Registry instances.
//
// Together with returned SubNetwork an instance of Notifier is provided that
// should be used by virtnet network implementation to notify created
// subnetwork to handle incoming events.
func NewSubNetwork(network string, engine Engine, registry Registry) (*SubNetwork, Notifier) {
	// XXX prefix network with "virtnet/" ?
	subnet := &SubNetwork{
		network:  network,
		engine:   engine,
		registry: registry,
		hostMap:  make(map[string]*Host),
		down:     make(chan struct{}),
	}

	return subnet, &notifier{subnet}
}


// shutdown shutdowns subnetwork.
//
// It is worker for Close and VNetDown.
//
// The error provided is the cause of shutdown - e.g. IO error from engine, or
// nil on plain close.
//
// It is safe to call shutdown multiple times and from multiple goroutines
// simultaneously - only the first call has the effect.
//
// The error returned is cumulative shutdown error - the cause + any error from
// closing engine and registry for the call when shutdown was actually performed.
func (n *SubNetwork) shutdown(err error) error {
	n.downOnce.Do(func() {
		close(n.down)

		// shutdown hosts
		n.hostMu.Lock()
		for _, host := range n.hostMap {
			host.shutdown()
		}
		n.hostMu.Unlock()

		var errv xerr.Errorv
		errv.Appendif( err )
		errv.Appendif( n.engine.Close() )
		errv.Appendif( n.registry.Close() )

		n.downErr = errv.Err()
	})

	return n.downErr
}

// Close shutdowns subnetwork.
//
// It recursively interrupts all blocking operations on the subnetwork and
// shutdowns all subnetwork's hosts and connections.
func (n *SubNetwork) Close() (err error) {
	defer xerr.Contextf(&err, "virtnet %q: close", n.network)
	return n.shutdown(nil)
}

// VNetDown implements Notifier by shutting subnetwork down upon engine error.
func (nn *notifier) VNetDown(err error) {
	nn.subnet.shutdown(err)
}


// NewHost creates new Host with given name.
//
// The host will be associated with SubNetwork via which it was created.
//
// Host names should be unique through whole virtnet network.
// If not - an error with ErrHostDup cause will be returned.
func (n *SubNetwork) NewHost(ctx context.Context, name string) (_ *Host, err error) {
	defer xerr.Contextf(&err, "virtnet %q: new host %q", n.network, name)

	// cancel on network shutdown
	origCtx := ctx
	ctx, cancel := xcontext.MergeChan(ctx, n.down); defer cancel()

	// announce new host
	err = n.engine.VNetNewHost(ctx, name, n.registry)
	if err != nil {
		if ctx.Err() != nil && origCtx.Err() == nil {
			// error due to subnetwork shutdown
			err = ErrNetDown
		}
		return nil, err
	}

	// announced ok -> host can be created
	n.hostMu.Lock()
	defer n.hostMu.Unlock()

	if n.hostMap[name] != nil {
		panic("announced ok but .hostMap already !empty")
	}

	host := &Host{subnet: n, name: name, down: make(chan struct{})}
	n.hostMap[name] = host

	return host, nil
}

// Host returns host on the subnetwork by name.
//
// If there is no such host - nil is returned.
func (n *SubNetwork) Host(name string) *Host {
	n.hostMu.Lock()
	defer n.hostMu.Unlock()

	return n.hostMap[name]
}

// shutdown is underlying worker for Close.
func (h *Host) shutdown() {
	h.downOnce.Do(func() {
		close(h.down)

		// shutdown all sockets
		h.sockMu.Lock()
		defer h.sockMu.Unlock()

		for _, sk := range h.socketv {
			if sk == nil {
				continue
			}
			if sk.conn != nil {
				sk.conn.shutdown()
			}
			if sk.listener != nil {
				sk.listener.shutdown()
			}
		}
	})
}

// Close shutdowns host.
//
// After host is shutdown connections to it cannot be established and all
// currently-established connections are shut down.
//
// Close interrupts all currently in-flight blocked I/O operations on Host or
// objects created from it: connections and listeners.
func (h *Host) Close() (err error) {
	defer xerr.Contextf(&err, "virtnet %q: host %q: close", h.subnet.network, h.name)
	h.shutdown()
	return nil
}

// Listen starts new listener on the host.
//
// It either allocates free port if laddr is "" or with 0 port, or binds to laddr.
// Once listener is started, Dials could connect to listening address.
// Connection requests created by Dials could be accepted via Accept.
func (h *Host) Listen(laddr string) (_ net.Listener, err error) {
	var netladdr net.Addr
	defer func() {
		if err != nil {
			err = &net.OpError{Op: "listen", Net: h.Network(), Addr: netladdr, Err: err}
		}
	}()

	if laddr == "" {
		laddr = ":0"
	}

	a, err := h.parseAddr(laddr)
	if err != nil {
		return nil, err
	}
	netladdr = a

	// cannot listen on other hosts
	if a.Host != h.name {
		return nil, ErrAddrNoListen
	}

	if ready(h.down) {
		return nil, h.errDown()
	}

	h.sockMu.Lock()
	defer h.sockMu.Unlock()

	var sk *socket

	// find first free port if autobind requested
	if a.Port == 0 {
		sk = h.allocFreeSocket()

	// else allocate socket in-place
	} else {
		// grow if needed
		for a.Port >= len(h.socketv) {
			h.socketv = append(h.socketv, nil)
		}

		if h.socketv[a.Port] != nil {
			return nil, ErrAddrAlreadyUsed
		}

		sk = &socket{host: h, port: a.Port}
		h.socketv[a.Port] = sk
	}

	// create listener under socket
	l := &listener{
		socket: sk,
		dialq:  make(chan dialReq),
		down:   make(chan struct{}),
	}
	sk.listener = l

	return l, nil
}

// shutdown shutdowns the listener.
//
// It interrupts all currently in-flight calls to Accept, but does not
// unregister listener from host's socket map.
func (l *listener) shutdown() {
	l.downOnce.Do(func() {
		close(l.down)
	})
}

// Close closes the listener.
//
// It interrupts all currently in-flight calls to Accept.
func (l *listener) Close() error {
	l.shutdown()
	l.closeOnce.Do(func() {
		sk := l.socket
		h := sk.host

		h.sockMu.Lock()
		defer h.sockMu.Unlock()

		sk.listener = nil
		if sk.empty() {
			h.socketv[sk.port] = nil
		}
	})
	return nil
}

// Accept tries to connect to Dial called with addr corresponding to our listener.
func (l *listener) Accept() (_ net.Conn, err error) {
	h := l.socket.host

	defer func() {
		if err != nil {
			err = &net.OpError{Op: "accept", Net: h.Network(), Addr: l.Addr(), Err: err}
		}
	}()

	for {
		var req dialReq

		select {
		case <-l.down:
			return nil, l.errDown()

		case req = <-l.dialq:
			// ok
		}

		// acceptor dials us - allocate empty socket so that we know accept address.
		h.sockMu.Lock()
		sk := h.allocFreeSocket()
		h.sockMu.Unlock()

		// give acceptor feedback that we are accepting the connection.
		ack := make(chan error)
		req.resp <- &Accept{sk.addr(), ack}

		// wait for ack from acceptor.
		select {
		case <-l.down:
			// acceptor was slow and we have to shutdown the listener.
			// we have to make sure we still receive on ack and
			// close req.conn / unallocate the socket appropriately.
			go func() {
				err := <-ack
				if err == nil {
					// acceptor conveyed us the connection - close it
					req.conn.Close()
				}
				h.sockMu.Lock()
				h.socketv[sk.port] = nil
				h.sockMu.Unlock()
			}()

			return nil, l.errDown()

		case err = <-ack:
			// ok
		}

		// we got feedback from acceptor
		// if there is an error - unallocate the socket and continue waiting.
		if err != nil {
			h.sockMu.Lock()
			h.socketv[sk.port] = nil
			h.sockMu.Unlock()
			continue
		}

		// all ok - allocate conn, bind it to socket and we are done.
		c := &conn{socket: sk, peerAddr: req.from, Conn: req.conn}
		h.sockMu.Lock()
		sk.conn = c
		h.sockMu.Unlock()

		return c, nil
	}
}

// VNetAccept implements Notifier by accepting or rejecting incoming connection.
func (nn *notifier) VNetAccept(ctx context.Context, src, dst *Addr, netconn net.Conn) (*Accept, error) {
	n := nn.subnet

	n.hostMu.Lock()
	host := n.hostMap[dst.Host]
	n.hostMu.Unlock()
	if host == nil {
		return nil, &net.AddrError{Err: "no such host", Addr: dst.String()}
	}

	host.sockMu.Lock()

	if dst.Port >= len(host.socketv) {
		host.sockMu.Unlock()
		return nil, ErrConnRefused
	}

	sk := host.socketv[dst.Port]
	if sk == nil || sk.listener == nil {
		host.sockMu.Unlock()
		return nil, ErrConnRefused
	}

	// there is listener corresponding to dst - let's connect it
	l := sk.listener
	host.sockMu.Unlock()

	resp := make(chan *Accept)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()

	case <-l.down:
		return nil, ErrConnRefused

	case l.dialq <- dialReq{from: src, conn: netconn, resp: resp}:
		return <-resp, nil
	}
}


// Dial dials address on the network.
//
// It tries to connect to Accept called on listener corresponding to addr.
func (h *Host) Dial(ctx context.Context, addr string) (_ net.Conn, err error) {
	// allocate socket in empty state early, so we can see in the error who
	// tries to dial.
	h.sockMu.Lock()
	sk := h.allocFreeSocket()
	h.sockMu.Unlock()
	defer func() {
		if err != nil {
			h.sockMu.Lock()
			h.socketv[sk.port] = nil
			h.sockMu.Unlock()
		}
	}()

	var netdst net.Addr
	defer func() {
		if err != nil {
			err = &net.OpError{Op: "dial", Net: h.Network(), Source: sk.addr(), Addr: netdst, Err: err}
		}

	}()


	dst, err := h.parseAddr(addr)
	if err != nil {
		return nil, err
	}
	netdst = dst

	n := h.subnet

	// cancel on host shutdown
	origCtx := ctx
	ctx, cancel := xcontext.MergeChan(ctx, h.down); defer cancel()
	errOrDown := func(err error) error {
		if ctx.Err() != nil && origCtx.Err() == nil {
			// error due to shutdown
			return h.errDown()
		}
		return err
	}

	// query registry
	dstdata, err := n.registry.Query(ctx, dst.Host)
	if err != nil {
		return nil, errOrDown(err)
	}

	// dial engine
	netconn, acceptAddr, err := n.engine.VNetDial(ctx, sk.addr(), dst, dstdata)
	if err != nil {
		return nil, errOrDown(err)
	}

	// handshake performed ok - we are done.
	c := &conn{socket: sk, peerAddr: acceptAddr, Conn: netconn}
	h.sockMu.Lock()
	sk.conn = c
	h.sockMu.Unlock()

	return c, nil
}

// ---- conn ----

// shutdown closes underlying network connection.
func (c *conn) shutdown() {
	c.downOnce.Do(func() {
		atomic.StoreUint32(&c.down, 1)
		c.errClose = c.Conn.Close()
	})
}

// Close closes network endpoint and unregisters conn from Host.
//
// All currently in-flight blocked IO is interrupted with an error.
func (c *conn) Close() error {
	c.shutdown()
	c.closeOnce.Do(func() {
		sk := c.socket
		h := sk.host

		h.sockMu.Lock()
		defer h.sockMu.Unlock()

		sk.conn = nil
		if sk.empty() {
			h.socketv[sk.port] = nil
		}
	})

	return c.errClose
}


// Read implements net.Conn .
//
// it delegates the read to underlying net.Conn but amends error if it was due
// to conn shutdown.
func (c *conn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if err != nil && err != io.EOF {
		if !errIsTimeout(err) {
			// an error that might be due to shutdown
			err = c.errOrDown(err)
		}

		// wrap error to be at virtnet level.
		// net.OpError preserve .Timeout() value if .Err has it.
		err = &net.OpError{
			Op:     "read",
			Net:    c.socket.host.Network(),
			Addr:   c.LocalAddr(),
			Source: c.RemoteAddr(),
			Err:    err,
		}
	}
	return n, err
}

// Write implements net.Conn .
//
// it delegates the write to underlying net.Conn but amends error if it was due
// to conn shutdown.
func (c *conn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if err != nil {
		if !errIsTimeout(err) {
			err = c.errOrDown(err)
		}

		err = &net.OpError{
			Op:     "write",
			Net:    c.socket.host.Network(),
			Addr:   c.RemoteAddr(),
			Source: c.LocalAddr(),
			Err:    err,
		}
	}
	return n, err
}


// LocalAddr implements net.Conn.
//
// it returns virtnet address of local end of connection.
func (c *conn) LocalAddr() net.Addr {
	return c.socket.addr()
}

// RemoteAddr implements net.Conn .
//
// it returns virtnet address of remote end of connection.
func (c *conn) RemoteAddr() net.Addr {
	return c.peerAddr
}

// ----------------------------------------

// allocFreeSocket finds first free port and allocates socket entry for it.
//
// must be called with SubNetwork.mu held.
func (h *Host) allocFreeSocket() *socket {
	// find first free port
	port := 1 // never allocate port 0 - it is used for autobind on listen only
	for ; port < len(h.socketv); port++ {
		if h.socketv[port] == nil {
			break
		}
	}
	// if all busy it exits with port >= len(h.socketv)

	// grow if needed
	for port >= len(h.socketv) {
		h.socketv = append(h.socketv, nil)
	}

	sk := &socket{host: h, port: port}
	h.socketv[port] = sk
	return sk
}

// empty checks whether socket's both conn and listener are all nil.
func (sk *socket) empty() bool {
	return sk.conn == nil && sk.listener == nil
}

// addr returns address corresponding to socket.
func (sk *socket) addr() *Addr {
	h := sk.host
	return &Addr{Net: h.Network(), Host: h.name, Port: sk.port}
}

// Network implements net.Addr .
func (a *Addr) Network() string { return a.Net }

// String implements net.Addr .
func (a *Addr) String() string { return net.JoinHostPort(a.Host, strconv.Itoa(a.Port)) }

// ParseAddr parses addr into virtnet address for named network.
func ParseAddr(network, addr string) (*Addr, error) {
	host, portstr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portstr)
	if err != nil || port < 0 {
		return nil, &net.AddrError{Err: "invalid port", Addr: addr}
	}
	return &Addr{Net: network, Host: host, Port: port}, nil
}

// parseAddr parses addr into virtnet address from host point of view.
//
// It is the same as ParseAddr except empty host string - e.g. as in ":0" -
// is resolved to the host itself.
func (h *Host) parseAddr(addr string) (*Addr, error) {
	a, err := ParseAddr(h.Network(), addr)
	if err != nil {
		return nil, err
	}

	// local host if host name omitted
	if a.Host == "" {
		a.Host = h.name
	}

	return a, nil
}

// Addr returns address where listener is accepting incoming connections.
func (l *listener) Addr() net.Addr {
	return l.socket.addr()
}

// Network returns full network name this subnetwork is part of.
func (n *SubNetwork) Network() string { return n.network }

// Network returns full network name of underlying network.
func (h *Host) Network() string { return h.subnet.Network() }

// Name returns host name.
func (h *Host) Name() string { return h.name }

// ----------------------------------------

// errDown returns appropriate error cause when h.down is found ready.
func (h *Host) errDown() error {
	switch {
	case ready(h.subnet.down):
		return ErrNetDown
	default:
		return ErrHostDown
	}
}

// errDown returns appropriate error cause when l.down is found ready.
func (l *listener) errDown() error {
	h := l.socket.host
	n := h.subnet

	switch {
	case ready(n.down):
		return ErrNetDown
	case ready(h.down):
		return ErrHostDown
	default:
		return ErrSockDown
	}
}

// errOrDown returns err or shutdown cause if c.shutdown was called.
func (c *conn) errOrDown(err error) error {
	// shutdown was not yet called - leave it as is.
	if atomic.LoadUint32(&c.down) == 0 {
		return err
	}

	// shutdown was called - find out the reason.
	h := c.socket.host
	n := h.subnet
	switch {
	case ready(n.down):
		return ErrNetDown
	case ready(h.down):
		return ErrHostDown
	default:
		return ErrSockDown
	}
}


// ready returns whether chan struct{} is ready.
func ready(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// errIsTimeout checks whether error is due to timeout.
//
// useful to check because net.Conn says:
//
//	"Read/Write can be made to time out and return an Error with Timeout() == true"
func errIsTimeout(err error) bool {
	e, ok := err.(interface{ Timeout() bool })
	if ok {
		return e.Timeout()
	}
	return false
}
