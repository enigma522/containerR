//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"
)

const (
	containerRoot = "/lib/containerR"
	imagesDir     = containerRoot + "/images"
	containersDir = containerRoot + "/containers"
)

func initContainer(rootfs string, call string) error {
	// Keep namespace-sensitive setup and exec on the same OS thread.
	runtime.LockOSThread()

	if err := syscall.Sethostname([]byte("container")); err != nil {
		fmt.Errorf("%w", err)
	}

	if err := syscall.Chroot(rootfs); err != nil {
		return fmt.Errorf("chroot: %w", err)
	}
	if err := os.Chdir("/"); err != nil {
		return err
	}
	if err := os.MkdirAll("/proc", 0555); err != nil {
		return err
	}

	if err := syscall.Mount("proc", "/proc", "proc", syscall.MS_NOSUID|syscall.MS_NODEV|syscall.MS_NOEXEC, ""); err != nil {
		return fmt.Errorf("mount /proc: %w", err)
	}

	// TODO: use the env vars that exist in the image config
	if err := os.Setenv("PATH", "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"); err != nil {
		return err
	}
	path, err := exec.LookPath(call)
	if err != nil {
		return err
	}

	//  replaces the running container-init program with call
	return syscall.Exec(path, []string{call}, os.Environ())
}

func runContainter(rootfs string, call string) error {
	// Re-exec this launcher before chroot, so it need not exist in the image.
	cmd := exec.Command("/proc/self/exe", "container-init", rootfs, call)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// CLONE_NEWUTS: creates a new UTS (Unix Time-Sharing) namespace This isolates the hostname and domain name

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS,
		// Go also makes inherited mounts recursively private for CLONE_NEWNS.
		Unshareflags: syscall.CLONE_NEWNS,
	}

	return cmd.Run()
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: containerR <run|pull|images|delete> ...")
}

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}

	switch os.Args[1] {
	case "container-init":
		if len(os.Args) != 4 {
			fmt.Fprintln(os.Stderr, "container-init requires rootfs and command")
			os.Exit(1)
		}
		if err := initContainer(os.Args[2], os.Args[3]); err != nil {
			fmt.Fprintln(os.Stderr, "container-init:", err)
			os.Exit(1)
		}
	case "run":
		if len(os.Args) < 4 {
			usage()
			return
		}
		rootfs, err := createContainerRootfs(os.Args[2])
		if err != nil {
			fmt.Println("run:", err)
			return
		}
		if err := runContainter(rootfs, os.Args[3]); err != nil {
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
