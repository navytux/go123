# -*- coding: utf-8 -*-
# Copyright (C) 2018  Nexedi SA and Contributors.
#                     Kirill Smelkov <kirr@nexedi.com>
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

import gopath
xerr  = gopath.gimport('lab.nexedi.com/kirr/go123/xerr')
lonet = gopath.gimport('lab.nexedi.com/kirr/go123/xnet/lonet')

from threading import Thread
from cStringIO import StringIO
import errno, logging as log


def xread(sk):
    # XXX might get only part of sent data
    return sk.recv(4096)

def xwrite(sk, data):
    sk.sendall(data)


# TODO test that fd of listener can be used in select/epoll
# TODO non-blocking mode


# _test_virtnet_basic runs basic tests on a virtnet network implementation.
# (this follows virtnettest.TestBasic)
def _test_virtnet_basic(subnet):
    # (verifying that error log stays empty)
    errorlog  = StringIO()
    errorlogh = log.StreamHandler(errorlog)
    l = log.getLogger()
    l.addHandler(errorlogh)

    try:
        __test_virtnet_basic(subnet)
    finally:
        subnet.close()

        l.removeHandler(errorlogh)
        assert errorlog.getvalue() == ""

def __test_virtnet_basic(subnet):
    def xaddr(addr):
        return lonet.Addr.parse(subnet.network(), addr)

    ha = subnet.new_host("α")
    hb = subnet.new_host("β")

    assert ha.network() == subnet.network()
    assert hb.network() == subnet.network()
    assert ha.name() == "α"
    assert hb.name() == "β"

    try:
        ha.dial(":0")
    except Exception as e:
        assert xerr.cause(e) is lonet.ErrConnRefused
        assert str(e) == "dial %s α:1->α:0: [Errno %d] connection refused" % (subnet.network(), errno.ECONNREFUSED)
    else:
        assert 0, "connection not refused"

    l1 = ha.listen("")
    assert l1.addr() == xaddr("α:1")

    try:
        ha.dial(":0")
    except Exception as e:
        assert xerr.cause(e) is lonet.ErrConnRefused
        assert str(e) == "dial %s α:2->α:0: [Errno %d] connection refused" % (subnet.network(), errno.ECONNREFUSED)
    else:
        assert 0, "connection not refused"


    def Tsrv():
        c1s = l1.accept()
        assert c1s.local_addr()  == xaddr("α:2")
        assert c1s.getsockname() == ("α", 2)
        assert c1s.remote_addr() == xaddr("β:1")
        assert c1s.getpeername() == ("β", 1)

        assert xread(c1s) == "ping"
        xwrite(c1s, "pong")

        c2s = l1.accept()
        assert c2s.local_addr()  == xaddr("α:3")
        assert c2s.getsockname() == ("α", 3)
        assert c2s.remote_addr() == xaddr("β:2")
        assert c2s.getpeername() == ("β", 2)

        assert xread(c2s) == "hello"
        xwrite(c2s, "world")


    tsrv = Thread(target=Tsrv)
    tsrv.start()

    c1c = hb.dial("α:1")
    assert c1c.local_addr()  == xaddr("β:1")
    assert c1c.getsockname() == ("β", 1)
    assert c1c.remote_addr() == xaddr("α:2")
    assert c1c.getpeername() == ("α", 2)

    xwrite(c1c, "ping")
    assert xread(c1c) == "pong"

    c2c = hb.dial("α:1")
    assert c2c.local_addr()  == xaddr("β:2")
    assert c2c.getsockname() == ("β", 2)
    assert c2c.remote_addr() == xaddr("α:3")
    assert c2c.getpeername() == ("α", 3)

    xwrite(c2c, "hello")
    assert xread(c2c) == "world"

    tsrv.join()

    l2 = ha.listen(":0")
    assert l2.addr() == xaddr("α:4")

    subnet.close()


def test_lonet_py_basic():
    subnet = lonet.join("")
    _test_virtnet_basic(subnet)


# test interaction with lonet.go
def test_lonet_py_go(network):
    subnet = lonet.join(network)
    try:
        _test_lonet_py_go(subnet)
    finally:
        subnet.close()

def _test_lonet_py_go(subnet):
    def xaddr(addr):
        return lonet.Addr.parse(subnet.network(), addr)

    hb = subnet.new_host("β")
    lb = hb.listen(":1")

    c1 = hb.dial("α:1")
    assert c1.local_addr() == xaddr("β:2")
    assert c1.remote_addr() == xaddr("α:2")
    assert xread(c1) == "hello py"
    xwrite(c1, "hello go")
    c1.close()

    c2 = lb.accept()
    assert c2.local_addr() == xaddr("β:2")
    assert c2.remote_addr() == xaddr("α:2")
    xwrite(c2, "hello2 go")
    assert xread(c2) == "hello2 py"
    c2.close()



# go created a registry. verify we can read values from it and write something back too.
# go side will check what we wrote.
def test_registry_pygo(registry_dbpath):
    try:
        lonet.SQLiteRegistry(registry_dbpath, "ddd")
    except lonet.RegistryError as e:
        assert 'network name mismatch: want "ddd"; have "ccc"' in str(e)
    else:
        assert 0, 'network name mismatch not detected'

    r = lonet.SQLiteRegistry(registry_dbpath, "ccc")
    assert r.query("α") == "alpha:1234"
    assert r.query("β") is None
    r.announce("β", "beta:py")
    assert r.query("β") == "beta:py"

    try:
        r.announce("β", "beta:py2")
    except lonet.RegistryError as e:
        # XXX py escapes utf-8 with \
        #assert "announce ('β', 'beta:py2'): host already registered" in str(e)
        assert ": host already registered" in str(e)
    else:
        assert 0, 'duplicate host announce not detected'

    # ok - hand over checks back to go side.
