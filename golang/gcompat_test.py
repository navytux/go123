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
gcompat = gopath.gimport('lab.nexedi.com/kirr/go123/golang/gcompat')
qq  = gcompat.qq

def test_qq():
    testv = (
        # in            want without leading/trailing "
        ('',            r""),
        ('\'',          r"'"),
        ('"',           r"\""),
        ('abc\ndef',    r"abc\ndef"),
        ('a\'c\ndef',   r"a'c\ndef"),
        ('a\"c\ndef',   r"a\"c\ndef"),
        # ('привет',      r"привет"),       TODO
    )

    for tin, twant in testv:
        twant = '"' + twant + '"'   # add lead/trail "
        assert qq(tin) == twant
