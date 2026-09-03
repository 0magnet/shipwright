package main

import (
	"fmt"
	"os"
	"runtime"
)

func main() {
	fmt.Printf("GOTAB-MARKER: hello from a Go program compiled and linked inside a browser tab (%s/%s, %s)\n", runtime.GOOS, runtime.GOARCH, runtime.Version())
	host, _ := os.Hostname()
	fmt.Println("GOTAB-HOST:", host)
}
