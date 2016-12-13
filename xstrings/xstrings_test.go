// Copyright (C) 2015-2016  Nexedi SA and Contributors.
//                          Kirill Smelkov <kirr@nexedi.com>
//
// This program is free software: you can Use, Study, Modify and Redistribute
// it under the terms of the GNU General Public License version 3, or (at your
// option) any later version, as published by the Free Software Foundation.
//
// This program is distributed WITHOUT ANY WARRANTY; without even the implied
// warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
//
// See COPYING file for full licensing terms.

package xstrings

import (
    "reflect"
    "testing"
)

func TestSplitLines(t *testing.T) {
    var tests = []struct { input, sep string; output []string } {
        {"",                    "\n",   []string{}},
        {"hello",               "\n",   []string{"hello"}},
        {"hello\n",             "\n",   []string{"hello"}},
        {"hello\nworld",        "\n",   []string{"hello", "world"}},
        {"hello\nworld\n",      "\n",   []string{"hello", "world"}},
        {"hello\x00world\x00",  "\n",   []string{"hello\x00world\x00"}},
        {"hello\x00world\x00",  "\x00", []string{"hello", "world"}},
    }

    for _, tt := range tests {
        sv := SplitLines(tt.input, tt.sep)
        if !reflect.DeepEqual(sv, tt.output) {
            t.Errorf("splitlines(%q, %q) -> %q  ; want %q", tt.input, tt.sep, sv, tt.output)
        }
    }
}

func TestSplit2(t *testing.T) {
    var tests = []struct { input, s1, s2 string; ok bool } {
        {"", "", "", false},
        {" ", "", "", true},
        {"hello", "", "", false},
        {"hello world", "hello", "world", true},
        {"hello world 1", "", "", false},
    }

    for _, tt := range tests {
        s1, s2, err := Split2(tt.input, " ")
        ok := err == nil
        if s1 != tt.s1 || s2 != tt.s2 || ok != tt.ok {
            t.Errorf("split2(%q) -> %q %q %v  ; want %q %q %v", tt.input, s1, s2, ok, tt.s1, tt.s2, tt.ok)
        }
    }
}

func TestHeadtail(t *testing.T) {
    var tests = []struct { input, head, tail string; ok bool } {
        {"",                "", "", false},
        {" ",               "", "", true},
        {"  ",              "", " ", true},
        {"hello world",     "hello", "world", true},
        {"hello world 1",   "hello", "world 1", true},
        {"hello  world 2",  "hello", " world 2", true},
    }

    for _, tt := range tests {
        head, tail, err := HeadTail(tt.input, " ")
        ok := err == nil
        if head != tt.head || tail != tt.tail || ok != tt.ok {
            t.Errorf("headtail(%q) -> %q %q %v  ; want %q %q %v", tt.input, head, tail, ok, tt.head, tt.tail, tt.ok)
        }
    }
}
