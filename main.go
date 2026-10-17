//go:build linux

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

func usage() {
	fmt.Fprintln(os.Stderr, "usage: containerR <run|pull|images|delete> ...")
}

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}

	db, err := SharedDatabase().Open()
	if err != nil {
		fmt.Fprintln(os.Stderr, "database:", err)
		os.Exit(1)
	}
	defer db.Close()

	switch os.Args[1] {
	case "container-init":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "container-init requires a container ID")
			os.Exit(1)
		}
		if err := initContainer(os.Args[2], os.Args[3:]); err != nil {
			fmt.Fprintln(os.Stderr, "container-init:", err)
			os.Exit(1)
		}
	case "run":
		if len(os.Args) < 3 {
			usage()
			return
		}
		container, err := createContainer(os.Args[2])
		if err != nil {
			fmt.Println("run:", err)
			return
		}
		if err := runContainter(container, os.Args[3:]); err != nil {
			fmt.Println("run:", err)
		}
	case "pull":
		if len(os.Args) < 3 {
			usage()
			return
		}
		if err := pull(os.Args[2]); err != nil {
			fmt.Println("pull:", err)
		}
	case "images":
		if len(os.Args) != 2 {
			usage()
			return
		}
		if err := getImages(); err != nil {
			fmt.Println("images:", err)
		}
	case "image delete":
		if len(os.Args) < 3 {
			usage()
			return
		}
		if err := deleteImage(os.Args[2]); err != nil {
			fmt.Println("delete:", err)
		}
	default:
		usage()
	}
}
