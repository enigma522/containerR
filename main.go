package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"
)

const (
	containerRoot = "/lib/containerR"
	imagesDir     = containerRoot + "/images"
	imagesIndex   = containerRoot + "/images.json"
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

// docker export $(docker create image) | tar -C rootfs -xvf -
func pull(image string, imageRepo *Repository[imageMetadata]) error {

	_, err := imageRepo.GetByName(image)

	if err == nil {
		return errors.New("Image already exists")
	}

	image_metadata := imageMetadata{}

	create := exec.Command("docker", "create", image)

	output, err := create.Output()
	if err != nil {
		return err
	}

	containerID := strings.TrimSpace(string(output))
	sum := sha256.Sum256([]byte(image))

	image_metadata.ID = hex.EncodeToString(sum[:])
	image_metadata.Name = image
	image_metadata.Path = imagesDir + "/" + containerID + "/rootfs"
	image_metadata.CreatedAt = time.Now()

	if err := os.MkdirAll(image_metadata.Path, 0755); err != nil {
		return err
	}

	export := exec.Command("docker", "export", containerID)

	tar := exec.Command("tar", "-C", image_metadata.Path, "-xvf", "-")

	tar.Stdin, err = export.StdoutPipe()

	if err != nil {
		return err
	}

	tar.Stderr = os.Stderr
	export.Stderr = os.Stderr

	if err := export.Start(); err != nil {
		return err
	}

	if err := tar.Run(); err != nil {
		return err
	}

	if err := export.Wait(); err != nil {
		return err
	}

	return imageRepo.Save(image_metadata)
}

func deleteImage(imageName string, imageRepo *Repository[imageMetadata]) error {
	image, err := imageRepo.GetByName(imageName)
	if err != nil {
		return err
	}

	imagePath := filepath.Clean(image.Path)
	if filepath.Base(imagePath) != "rootfs" {
		return fmt.Errorf("invalid image path: %q", image.Path)
	}

	imageDir := filepath.Dir(imagePath)
	relativePath, err := filepath.Rel(imagesDir, imageDir)
	if err != nil || relativePath == "." || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return fmt.Errorf("image path is outside the images directory: %q", image.Path)
	}

	if err := os.RemoveAll(imageDir); err != nil {
		return err
	}

	_, err = imageRepo.DeleteByName(imageName)
	return err
}

func getImages(imageRepo *Repository[imageMetadata]) error {
	images, err := imageRepo.All()
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tIMAGE ID\tCREATED")
	for _, image := range images {
		fmt.Fprintf(w, "%s\t%s\t%s\n", image.Name, image.ID, image.CreatedAt.Format(time.RFC3339))
	}
	return w.Flush()
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: containerR <run|pull|images|delete> ...")
}

func main() {
	imageRepo := NewRepository[imageMetadata](imagesIndex)

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
		image, err := imageRepo.GetByName(os.Args[2])
		if err != nil {
			fmt.Println("run:", err)
			return
		}
		if err := runContainter(image.Path, os.Args[3]); err != nil {
			fmt.Println("run:", err)
		}
	case "pull":
		if len(os.Args) < 3 {
			usage()
			return
		}
		if err := pull(os.Args[2], imageRepo); err != nil {
			fmt.Println("pull:", err)
		}
	case "images":
		if len(os.Args) != 2 {
			usage()
			return
		}
		if err := getImages(imageRepo); err != nil {
			fmt.Println("images:", err)
		}
	case "image delete":
		if len(os.Args) < 3 {
			usage()
			return
		}
		if err := deleteImage(os.Args[2], imageRepo); err != nil {
			fmt.Println("delete:", err)
		}
	default:
		usage()
	}
}
