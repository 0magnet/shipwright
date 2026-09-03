// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !unix && !windows

package filelock

type lockType int8

const (
	readLock = iota + 1
	writeLock
)

func lock(f File, lt lockType) error {
	// bottle overlay: a wasm tab runs one cmd/go on one thread, so file locks
	// have no one to exclude — advisory locking is a no-op here rather than
	// the unsupported error that made lockedfile fail.
	return nil
}

func unlock(f File) error {
	return nil
}
