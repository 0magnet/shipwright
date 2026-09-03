// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build wasm

package exec

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotFound is the error resulting if a path search failed to find an executable file.
var ErrNotFound = errors.New("executable file not found in $PATH")

// bottle overlay: wasm CAN execute processes now (bottle's proc layer
// instantiates another wasm program from the page filesystem). So LookPath
// resolves against that filesystem instead of unconditionally failing:
// a slashed path is checked directly, a bare name is searched along $PATH,
// and "executable" means the mode bit is set (there is no Eaccess on js).
func findExecutable(file string) error {
	d, err := os.Stat(file)
	if err != nil {
		return err
	}
	if m := d.Mode(); m.IsDir() {
		return fs.ErrPermission
	} else if m&0111 == 0 {
		return fs.ErrPermission
	}
	return nil
}

func lookPath(file string) (string, error) {
	if strings.ContainsRune(file, '/') {
		err := findExecutable(file)
		if err == nil {
			return file, nil
		}
		return "", &Error{file, err}
	}
	path := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			dir = "."
		}
		p := filepath.Join(dir, file)
		if err := findExecutable(p); err == nil {
			return p, nil
		}
	}
	return "", &Error{file, ErrNotFound}
}

// lookExtensions is a no-op on non-Windows platforms, since
// they do not restrict executables to specific extensions.
func lookExtensions(path, dir string) (string, error) {
	return path, nil
}
