//go:build darwin

package main

import (
	"fmt"
	"os"
)

const (
	containerRoot = "/lib/containerR"
	imagesDir     = containerRoot + "/images"
	containersDir = containerRoot + "/containers"
)

func main() {
	fmt.Fprintln(os.Stderr, "containerR on macOS is not configured yet. use lima VM")
	os.Exit(1)
}
