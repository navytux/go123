// Copyright (C) 2017-2021  Nexedi SA and Contributors.
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

// Package pipenet provides TCP-like synchronous in-memory network of net.Pipes.
//
// Addresses on pipenet are host:port pairs. A host is xnet.Networker and so
// can be worked with similarly to regular TCP network with Dial/Listen/Accept/...
//
// Example:
//
//	net := pipenet.New("")
//	h1 := net.Host("abc")
//	h2 := net.Host("def")
//
//	l, err := h1.Listen(ctx, ":10")     // starts listening on address "abc:10"
//	go func() {
//		csrv, err := l.Accept(ctx)  // csrv will have LocalAddr "abc:1"
//	}()
//	ccli, err := h2.Dial(ctx, "abc:10") // ccli will be connection between "def:1" - "abc:1"
//
// Pipenet might be handy for testing interaction of networked applications in 1
// process without going to OS networking stack.
//
// See also package lab.nexedi.com/kirr/go123/xnet/lonet for similar network
// that can work across several OS-level processes.
package pipenet

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/pkg/errors"

	"lab.nexedi.com/kirr/go123/xnet/virtnet"
)

const netPrefix = "pipe" // pipenet package creates only "pipe*" networks

// Network implements synchronous in-memory TCP-like network of pipes.
type Network struct {
	vnet    *virtnet.SubNetwork
	vnotify virtnet.Notifier
}

// vengine implements virtnet.Engine for Network.
type vengine struct {
	network *Network
}

// ramRegistry implements virtnet.Registry in RAM.
//
// Pipenet does not need a registry but virtnet is built for general case which
// needs one.
//
// Essentially it works as map protected by mutex.
type ramRegistry struct {
	name string

	mu      sync.Mutex
	hostTab map[string]string // hostname -> hostdata
	closed  bool              // 1 after Close
}

// New creates new pipenet Network.
//
// Name is name of this network under "pipe" namespace, e.g. "α" will give full
// network name "pipeα".
//
// New does not check whether network name provided is unique.
func New(name string) *Network {
	netname := netPrefix + name
	n := &Network{}
	v := &vengine{n}
	r := newRAMRegistry(fmt.Sprintf("ram(%s)", netname))
	subnet, vnotify := virtnet.NewSubNetwork(netname, v, r)
	n.vnet = subnet
	n.vnotify = vnotify
	return n
}

// AsVirtNet exposes Network as virtnet subnetwork.
//
// Since pipenet works entirely in RAM and in 1 OS process, its user interface
// is simpler compared to more general virtnet - for example there is no error
// when creating hosts. However sometimes it is handy to get access to pipenet
// network via full virtnet interface, when the code that is using pipenet
// network does not want to depend on pipenet API specifics.
func AsVirtNet(n *Network) *virtnet.SubNetwork {
	return n.vnet
}

// Network returns name of the network.
func (n *Network) Network() string {
	return n.vnet.Network()
}

// Host returns network access point by name.
//
// If there was no such host before it creates new one.
//
// Host panics if underlying virtnet subnetwork was shut down.
func (n *Network) Host(name string) *virtnet.Host {
	// check if it is already there
	host := n.vnet.Host(name)
	if host != nil {
		return host
	}

	// if not - create it. Creation will not block.
	host, err := n.vnet.NewHost(context.Background(), name)
	if host != nil {
		return host
	}

	// the only way we could get error here is due to either someone else
	// making the host in parallel to us, or virtnet shutdown.
	switch errors.Cause(err) {
	case virtnet.ErrHostDup:
		// ok
	case virtnet.ErrNetDown:
		panic(err)

	default:
		panic(fmt.Sprintf("pipenet: NewHost failed not due to dup or shutdown: %s", err))
	}

	// if it was dup - we should be able to get it.
	//
	// even if dup.Close is called in the meantime it will mark the host as
	// down, but won't remove it from vnet .hostMap.
	host = n.vnet.Host(name)
	if host == nil {
		panic("pipenet: NewHost said host already is there, but it was not found")
	}

	return host
}

// VNetNewHost implements virtnet.Engine .
func (v *vengine) VNetNewHost(ctx context.Context, hostname string, registry virtnet.Registry) error {
	// for pipenet there is neither need to create host resources, nor need
	// to keep any hostdata.
	return registry.Announce(ctx, hostname, "")
}

// VNetDial implements virtnet dialing for pipenet.
//
// Simply create pipe pair and send one end directly to virtnet acceptor.
func (v *vengine) VNetDial(ctx context.Context, src, dst *virtnet.Addr, _ string) (_ net.Conn, addrAccept *virtnet.Addr, _ error) {
	pc, ps := net.Pipe()
	accept, err := v.network.vnotify.VNetAccept(ctx, src, dst, ps)
	if err != nil {
		pc.Close()
		ps.Close()
		return nil, nil, err
	}

	accept.Ack <- nil
	return pc, accept.Addr, nil
}

// Close implements virtnet.Engine .
func (v *vengine) Close() error {
	return nil // nop: there is no underlying resources to release.
}



// Announce implements virtnet.Registry .
func (r *ramRegistry) Announce(ctx context.Context, hostname, hostdata string) (err error) {
	defer r.regerr(&err, "announce", hostname, hostdata)

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return virtnet.ErrRegistryDown
	}

	if _, already := r.hostTab[hostname]; already {
		return virtnet.ErrHostDup
	}

	r.hostTab[hostname] = hostdata
	return nil
}

// Query implements virtnet.Registry .
func (r *ramRegistry) Query(ctx context.Context, hostname string) (hostdata string, err error) {
	defer r.regerr(&err, "query", hostname)

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return "", virtnet.ErrRegistryDown
	}

	hostdata, ok := r.hostTab[hostname]
	if !ok {
		return "", virtnet.ErrNoHost
	}

	return hostdata, nil
}

// Close implements virtnet.Registry .
func (r *ramRegistry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.closed = true
	return nil
}

func newRAMRegistry(name string) *ramRegistry {
	return &ramRegistry{name: name, hostTab: make(map[string]string)}
}

// regerr is syntactic sugar to wrap !nil *errp into RegistryError.
//
// intended too be used like
//
//	defer r.regerr(&err, "operation", arg1, arg2, ...)
func (r *ramRegistry) regerr(errp *error, op string, args ...interface{}) {
	if *errp == nil {
		return
	}

	*errp = &virtnet.RegistryError{
		Registry: r.name,
		Op:       op,
		Args:     args,
		Err:      *errp,
	}
}
