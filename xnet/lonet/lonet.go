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

// Package lonet provides TCP network simulated on top of localhost TCP loopback.
//
// For testing distributed systems it is sometimes handy to imitate network of
// several TCP hosts. It is also handy that ports allocated on Dial/Listen/Accept on
// that hosts be predictable - that would help tests to verify network events
// against expected sequence. When whole system could be imitated in 1 OS-level
// process, package lab.nexedi.com/kirr/go123/xnet/pipenet serves the task via
// providing TCP-like synchronous in-memory network of net.Pipes. However
// pipenet cannot be used for cases where tested system consists of 2 or more
// OS-level processes. This is where lonet comes into play:
//
// Similarly to pipenet addresses on lonet are host:port pairs and several
// hosts could be created with different names. A host is xnet.Networker and
// so can be worked with similarly to regular TCP network access-point with
// Dial/Listen/Accept. Host's ports allocation is predictable: ports of a host
// are contiguous integer sequence starting from 1 that are all initially free,
// and whenever autobind is requested the first free port of the host will be
// used.
//
// Internally lonet network maintains registry of hosts so that lonet
// addresses could be resolved to OS-level addresses, for example α:1 and β:1
// to 127.0.0.1:4567 and 127.0.0.1:8765, and once lonet connection is
// established it becomes served by OS-level TCP connection over loopback.
//
// Example:
//
//	net, err := lonet.Join(ctx, "mynet")
//	hα, err := net.NewHost(ctx, "α")
//	hβ, err := net.NewHost(ctx, "β")
//
//	// starts listening on address "α:10"
//	l, err := hα.Listen(":10")
//	go func() {
//		csrv, err := l.Accept()   // csrv will have LocalAddr "α:1"
//	}()
//	ccli, err := hβ.Dial(ctx, "α:10") // ccli will be connection between "β:1" - "α:1"
//
// Once again lonet is similar to pipenet, but since it works via OS TCP stack
// it could be handy for testing networked application when there are several
// OS-level processes involved.
package lonet

// Lonet organization
//
// For every lonet network there is a registry with information about hosts
// available on the network, and for each host its OS-level listening address.
// The registry is kept as SQLite database under
//
//	/<tmp>/lonet/<network>/registry.db
//
// Whenever host α needs to establish connection to address on host β, it
// queries the registry for β and further talks to β on that address.
// Correspondingly when a host joins the network, it announces itself to the
// registry so that other hosts could see it.
//
//
// Handshake protocol
//
// After α establishes OS-level connection to β via main β address, it sends
// request to further establish lonet connection on top of that:
//
//	> lonet "<network>" dial "<α:portα>" "<β:portβ>"\n
//
// β checks whether portβ is listening, and if yes, accepts the connection on
// corresponding on-β listener with giving feedback to α that connection was
// accepted:
//
//	< lonet "<network>" connected "<β:portβ'>"\n
//
// After that connection is considered to be lonet-established and all further
// exchange on it is directly controlled by corresponding lonet-level
// Read/Write on α and β.
//
// If, on the other hand, lonet-level connection cannot be established, β replies:
//
//	< lonet "<networkβ>" E "<error>"\n
//
// where <error> could be:
//
//	- connection refused	if <β:portβ> is not listening
//	- network mismatch	if β thinks it works on different lonet network than α
//	- protocol error	if β thinks that α send incorrect dial request
//	- ...

import (
	"context"
	stderrors "errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/pkg/errors"

	"lab.nexedi.com/kirr/go123/xerr"
	"lab.nexedi.com/kirr/go123/xnet"
	"lab.nexedi.com/kirr/go123/xnet/virtnet"
)

const netPrefix = "lonet" // lonet package creates only "lonet*" networks


// protocolError represents logical error in lonet handshake exchange.
type protocolError struct {
	err error
}

// subNetwork represents one subnetwork of a lonet network.
type subNetwork struct {
	vnet *virtnet.SubNetwork

	// OS-level listener of this subnetwork.
	// whenever connection to subnet's host is tried to be established it goes here.
	oslistener net.Listener

	// accepted connections are further routed here for virtnet to handle.
	vnotify virtnet.Notifier

	// cancel for spawned .serve(ctx)
	serveCancel func()
}

// vengine implements virtnet.Engine for subNetwork.
type vengine struct {
	subnet *subNetwork
}

var tcp4 = xnet.NetPlain("tcp4")

// Join joins or creates new lonet network with given name.
//
// Network is the name of this network under "lonet" namespace, e.g. "α" will
// give full network name "lonetα".
//
// If network is "" new network with random unique name will be created.
//
// Join returns new subnetwork on the joined network.
//
// See package lab.nexedi.com/kirr/go123/xnet/virtnet for documentation on how
// to use returned subnetwork.
func Join(ctx context.Context, network string) (_ *virtnet.SubNetwork, err error) {
	defer xerr.Contextf(&err, "lonet: join %q", network)

	// create/join registry under /tmp/lonet/<network>/registry.db
	lonet := os.TempDir() + "/lonet"
	err = os.MkdirAll(lonet, 0777 | os.ModeSticky)
	if err != nil {
		return nil, err
	}

	var netdir string
	if network != "" {
		netdir = lonet + "/" + network
		err = os.MkdirAll(netdir, 0700)
	} else {
		// new with random name
		netdir, err = ioutil.TempDir(lonet, "")
		network = filepath.Base(netdir)
	}
	if err != nil {
		return nil, err
	}

	registry, err := openRegistrySQLite(ctx, netdir + "/registry.db", network)
	if err != nil {
		return nil, err
	}

	// start OS listener
	oslistener, err := tcp4.Listen("127.0.0.1:")
	if err != nil {
		registry.Close()
		return nil, err
	}

	// joined ok
	losubnet := &subNetwork{oslistener: oslistener}
	engine := &vengine{losubnet}
	subnet, vnotify := virtnet.NewSubNetwork(netPrefix + network, engine, registry)
	losubnet.vnet = subnet
	losubnet.vnotify = vnotify

	serveCtx, serveCancel := context.WithCancel(context.Background())
	losubnet.serveCancel = serveCancel
	go losubnet.serve(serveCtx)

	return subnet, nil
}

// ---- subnetwork OS-level serving ----

// Close implements virtnet.Engine .
func (v *vengine) Close() (err error) {
	n := v.subnet
	defer xerr.Contextf(&err, "lonet %q: close", n.network())

	n.serveCancel()             // this will cancel loaccepts spawned by serve
	return n.oslistener.Close() // this will interrupt Accept in serve
}

// serve serves incoming OS-level connections to this subnetwork.
//
// for every accepted connection lonet-handshake is initiated.
func (n *subNetwork) serve(ctx context.Context) {
	var wg sync.WaitGroup
	defer wg.Wait()

	// wait for incoming OS connections and do lonet protocol handshake on them.
	// if successful - route handshaked connection to particular Host's listener.
	for {
		osconn, err := n.oslistener.Accept()
		if err != nil {
			// mark subnetwork as being down and stop
			n.vnotify.VNetDown(err)
			return
		}

		wg.Add(1)
		go func(osconn net.Conn) {
			defer wg.Done()

			err := n.loaccept(ctx, osconn)
			if err == nil {
				return
			}

			// log if error is unexpected
			switch errors.Cause(err) {
			case virtnet.ErrConnRefused,
			     context.Canceled,
			     context.DeadlineExceeded:
				return // all ok - don't log.
			}

			log.Printf("lonet %q: serve %s <- %s : %s", n.network(),
				n.oslistener.Addr(), osconn.RemoteAddr(), err)
		}(osconn)
	}
}


// ---- acceptor and dialer that talk to each other via lonet handshake protocol ----

// loaccept handles incoming OS-level connection.
//
// It performs lonet protocol handshake as listener, and if successful further
// conveys accepted connection to lonet-level Accept.
//
// If handshake is not successful the connection is closed.
func (n *subNetwork) loaccept(ctx context.Context, osconn net.Conn) (err error) {
	defer xerr.Context(&err, "loaccept")

	// close osconn on error
	osconnClosed := false
	defer func() {
		if err != nil && !osconnClosed {
			osconn.Close()
		}
	}()

	// spawn accept
	type ret struct { err error }
	doneq := make(chan ret)
	go func() {
		err := n._loaccept(ctx, osconn)
		doneq <- ret{err}
	}()

	// wait for completion / interrupt IO on ctx cancel
	select {
	case <-ctx.Done():
		osconnClosed = true
		osconn.Close()
		<-doneq
		return ctx.Err()

	case ret := <-doneq:
		return ret.err
	}
}

func (n *subNetwork) _loaccept(ctx context.Context, osconn net.Conn) error {
	// read handshake line and parse it
	line, err := readline(osconn, 1024) // limit line length not to cause memory dos
	if err != nil {
		return err
	}

	// replyf performs formatted reply to osconn.
	// the error returned is for result of osconn.Write.
	replyf := func(format string, argv ...interface{}) error {
		line := fmt.Sprintf("< lonet %q " + format + "\n",
				append([]interface{}{n.network()}, argv...)...)
		_, err := osconn.Write([]byte(line))
		return err
	}

	// ereply performs error reply to osconn.
	// for convenience returned error is the error itself, not the
	// error returned from osconn.Write.
	ereply := func(err error) error {
		replyf("E %q", err) // ignore osconn.Write error
		return err
	}

	// eproto prepares protocol error and replies it to osconn.
	//
	// the error sent to peer contains only ereason, not details.
	// for convenience returned error is protocol error constructed from
	// error reason and details.
	//
	// error from osconn.Write is ignored.
	eproto := func(ereason, detailf string, argv ...interface{}) error {
		ereply(protocolErrorf(ereason))
		return protocolErrorf(ereason + ": " + detailf, argv...)
	}

	var network, src, dst string
	_, err = fmt.Sscanf(line, "> lonet %q dial %q %q\n", &network, &src, &dst)
	if err != nil {
		return eproto("invalid dial request", "%q", line)
	}

	if network != n.network() {
		return eproto("network mismatch", "%q", network)
	}

	asrc, err := virtnet.ParseAddr(network, src)
	if err != nil {
		return eproto("src address invalid", "%q", src)
	}
	adst, err := virtnet.ParseAddr(network, dst)
	if err != nil {
		return eproto("dst address invalid", "%q", dst)
	}

	defer xerr.Contextf(&err, "%s <- %s", dst, src)

	accept, err := n.vnotify.VNetAccept(ctx, asrc, adst, osconn)
	if err != nil {
		return ereply(err)
	}

	err = replyf("connected %q", accept.Addr)
	accept.Ack <- err
	return err
}

func (n *subNetwork) _loconnect(osconn net.Conn, src, dst *virtnet.Addr) (*virtnet.Addr, error) {
	_, err := osconn.Write([]byte(fmt.Sprintf("> lonet %q dial %q %q\n", n.network(), src, dst)))
	if err != nil {
		return nil, err
	}

	line, err := readline(osconn, 1024)
	if err != nil {
		return nil, err
	}

	var network, reply, arg string
	_, err = fmt.Sscanf(line, "< lonet %q %s %q\n", &network, &reply, &arg)
	if err != nil {
		return nil, protocolErrorf("invalid dial reply: %q", line)
	}

	switch reply {
	default:
		return nil, protocolErrorf("invalid reply verb: %q", reply)

	case "E":
		switch arg {
		// handle canonical errors like ErrConnRefused
		case "connection refused":
			err = virtnet.ErrConnRefused
		default:
			err = stderrors.New(arg)
		}

		return nil, err

	case "connected":
		// ok
	}

	if network != n.network() {
		return nil, protocolErrorf("connected, but network mismatch: %q", network)
	}

	acceptAddr, err := virtnet.ParseAddr(network, arg)
	if err != nil {
		return nil, protocolErrorf("connected, but accept address invalid: %q", acceptAddr)
	}
	if acceptAddr.Host != dst.Host {
		return nil, protocolErrorf("connected, but accept address is for different host: %q", acceptAddr.Host)
	}

	// everything is ok
	return acceptAddr, nil
}

// loconnect tries to establish lonet connection on top of OS-level connection.
//
// It performs lonet protocol handshake as dialer, and if successful returns
// lonet-level peer's address of the accepted lonet connection.
//
// If handshake is not successful the connection is closed.
func (n *subNetwork) loconnect(ctx context.Context, osconn net.Conn, src, dst *virtnet.Addr) (acceptAddr *virtnet.Addr, err error) {
	defer func() {
		switch err {
		default:
			// n.network, src, dst will be provided by virtnet while
			// wrapping us with net.OpError{"dial", ...}
			xerr.Contextf(&err, "loconnect %s", osconn.RemoteAddr())

		// this errors remain unwrapped
		case nil:
		case virtnet.ErrConnRefused:
		}
	}()

	// close osconn on error
	osconnClosed := false
	defer func() {
		if err != nil && !osconnClosed {
			osconn.Close()
		}
	}()

	// spawn connect
	type ret struct { acceptAddr *virtnet.Addr; err error }
	doneq := make(chan ret)
	go func() {
		acceptAddr, err := n._loconnect(osconn, src, dst)
		doneq <- ret{acceptAddr, err}
	}()

	// wait for completion / interrupt IO on ctx cancel
	select {
	case <-ctx.Done():
		osconnClosed = true
		osconn.Close()
		<-doneq
		return nil, ctx.Err()

	case ret := <-doneq:
		acceptAddr, err = ret.acceptAddr, ret.err
		return acceptAddr, err
	}
}

// VNetDial implements virtnet.Engine .
func (v *vengine) VNetDial(ctx context.Context, src, dst *virtnet.Addr, dstosladdr string) (_ net.Conn, addrAccept *virtnet.Addr, _ error) {
	n := v.subnet

	// dial to OS addr for host and perform lonet handshake
	osconn, err := tcp4.Dial(ctx, dstosladdr)
	if err != nil {
		return nil, nil, err
	}

	addrAccept, err = n.loconnect(ctx, osconn, src, dst)
	if err != nil {
		return nil, nil, err
	}

	return osconn, addrAccept, nil
}


// ----------------------------------------

// VNetNewHost implements virtnet.Engine .
func (v *vengine) VNetNewHost(ctx context.Context, hostname string, registry virtnet.Registry) error {
	n := v.subnet

	// no need to create host resources - we accept all connections on 1
	// port for whole subnetwork.
	return registry.Announce(ctx, hostname, n.oslistener.Addr().String())
}

// network returns name of the network this subnetwork is part of.
func (n *subNetwork) network() string {
	return n.vnet.Network()
}

// Error implements error.
func (e *protocolError) Error() string {
	return fmt.Sprintf("protocol error: %s", e.err)
}

// protocolErrorf constructs protocolError with error formatted via fmt.Errorf .
func protocolErrorf(format string, argv ...interface{}) *protocolError {
	return &protocolError{fmt.Errorf(format, argv...)}
}


// readline reads 1 line from r up to maxlen bytes.
func readline(r io.Reader, maxlen int) (string, error) {
	buf1 := []byte{0}
	var line []byte
	for len(line) < maxlen {
		n, err := r.Read(buf1)
		if n == 1 {
			err = nil
		}
		if err != nil {
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return string(line), err
		}

		line = append(line, buf1...)
		if buf1[0] == '\n' {
			break
		}
	}

	return string(line), nil
}
