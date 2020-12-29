# -*- coding: utf-8 -*-
# Copyright (C) 2018-2020  Nexedi SA and Contributors.
#                          Kirill Smelkov <kirr@nexedi.com>
#
# This program is free software: you can Use, Study, Modify and Redistribute
# it under the terms of the GNU General Public License version 3, or (at your
# option) any later version, as published by the Free Software Foundation.
#
# You can also Link and Combine this program with other software covered by
# the terms of any of the Free Software licenses or any of the Open Source
# Initiative approved licenses and Convey the resulting work. Corresponding
# source of such a combination shall include the source code for all other
# software used.
#
# This program is distributed WITHOUT ANY WARRANTY; without even the implied
# warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
#
# See COPYING file for full licensing terms.
# See https://www.nexedi.com/licensing for rationale and options.
"""Package lonet provides TCP network simulated on top of localhost TCP loopback.

See lonet.go for lonet description, organization and protocol.
"""

# NOTE this package is deliberately concise and follows lonet.go structure,
# which is more well documented.

import sys, os, stat, errno, tempfile, re
import socket as net

import sqlite3
import functools
import threading
import logging as log

from golang import func, defer, go, chan, select, default, panic, gimport
from golang import sync
from golang.gcompat import qq

xerr    = gimport('lab.nexedi.com/kirr/go123/xerr')
Error   = xerr.Error
errctx  = xerr.context
errcause= xerr.cause


# set_once sets threading.Event, but only once.
#
# it returns whether event was set.
#
#   if set_once(down_once):
#     ...
#
# is analog of
#
#   downOnce.Do(...)
#
# in Go.
#
# TODO just use sync.Once from pygolang.
_oncemu = threading.Lock()
def set_once(event):
    with _oncemu:
        if event.is_set():
            return False
        event.set()
        return True



# -------- virtnet --------
#
# See ../virtnet/virtnet.go for details.

# neterror creates net.error and registers it as WDE to xerr.
def neterror(*argv):
    err = net.error(*argv)
    xerr.register_wde_object(err)
    return err

ErrNetDown          = neterror(errno.EBADFD,         "network is down")
ErrHostDown         = neterror(errno.EBADFD,         "host is down")
ErrSockDown         = neterror(errno.EBADFD,         "socket is down")
ErrAddrAlreadyUsed  = neterror(errno.EADDRINUSE,     "address already in use")
ErrAddrNoListen     = neterror(errno.EADDRNOTAVAIL,  "cannot listen on requested address")
ErrConnRefused      = neterror(errno.ECONNREFUSED,   "connection refused")

ErrNoHost           = neterror("no such host")
ErrHostDup          = neterror("host already registered")


# addrstr4 formats host:port as if for TCP4 network.
def addrstr4(host, port):
    return "%s:%d" % (host, port)

# Addr represent address of a virtnet endpoint.
class Addr(object):
    # .net  str
    # .host str
    # .port int

    def __init__(self, net, host, port):
        self.net, self.host, self.port = net, host, port

    # netaddr returns address as net.AF_INET (host, port) pair.
    def netaddr(self):
        return (self.host, self.port)

    def __str__(self):
        return addrstr4(*self.netaddr())

    def __eq__(a, b):
        return isinstance(b, Addr) and a.net == b.net and a.host == b.host and a.port == b.port


# VirtSubNetwork represents one subnetwork of a virtnet network.
class VirtSubNetwork(object):
    # ._network     str
    # ._registry    Registry
    # ._hostmu      μ
    # ._hostmap     {} hostname -> Host
    # ._nopenhosts  int
    # ._autoclose   bool
    # ._down        chan ø
    # ._down_once   threading.Event

    def __init__(self, network, registry):
        self._network    = network
        self._registry   = registry
        self._hostmu     = threading.Lock()
        self._hostmap    = {}
        self._nopenhosts = 0
        self._autoclose  = False
        self._down       = chan()
        self._down_once  = threading.Event()

    # must be implemented in particular virtnet implementation
    def _vnet_newhost(self, hostname, registry):    raise NotImplementedError()
    def _vnet_dial(self, src, dst, dstosladdr):     raise NotImplementedError()
    def _vnet_close(self):                          raise NotImplementedError()


# Host represents named access point on a virtnet network.
class Host(object):
    # ._subnet      VirtSubNetwork
    # ._name        str
    # ._sockmu      μ
    # ._socketv     []socket ; port -> listener | conn ; [0] is always None
    # ._down        chan ø
    # ._down_once   threading.Event
    # ._close_once  sync.Once

    def __init__(self, subnet, name):
        self._subnet     = subnet
        self._name       = name
        self._sockmu     = threading.Lock()
        self._socketv    = []
        self._down       = chan()
        self._down_once  = threading.Event()
        self._close_once = sync.Once()


# socket represents one endpoint entry on Host.
class socket(object):
    # ._host    Host
    # ._port    int

    # ._conn      conn | None
    # ._listener  listener | None

    def __init__(self, host, port):
        self._host, self._port = host, port
        self._conn = self._listener = None


# conn represents one endpoint of a virtnet connection.
class conn(object):
    # ._socket      socket
    # ._peerAddr    Addr
    # ._netsk       net.socket (embedded)
    # ._down        chan()
    # ._down_once   threading.Event
    # ._close_once  threading.Event

    def __init__(self, sk, peerAddr, netsk):
        self._socket, self._peerAddr, self._netsk = sk, peerAddr, netsk
        self._down       = chan()
        self._down_once  = threading.Event()
        self._close_once = threading.Event()

    # ._netsk embedded:
    def __getattr__(self, name):
        return getattr(self._netsk, name)


# listener implements net.Listener for Host.
class listener(object):
    # ._socket      socket
    # ._dialq       chan dialReq
    # ._down        chan ø
    # ._down_once   threading.Event
    # ._close_once  threading.Event

    def __init__(self, sk):
        self._socket     = sk
        self._dialq      = chan()
        self._down       = chan()
        self._down_once  = threading.Event()
        self._close_once = threading.Event()


# dialReq represents one dial request to listener from acceptor.
class dialReq(object):
    # ._from    Addr
    # ._netsk   net.socket
    # ._resp    chan Accept

    def __init__(self, from_, netsk, resp):
        self._from, self._netsk, self._resp = from_, netsk, resp


# Accept represents successful acceptance decision from VirtSubNetwork._vnet_accept .
class Accept(object):
    # .addr     Addr
    # .ack      chan error
    def __init__(self, addr, ack):
        self.addr, self.ack = addr, ack


# ----------------------------------------

# _shutdown is worker for close and _vnet_down.
@func(VirtSubNetwork)
def _shutdown(n, exc):
    n.__shutdown(exc, True)
@func(VirtSubNetwork)
def __shutdown(n, exc, withHosts):
    if not set_once(n._down_once):
        return

    n._down.close()

    if withHosts:
        with n._hostmu:
            for host in n._hostmap.values():
                host._shutdown()

    # XXX py: we don't collect / remember .downErr
    if exc is not None:
        log.error(exc)
    n._vnet_close()
    n._registry.close()


# close shutdowns subnetwork.
@func(VirtSubNetwork)
def close(n):
    n.__close(True)
@func(VirtSubNetwork)
def _closeWithoutHosts(n):
    n.__close(False)
@func(VirtSubNetwork)
def __close(n, withHosts):
    with errctx("virtnet %s: close" % qq(n._network)):
        n.__shutdown(None, withHosts)

# _vnet_down shutdowns subnetwork upon engine error.
@func(VirtSubNetwork)
def _vnet_down(n, exc):
    # XXX py: errctx here (go does not have) because we do not reraise .downErr in close
    with errctx("virtnet %s: shutdown" % qq(n._network)):
        n._shutdown(exc)


# new_host creates new Host with given name.
@func(VirtSubNetwork)
def new_host(n, name):
    with errctx("virtnet %s: new host %s" % (qq(n._network), qq(name))):
        n._vnet_newhost(name, n._registry)
        # XXX check err due to subnet down

        with n._hostmu:
            if name in n._hostmap:
                panic("announced ok but .hostMap already !empty" % (qq(n._network), qq(name)))

            host = Host(n, name)
            n._hostmap[name] = host
            n._nopenhosts += 1
            return host


# host returns host on the subnetwork by name.
@func(VirtSubNetwork)
def host(n, name):
    with n._hostmu:
        return n._hostmap.get(name)


# _shutdown is underlying worker for close.
@func(Host)
def _shutdown(h):
    if not set_once(h._down_once):
        return

    h._down.close()

    with h._sockmu:
        for sk in h._socketv:
            if sk is None:
                continue
            if sk._conn is not None:
                sk._conn._shutdown()
            if sk._listener is not None:
                sk._listener._shutdown()

# close shutdowns host.
@func(Host)
def close(h):
    def autoclose():
        def _():
            n = h._subnet
            with n._hostmu:
                n._nopenHosts -= 1
                if n._nopenHosts < 0:
                    panic("SubNetwork._nopenHosts < 0")
                if n._autoclose and n._nopenHosts == 0:
                    n._closeWithoutHosts()
        h._close_once.do(_)
    defer(autoclose)

    with errctx("virtnet %s: host %s: close" % (qq(h._subnet._network), qq(h._name))):
        h._shutdown()

# autoclose schedules close to be called after last host on this subnetwork is closed.
@func(VirtSubNetwork)
def autoclose(n):
    with n._hostmu:
        if n._nopenHosts == 0:
            panic("BUG: no opened hosts")
        n._autoclose = True


# listen starts new listener on the host.
@func(Host)
def listen(h, laddr):
    if laddr == "":
        laddr = ":0"

    with errctx("listen %s %s" % (h.network(), laddr)):
        a = h._parseAddr(laddr)

        if a.host != h._name:
            raise ErrAddrNoListen

        if ready(h._down):
            h._excDown()

        with h._sockmu:
            if a.port == 0:
                sk = h._allocFreeSocket()

            else:
                while a.port >= len(h._socketv):
                    h._socketv.append(None)

                if h._socketv[a.port] is not None:
                    raise ErrAddrAlreadyUsed

                sk = socket(h, a.port)
                h._socketv[a.port] = sk

            l = listener(sk)
            sk._listener = l

            return l


# _shutdown shutdowns the listener.
@func(listener)
def _shutdown(l):
    if set_once(l._down_once):
        l._down.close()

# close closes the listener.
@func(listener)
def close(l):
    l._shutdown()
    if not set_once(l._close_once):
        return

    sk = l._socket
    h  = sk._host

    with h._sockmu:
        sk._listener = None
        if sk._empty():
            h._socketv[sk.port] = None


# accept tries to connect to dial called with addr corresponding to our listener.
@func(listener)
def accept(l):
    h = l._socket._host

    with errctx("accept %s %s" % (h.network(), l.addr())):
        while 1:
            _, _rx = select(
                l._down.recv,   # 0
                l._dialq.recv,  # 1
            )
            if _ == 0:
                l._excDown()
            if _ == 1:
                req = _rx

            with h._sockmu:
                sk = h._allocFreeSocket()

            ack = chan()
            req._resp.send(Accept(sk.addr(), ack))

            _, _rx = select(
                l._down.recv,   # 0
                ack.recv,       # 1
            )
            if _ == 0:
                def purgesk():
                    err = ack.recv()
                    if err is None:
                        try:
                            req._netsk.close()
                        except:
                            pass
                    with h._sockmu:
                        h._socketv[sk._port] = None

                go(purgesk)
                l._excDown()

            if _ == 1:
                err = _rx

            if err is not None:
                with h._sockmu:
                    h._socketv[sk._port] = None
                continue

            c = conn(sk, req._from, req._netsk)
            with h._sockmu:
                sk.conn = c

            return c


# _vnet_accept accepts or rejects incoming connection.
@func(VirtSubNetwork)
def _vnet_accept(n, src, dst, netconn):
    with n._hostmu:
        host = n._hostmap.get(dst.host)
    if host is None:
        raise net.gaierror('%s: no such host' % dst.host)

    host._sockmu.acquire()

    if dst.port >= len(host._socketv):
        host._sockmu.release()
        raise ErrConnRefused

    sk = host._socketv[dst.port]
    if sk is None or sk._listener is None:
        host._sockmu.release()
        raise ErrConnRefused

    l = sk._listener
    host._sockmu.release()

    resp = chan()
    req  = dialReq(src, netconn, resp)

    _, _rx = select(
        l._down.recv,           # 0
        (l._dialq.send, req),   # 1
    )
    if _ == 0:
        raise ErrConnRefused
    if _ == 1:
        return resp.recv()


# dial dials address on the network.
@func(Host)
def dial(h, addr):
    with h._sockmu:
        sk = h._allocFreeSocket()

    # XXX py: default dst to addr to be able to render it in error if it happens before parse
    dst = addr

    try:
        dst = h._parseAddr(addr)
        n   = h._subnet

        # XXX cancel on host shutdown

        dstdata = n._registry.query(dst.host)
        if dstdata is None:
            raise ErrNoHost

        netsk, acceptAddr = n._vnet_dial(sk.addr(), dst, dstdata)

        c = conn(sk, acceptAddr, netsk)
        with h._sockmu:
            sk._conn = c
        return c

    except Exception as err:
        with h._sockmu:
            h._socketv[sk._port] = None

        _, _, tb = sys.exc_info()
        raise Error("dial %s %s->%s" % (h.network(), sk.addr(), dst), err, tb)


# ---- conn ----

# _shutdown closes underlying network connection.
@func(conn)
def _shutdown(c):
    if not set_once(c._down_once):
        return

    c._down.close()
    # XXX py: we don't remember .errClose
    c._netsk.close()


# close closes network endpoint and unregisters conn from Host.
@func(conn)
def close(c):
    c._shutdown()
    if set_once(c._close_once):
        sk = c._socket
        h  = sk._host

        with h._sockmu:
            sk._conn = None
            if sk._empty():
                h._socketv[sk._port] = None

    # XXX py: we don't reraise c.errClose

# XXX py: don't bother to override recv (Read)
# XXX py: don't bother to override send (Write)

# local_addr returns virtnet address of local end of connection.
@func(conn)
def local_addr(c):
    return c._socket.addr()

# getsockname returns virtnet address of local end of connection as net.AF_INET (host, port) pair.
@func(conn)
def getsockname(c):
    return c.local_addr().netaddr()

# remote_addr returns virtnet address of remote end of connection.
@func(conn)
def remote_addr(c):
    return c._peerAddr

# getpeername returns virtnet address of remote end of connection as net.AF_INET (host, port) pair.
@func(conn)
def getpeername(c):
    return c.remote_addr().netaddr()

# ----------------------------------------

# _allocFreeSocket finds first free port and allocates socket entry for it.
@func(Host)
def _allocFreeSocket(h):
    port = 1
    while port < len(h._socketv):
        if h._socketv[port] is None:
            break
        port += 1

    while port >= len(h._socketv):
        h._socketv.append(None)

    sk = socket(h, port)
    h._socketv[port] = sk
    return sk


# empty checks whether socket's both conn and listener are all nil.
@func(socket)
def _empty(sk):
    return (sk._conn is None and sk._listener is None)

# addr returns address corresponding to socket.
@func(socket)
def addr(sk):
    h = sk._host
    return Addr(h.network(), h.name(), sk._port)

# Addr.parse parses addr into virtnet address for named network.
@func(Addr)
@staticmethod
def parse(net, addr):
    try:
        addrv = addr.split(':')
        if len(addrv) != 2:
            raise ValueError()
        return Addr(net, addrv[0], int(addrv[1]))
    except:
        raise ValueError('%s is not valid virtnet address' % addr)

# _parseAddr parses addr into virtnet address from host point of view.
@func(Host)
def _parseAddr(h, addr):
    a = Addr.parse(h.network(), addr)
    if a.host == "":
        a.host = h._name
    return a

# addr returns address where listener is accepting incoming connections.
@func(listener)
def addr(l):
    return l._socket.addr()


# network returns full network name this subnetwork is part of.
@func(VirtSubNetwork)
def network(n):
    return n._network

# network returns full network name of underlying network.
@func(Host)
def network(h):
    return h._subnet.network()

# name returns host name.
@func(Host)
def name(h):
    return h._name

# ----------------------------------------

# _excDown raises appropriate exception cause when h.down is found ready.
@func(Host)
def _excDown(h):
    if ready(h._subnet._down):
        raise ErrNetDown
    else:
        raise ErrHostDown

# _excDown raises appropriate exception cause when l.down is found ready.
@func(listener)
def _excDown(l):
    h = l._socket._host
    n = h._subnet

    if ready(n._down):
        raise ErrNetDown
    elif ready(h._down):
        raise ErrHostDown
    else:
        raise ErrSockDown

# XXX py: conn.errOrDown is not implemented because conn.{Read,Write} are not wrapped.

# ready returns whether channel ch is ready.
def ready(ch):
    _, _rx = select(
        ch.recv,    # 0
        default,    # 1
    )
    if _ == 0:
        return True
    if _ == 1:
        return False


# -------- lonet networking --------
#
# See lonet.go for details.

# protocolError represents logical error in lonet handshake exchange.
class protocolError(Exception):
    pass

xerr.register_wde_class(protocolError)


# `mkdir -p`; https://stackoverflow.com/a/273227
def _mkdir_p(path, mode):
    try:
        os.makedirs(path, mode)
    except OSError as e:
        if e.errno != errno.EEXIST:
            raise

# join joins or creates new lonet network with given name.
def join(network):
    with errctx("lonet: join %s" % qq(network)):
        lonet = tempfile.gettempdir() + "/lonet"
        _mkdir_p(lonet, 0777 | stat.S_ISVTX)

        if network != "":
            netdir = lonet + "/" + network
            _mkdir_p(netdir, 0700)
        else:
            netdir = tempfile.mkdtemp(dir=lonet)
            network = os.path.basename(netdir)

        registry = SQLiteRegistry(netdir + "/registry.db", network)
        return _SubNetwork("lonet" + network, registry)


# lonet handshake:
# scanf("> lonet %q dial %q %q\n", network, src, dst)
# scanf("< lonet %q %s %q\n", network, reply, arg)
_lodial_re  = re.compile(r'> lonet "(?P<network>.*?[^\\])" dial "(?P<src>.*?[^\\])" "(?P<dst>.*?[^\\])"\n')
_loreply_re = re.compile(r'< lonet "(?P<network>.*?[^\\])" (?P<reply>[^\s]+) "(?P<arg>.*?[^\\])"\n')

# _SubNetwork represents one subnetwork of a lonet network.
class _SubNetwork(VirtSubNetwork):
    # ._oslistener  net.socket
    # ._tserve      Thread(._serve)

    def __init__(n, network, registry):
        super(_SubNetwork, n).__init__(network, registry)

        try:
            # start OS listener
            oslistener = net.socket(net.AF_INET, net.SOCK_STREAM)
            oslistener.bind(("127.0.0.1", 0))
            oslistener.listen(1024)

        except:
            registry.close()
            raise

        n._oslistener = oslistener

        # XXX -> go(n._serve, serveCtx) + cancel serveCtx in close
        n._tserve = threading.Thread(target=n._serve, name="%s/serve" % n._network)
        n._tserve.start()


    def _vnet_close(n):
        # XXX py: no errctx here - it is in _vnet_down
        # XXX cancel + join tloaccept*
        n._oslistener.close()
        n._tserve.join()


    # _serve serves incoming OS-level connections to this subnetwork.
    def _serve(n):
        # XXX net.socket.close does not interrupt sk.accept
        # XXX we workaround it with accept timeout and polling for ._down
        n._oslistener.settimeout(1E-3)   # 1ms
        while 1:
            if ready(n._down):
                break

            try:
                osconn, _ = n._oslistener.accept()
            except net.timeout:
                continue

            except Exception as e:
                n._vnet_down(e)
                return

            # XXX wg.Add(1)
            def _(osconn):
                # XXX defer wg.Done()

                myaddr   = addrstr4(*n._oslistener.getsockname())
                peeraddr = addrstr4(*osconn.getpeername())

                try:
                    n._loaccept(osconn)
                except Exception as e:
                    if errcause(e) is not ErrConnRefused:
                        log.error("lonet %s: serve %s <- %s : %s" % (qq(n._network), myaddr, peeraddr, e))

            go(_, osconn)


    # --- acceptor vs dialer ---

    # _loaccept handles incoming OS-level connection.
    def _loaccept(n, osconn):
        # XXX does not support interruption
        with errctx("loaccept"):
            try:
                n.__loaccept(osconn)
            except Exception:
                # close osconn on error
                osconn.close()
                raise

    def __loaccept(n, osconn):
        line = skreadline(osconn, 1024)

        def reply(reply):
            line = "< lonet %s %s\n" % (qq(n._network), reply)
            osconn.sendall(line)

        def ereply(err, tb):
            e = err
            if err is ErrConnRefused:
                e = "connection refused"  # str(ErrConnRefused) is "[Errno 111] connection refused"
            reply("E %s" % qq(e))
            if not xerr.well_defined(err):
                err = Error("BUG", err, cause_tb=tb)
            raise err

        def eproto(ereason, detail):
            reply("E %s" % qq(protocolError(ereason)))
            raise protocolError(ereason + ": " + detail)


        m = _lodial_re.match(line)
        if m is None:
            eproto("invalid dial request", "%s" % qq(line))

        network = m.group('network').decode('string_escape')
        src     = m.group('src').decode('string_escape')
        dst     = m.group('dst').decode('string_escape')

        if network != n._network:
            eproto("network mismatch", "%s" % qq(network))

        try:
            asrc = Addr.parse(network, src)
        except ValueError:
            eproto("src address invalid", "%s" % qq(src))

        try:
            adst = Addr.parse(network, dst)
        except ValueError:
            eproto("dst address invalid", "%s" % qq(dst))

        with errctx("%s <- %s" % (dst, src)):
            try:
                accept = n._vnet_accept(asrc, adst, osconn)
            except Exception as e:
                _, _, tb = sys.exc_info()
                ereply(e, tb)

            try:
                reply('connected %s' % qq(accept.addr))
            except Exception as e:
                accept.ack.send(e)
                raise
            else:
                accept.ack.send(None)


    # _loconnect tries to establish lonet connection on top of OS-level connection.
    def _loconnect(n, osconn, src, dst):
        # XXX does not support interruption
        try:
            return n.__loconnect(osconn, src, dst)
        except Exception as err:
            peeraddr = addrstr4(*osconn.getpeername())

            # close osconn on error
            osconn.close()

            _, _, tb = sys.exc_info()
            if err is not ErrConnRefused:
                err = Error("loconnect %s" % peeraddr, err, tb)
            raise err


    def __loconnect(n, osconn, src, dst):
        osconn.sendall("> lonet %s dial %s %s\n" % (qq(n._network), qq(src), qq(dst)))
        line = skreadline(osconn, 1024)
        m = _loreply_re.match(line)
        if m is None:
            raise protocolError("invalid dial reply: %s" % qq(line))

        network = m.group('network').decode('string_escape')
        reply   = m.group('reply') # no unescape
        arg     = m.group('arg').decode('string_escape')

        if reply == "E":
            if arg == "connection refused":
                raise ErrConnRefused
            else:
                raise Error(arg)

        if reply == "connected":
            pass    # ok
        else:
            raise protocolError("invalid reply verb: %s" % qq(reply))

        if network != n._network:
            raise protocolError("connected, but network mismatch: %s" % qq(network))

        try:
            acceptAddr = Addr.parse(network, arg)
        except ValueError:
            raise protocolError("connected, but accept address invalid: %s" % qq(acceptAddr))

        if acceptAddr.host != dst.host:
            raise protocolError("connected, but accept address is for different host: %s" % qq(acceptAddr.host))

        # everything is ok
        return acceptAddr


    def _vnet_dial(n, src, dst, dstosladdr):
        try:
            # XXX abusing Addr.parse to parse TCP address
            a = Addr.parse("", dstosladdr)
        except ValueError:
            raise ValueError('%s is not valid TCP address' % dstosladdr)

        osconn = net.socket(net.AF_INET, net.SOCK_STREAM)
        osconn.connect((a.host, a.port))
        addrAccept = n._loconnect(osconn, src, dst)
        return osconn, addrAccept

    def _vnet_newhost(n, hostname, registry):
        registry.announce(hostname, '%s:%d' % n._oslistener.getsockname())


@func(protocolError)
def __str__(e):
    return "protocol error: %s" % e.args


# skreadline reads 1 line from sk up to maxlen bytes.
def skreadline(sk, maxlen):
    line = ""
    while len(line) < maxlen:
        b = sk.recv(1)
        if len(b) == 0: # EOF
            raise Error('unexpected EOF')
        assert len(b) == 1
        line += b
        if b == "\n":
            break

    return line



# -------- registry --------
#
# See registry_sqlite.go for details.


# RegistryError is the error raised by registry operations.
class RegistryError(Exception):
    def __init__(self, err, registry, op, *argv):
        self.err, self.registry, self.op, self.argv = err, registry, op, argv

    def __str__(self):
        return "%s: %s %s: %s" % (self.registry, self.op, self.argv, self.err)

# _regerr wraps f to raise RegistryError exception.
def _regerr(f):
    @functools.wraps(f)
    def f_regerr(self, *argv):
        try:
            return f(self, *argv)
        except Exception as err:
            if not xerr.well_defined(err):
                _, _, tb = sys.exc_info()
                err = Error("BUG", err, tb)
            raise RegistryError(err, self.uri, f.__name__, *argv)

    return f_regerr


# DBPool provides pool of SQLite connections.
class DBPool(object):

    def __init__(self, dburi):
        # factory to create new connection.
        #
        # ( !check_same_thread because it is safe from long ago to pass SQLite
        #   connections in between threads, and with using pool it can happen. )
        def factory():
            conn = sqlite3.connect(dburi, check_same_thread=False)
            conn.text_factory    = str  # always return bytestrings - we keep text in UTF-8
            conn.isolation_level = None # autocommit
            return conn

        self._factory = factory             # None when pool closed
        self._lock    = threading.Lock()
        self._connv   = []                  # of sqlite3.connection

    # get gets connection from the pool.
    #
    # once user is done with it, it has to put the connection back via put.
    def get(self):
        # try getting already available connection
        with self._lock:
            factory = self._factory
            if factory is None:
                raise RuntimeError("sqlite: pool: get on closed pool")
            if len(self._connv) > 0:
                conn = self._connv.pop()
                return conn

        # no connection available - open new one
        return factory()


    # put puts connection back into the pool.
    def put(self, conn):
        with self._lock:
            if self._factory is not None:
                self._connv.append(conn)
                return

        # conn is put back after pool was closed - close conn.
        conn.close()

    # close closes the pool.
    def close(self):
        with self._lock:
            self._factory = None
            connv = self._connv
            self._connv = []

        for conn in connv:
            conn.close()


    # with xget one can use DBPool as context manager to automatically get / put a connection.
    def xget(self):
        return _DBPoolContext(self)

class _DBPoolContext(object):

    def __init__(self, pool):
        self.pool = pool
        self.conn = pool.get()

    def __enter__(self):
        return self.conn

    def __exit__(self, exc_type, exc_val, exc_tb):
        self.pool.put(self.conn)


# SQLiteRegistry implements network registry as shared SQLite file.
class SQLiteRegistry(object):

    schema_ver = "lonet.1"

    @_regerr
    def __init__(r, dburi, network):
        r.uri = dburi
        r._dbpool = DBPool(dburi)
        r._setup(network)

    def close(r):
        r._dbpool.close()

    def _setup(r, network):
        with errctx('setup %s' % qq(network)):
            with r._dbpool.xget() as conn:
                with conn:
                    conn.execute("""
                        CREATE TABLE IF NOT EXISTS hosts (
                                hostname	TEXT NON NULL PRIMARY KEY,
                                osladdr		TEXT NON NULL
                        )
                    """)

                    conn.execute("""
                    CREATE TABLE IF NOT EXISTS meta (
                                name		TEXT NON NULL PRIMARY KEY,
                                value		TEXT NON NULL
                        )
                    """)

                    ver = r._config(conn, "schemaver")
                    if ver == "":
                        ver = r.schema_ver
                        r._set_config(conn, "schemaver", ver)
                    if ver != r.schema_ver:
                        raise Error('schema version mismatch: want %s; have %s' % (qq(r._schema_ver), qq(ver)))

                    dbnetwork = r._config(conn, "network")
                    if dbnetwork == "":
                        dbnetwork = network
                        r._set_config(conn, "network", dbnetwork)
                    if dbnetwork != network:
                        raise Error('network name mismatch: want %s; have %s' % (qq(network), qq(dbnetwork)))


    def _config(r, conn, name):
        with errctx('config: get %s' % qq(name)):
            rowv = query(conn, "SELECT value FROM meta WHERE name = ?", name)
            if len(rowv) == 0:
                return ""
            if len(rowv) > 1:
                raise Error("registry broken: duplicate config entries")
            return rowv[0][0]


    def _set_config(r, conn, name, value):
        with errctx('config: set %s = %s' % (qq(name), qq(value))):
            conn.execute(
                    "INSERT OR REPLACE INTO meta (name, value) VALUES (?, ?)",
                    (name, value))


    @_regerr
    def announce(r, hostname, osladdr):
        with r._dbpool.xget() as conn:
            try:
                conn.execute(
                    "INSERT INTO hosts (hostname, osladdr) VALUES (?, ?)",
                    (hostname, osladdr))
            except sqlite3.IntegrityError as e:
                if e.message.startswith('UNIQUE constraint failed'):
                    raise ErrHostDup
                raise

    @_regerr
    def query(r, hostname):
        with r._dbpool.xget() as conn:
            rowv = query(conn, "SELECT osladdr FROM hosts WHERE hostname = ?", hostname)
            if len(rowv) == 0:
                return None
            if len(rowv) > 1:
                raise Error("registry broken: duplicate host entries")
            return rowv[0][0]


# query executes query on connection, fetches and returns all rows as [].
def query(conn, sql, *argv):
    rowi = conn.execute(sql, argv)
    return list(rowi)
