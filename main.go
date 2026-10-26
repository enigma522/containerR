//go:build linux

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

var (
	containerRoot string
	imagesDir     string
	containersDir string
	databaseFile  string
)

func initStoragePaths() error {
	home, _ := os.UserHomeDir()
	root := filepath.Join(home, ".local", "share", "containerR")

	containerRoot = root
	imagesDir = filepath.Join(root, "images")
	containersDir = filepath.Join(root, "containers")
	databaseFile = filepath.Join(root, "containerr.db")

	return os.MkdirAll(root, 0755)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: containerR <run|start|pull|images|ps|delete|rm> ...")
}

func main() {
	if err := initStoragePaths(); err != nil {
		fmt.Fprintln(os.Stderr, "storage:", err)
		os.Exit(1)
	}

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
	case "ps":
		fs := flag.NewFlagSet("ps", flag.ContinueOnError)
		all := fs.Bool("all", false, "a bool")

		fs.Parse(os.Args[2:])

		if err := getContainers(!*all); err != nil {
			fmt.Println("images:", err)
		}
	case "start":
		if len(os.Args) != 3 {
			fmt.Fprintln(os.Stderr, "usage: containerR start <container-id>")
			return
		}
		if err := startContainer(os.Args[2]); err != nil {
			fmt.Println("start:", err)
		}
	case "delete", "rm":
		if len(os.Args) != 3 {
			fmt.Fprintln(os.Stderr, "usage: containerR delete <container-id>")
			return
		}
		if err := deleteContainer(os.Args[2]); err != nil {
			fmt.Println("delete:", err)
		}

	default:
		usage()
	}
}
