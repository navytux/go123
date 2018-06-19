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

package virtnet
// interfaces that virtnet uses in its working.

import (
	"context"
	"errors"
	"fmt"
	"net"
)

// Engine is the interface for particular virtnet network implementation to be
// used by SubNetwork.
//
// A virtnet network implementation should provide Engine instance to
// SubNetwork when creating it. The subnetwork will use provided engine for its
// operations.
//
// It should be safe to access Engine from multiple goroutines simultaneously.
type Engine interface {
	// VNetNewHost creates resources for host and announces it to registry.
	//
	// VNetNewHost should create resources for new host and announce
	// hostname to provided registry. When announcing it should encode in
	// hostdata a way for VNetDial - potentially run on another subnetwork
	// - to find out where to connect to when dialing to this host.
	//
	// On error the returned error will be wrapped by virtnet with "new
	// host" operation and hostname.
	VNetNewHost(ctx context.Context, hostname string, registry Registry) error

	// VNetDial creates outbound virtnet connection.
	//
	// VNetDial, given destination virtnet address and destination
	// hostdata, should establish connection to destination. It should let
	// remote side know that its peer virtnet address is src.
	//
	// On success net.Conn that will be handling data exchange via its
	// Read/Write should be returned. This net.Conn will be wrapped by
	// virtnet with overwritten LocalAddr and RemoteAddr to be src and
	// addrAccept correspondingly.
	//
	// On error the returned error will be wrapped by virtnet with
	// corresponding net.OpError{"dial", src, dst}.
	//
	// Virtnet always passes to VNetDial src and dst with the same network
	// name that was used when creating corresponding SubNetwork.
	VNetDial(ctx context.Context, src, dst *Addr, dsthostdata string) (_ net.Conn, addrAccept *Addr, _ error)

	// Close shuts down subnetwork engine.
	//
	// Close should close engine resources and return corresponding error.
	//
	// There is no need to explicitly interrupt other engine operations -
	// to those virtnet always passes ctx that is canceled before
	// engine.Close is called.
	Close() error
}


// Notifier is the interface to be used by particular virtnet network
// implementation for notifying SubNetwork.
//
// A virtnet network implementation receives instance of Notifier together with
// SubNetwork when creating it. The network implementation should use provided
// Notifier to notify the subnetwork to handle incoming events.
//
// It should be safe to access Notifier from multiple goroutines simultaneously.
type Notifier interface {
	// VNetAccept notifies virtnet about incoming connection.
	//
	// VNetAccept, given destination virtnet address, should make decision
	// to either accept or reject provided connection.
	//
	// On success the connection is pre-accepted and corresponding Accept
	// is returned to virtnet network implementation.
	//
	// On error an error is returned without any "accept" prefix, e.g.
	// ErrConnRefused. Such accept prefix should be provided by network
	// implementation that is using VNetAccept.
	VNetAccept(ctx context.Context, src, dst *Addr, netconn net.Conn) (*Accept, error)

	// VNetDown notifies virtnet that underlying network is down.
	//
	// Provided err describes the cause of why the network is down.
	VNetDown(err error)
}

// Accept represents successful acceptance decision from Notifier.VNetAccept .
//
// On successful accept decision corresponding virtnet-level Accept() is
// waiting on .Ack to continue and will check the error from there.
//
// On success the connection will be finally accepted and corresponding virtnet
// listener will be notified. Provided netconn will be adjusted by virtnet
// internally with overwritten LocalAddr and RemoteAddr to be correspondingly
// .Addr and src that was originally passed to VNetAccept.
//
// On error the acceptance will be canceled.
type Accept struct {
	Addr *Addr      // accepting with this local address
	Ack  chan error
}


// Registry represents access to a virtnet network registry.
//
// A virtnet network implementation should provide Registry instance to
// SubNetwork when creating it. The subnetwork will eventually use it when
// creating hosts via NewHost, and in turn creating outbound connections on
// them via Host.Dial.
//
// The registry holds information about hosts available on the network, and
// for each host additional data about it. Whenever host α needs to establish
// connection to address on host β, it queries the registry for β.
// Correspondingly when a host joins the network, it announces itself to the
// registry so that other hosts could see it.
//
// The registry could be implemented in several ways, for example:
//
//	- dedicated network server,
//	- hosts broadcasting information to each other similar to ARP,
//	- shared memory or file,
//	- ...
//
// It should be safe to access registry from multiple goroutines simultaneously.
type Registry interface {
	// Announce announces host to registry.
	//
	// Returned error, if !nil, is *RegistryError with .Err describing the
	// error cause:
	//
	//	- ErrRegistryDown  if registry cannot be accessed,
	//	- ErrHostDup       if hostname was already announced,
	//	- some other error indicating e.g. IO problem.
	Announce(ctx context.Context, hostname, hostdata string) error

	// Query queries registry for host.
	//
	// Returned error, if !nil, is *RegistryError with .Err describing the
	// error cause:
	//
	//	- ErrRegistryDown  if registry cannot be accessed,
	//	- ErrNoHost        if hostname was not announced to registry,
	//	- some other error indicating e.g. IO problem.
	Query(ctx context.Context, hostname string) (hostdata string, _ error)

	// Close closes access to registry.
	//
	// Close should close registry resources and return corresponding error.
	//
	// There is no need to explicitly interrupt other registry operations -
	// to those virtnet always passes ctx that is canceled before
	// registry.Close is called.
	Close() error
}

var (
	ErrRegistryDown = errors.New("registry is down")
	ErrNoHost       = errors.New("no such host")
	ErrHostDup      = errors.New("host already registered")
)

// RegistryError represents an error of a registry operation.
type RegistryError struct {
	Registry string      // name of the registry
	Op       string      // operation that failed
	Args     interface{} // operation arguments, if any
	Err      error       // actual error that occurred during the operation
}

func (e *RegistryError) Error() string {
	s := e.Registry + ": " + e.Op
	if e.Args != nil {
		s += fmt.Sprintf(" %q", e.Args)
	}
	s += ": " + e.Err.Error()
	return s
}

func (e *RegistryError) Cause() error {
	return e.Err
}
