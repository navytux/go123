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

// Package provides addons to standard package strings
package xstrings

import (
    "fmt"
    "strings"
)

// split string into lines. The last line, if it is empty, is omitted from the result
// (rationale is: string.Split("hello\nworld\n", "\n") -> ["hello", "world", ""])
func SplitLines(s, sep string) []string {
    sv := strings.Split(s, sep)
    l := len(sv)
    if l > 0 && sv[l-1] == "" {
        sv = sv[:l-1]
    }
    return sv
}

// split string by sep and expect exactly 2 parts
func Split2(s, sep string) (s1, s2 string, err error) {
    parts := strings.Split(s, sep)
    if len(parts) != 2 {
        return "", "", fmt.Errorf("split2: %q has %v parts (expected 2, sep: %q)", s, len(parts), sep)
    }
    return parts[0], parts[1], nil
}

// (head+sep+tail) -> head, tail
func HeadTail(s, sep string) (head, tail string, err error) {
    parts := strings.SplitN(s, sep, 2)
    if len(parts) != 2 {
        return "", "", fmt.Errorf("headtail: %q has no %q", s, sep)
    }
    return parts[0], parts[1], nil
}
