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
"""Package xerr provides addons for error-handling.

Context is handy to concisely add context to raised error, for example:

    def myfunc(arg1, arg2):
        with xerr.context("doing something %s" % name):
            do1()
            do2()
            ...

will wrap an exception that inner code block raises (if any) with

    Error("doing something %s" % name, ...)

The original unwrapped error will be still accessible as the cause of returned
error with

    xerr.cause(err)

This package is modelled after equally-named Go package.
"""

import traceback

# Error is exception that represents an error.
#
# It has error string/object (.err), and optionally cause exception that
# caused this error.
#
# It is similar to `raise ... from ...` in python3.
class Error(Exception):
    def __init__(self, err, cause_exc=None, cause_tb=None):
        self.err, self.cause_exc, self.cause_tb = err, cause_exc, cause_tb

    # __str__ returns string representation of the error.
    #
    # besides main error line, it also includes cause traceback, if cause is
    # not "well-defined-error".
    def __str__(self):
        errv = []   # of .err + .cause_exc in the end
        e = self
        while isinstance(e, Error):
            errv.append(e.err)
            tb = e.cause_tb
            e  = e.cause_exc

        # !Error cause
        if e is not None:
            errv.append(e)

        s = ": ".join(["%s" % _ for _ in errv])

        # cause traceback
        if tb is not None:
            if not well_defined(e):
                s += "\n\ncause traceback:\n%s" % ('\n'.join(traceback.format_tb(tb)),)

        return s

    def __repr__(self):
        return "Error(%r, %r, %r)" % (self.err, self.cause_exc, self.cause_tb)


# well_defined returns whether err is well-defined error or not.
#
# well-defined-errors are those that carry on all information and do not need
# traceback to interpret them if they are wrapped with another Error properly.
_wde_classv  = ()
_wde_objectv = ()
def well_defined(err):
    if isinstance(err, _wde_classv):
        return True

    for e in _wde_objectv:
        if e is err:
            return True

    return False

# register_wde_* let Error know that an error type or object is well-defined.
def register_wde_class(exc_class):
    global _wde_classv
    _wde_classv += (exc_class,)

def register_wde_object(exc_object):
    global _wde_objectv
    _wde_objectv += (exc_object,)

register_wde_class(Error)


# cause returns deepest !None cause of an error.
def cause(err):
    while isinstance(err, Error):
        if err.cause_exc is None:
            return err
        err = err.cause_exc

    return err

# context is context manager to wrap code block with an error context.
#
# for example
#
#   with xerr.context("doing something %s" % name):
#       do1()
#       do2()
#       ...
#
# will wrap an exception that inner code block raises with Error.
class context(object):

    def __init__(self, errprefix):
        self.errprefix = errprefix

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        if exc_val is not None:
            raise Error(self.errprefix, exc_val, exc_tb)
