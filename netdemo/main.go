// netdemo builds against an external module (github.com/pkg/errors) fetched
// over the /goproxy passthrough. It is the fixture probe-gonet.html builds in
// the tab, and the source stdsrc.sh reads to size the std closure so the
// dependency's standard-library imports are seeded.
package main

import (
	"fmt"

	"github.com/pkg/errors"
)

func main() {
	err := errors.Wrap(errors.New("root cause"), "wrapped")
	fmt.Printf("%v\n", err)
}
