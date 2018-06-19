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

# pytest setup so that Go side could pass fixture parameters to Python side.

import pytest


# --network: for go side to pass network name to join to py side.
# --registry-dbpath: for go side to pass registry db location to py side.

def pytest_addoption(parser):
    parser.addoption("--network", action="store")
    parser.addoption("--registry-dbpath", action="store")

@pytest.fixture
def network(request):
    network = request.config.getoption("--network")
    if network is None:
        raise RuntimeError("--network not set")
    return network

@pytest.fixture
def registry_dbpath(request):
    dbpath = request.config.getoption("--registry-dbpath")
    if dbpath is None:
        raise RuntimeError("--registry-dbpath not set")
    return dbpath
