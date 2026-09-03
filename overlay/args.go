// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sys

// ExecArgLengthLimit is the number of bytes we can safely
// pass as arguments to an exec.Command.
//
// Windows has a limit of 32 KB. To be conservative and not worry about whether
// that includes spaces or not, just use 30 KB. Darwin's limit is less clear.
// The OS claims 256KB, but we've seen failures with arglen as small as 50KB.
// bottle overlay: wasm_exec packs argv strings, env strings, and the
// pointer array into one 8KB window below the linker's data start, and
// throws past it. That budget is shared, so bounding argv alone at 8KB
// still overflows once env is added. Force a response file on any
// non-trivial argv (the tools all understand @file), leaving only env +
// a tiny "compile @/tmp/args" to fit.
const ExecArgLengthLimit = (2 << 10)
