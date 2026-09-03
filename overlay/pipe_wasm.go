// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build wasm

package os

import "syscall"

// Pipe returns a connected pair of Files; reads from r return bytes written
// to w.
//
// bottle overlay: js/wasm has no OS pipes, but the syscall_js overlay provides
// in-Go pipes (a shared byte buffer with a read-end and a write-end fd). Wrap
// those fds as *File so os/exec and cmd/go build child stdio out of them.
func Pipe() (r *File, w *File, err error) {
	var p [2]int
	if e := syscall.Pipe(p[:]); e != nil {
		return nil, nil, NewSyscallError("pipe", e)
	}
	return NewFile(uintptr(p[0]), "|0"), NewFile(uintptr(p[1]), "|1"), nil
}
