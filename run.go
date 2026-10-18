//go:build linux

package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)


func createContainer(imageRef string) (*ContainerRecord, error) {
	_, image, err := imageForName(imageRef)

	if err != nil {
		return nil, err
	}

	config, err := image.ConfigFile()

	if err != nil {
		return nil, err
	}

	id, err := newContainerID()
	if err != nil {
		return nil, err
	}
	rootfs, err := createContainerRootfs(id, image)

	if err != nil {
		return nil, err
	}

	digest, err := image.Digest()
	if err != nil {
		_ = os.RemoveAll(filepath.Dir(rootfs))
		return nil, fmt.Errorf("get image digest: %w", err)
	}
	cmd, err := json.Marshal(config.Config.Cmd)
	if err != nil {
		_ = os.RemoveAll(filepath.Dir(rootfs))
		return nil, fmt.Errorf("encode image command: %w", err)
	}
	entrypoint, err := json.Marshal(config.Config.Entrypoint)
	if err != nil {
		_ = os.RemoveAll(filepath.Dir(rootfs))
		return nil, fmt.Errorf("encode image entrypoint: %w", err)
	}
	env, err := json.Marshal(config.Config.Env)
	if err != nil {
		_ = os.RemoveAll(filepath.Dir(rootfs))
		return nil, fmt.Errorf("encode image environment: %w", err)
	}
	record := ContainerRecord{
		ID: id, Name: id, ImageRef: imageRef, ImageDigest: digest.String(), Rootfs: rootfs,
		Cmd: string(cmd), Entrypoint: string(entrypoint), Env: string(env),
		WorkingDir: config.Config.WorkingDir, Hostname: config.Config.Hostname, User: config.Config.User, Status: "created",
	}
	if err := NewContainerRepository().Save(record); err != nil {
		_ = os.RemoveAll(filepath.Dir(rootfs))
		return nil, err
	}
	return &record, nil

}

func newContainerID() (string, error) {
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return "", fmt.Errorf("generate container ID: %w", err)
	}
	return hex.EncodeToString(idBytes), nil
}

func createContainerRootfs(id string, image v1.Image) (string, error) {
	rootfs := filepath.Join(containersDir, id, "rootfs")
	if err := extractRootfs(image, rootfs); err != nil {
		return "", err
	}
	return rootfs, nil
}

func extractRootfs(image v1.Image, rootfs string) error {
	if err := os.MkdirAll(filepath.Dir(rootfs), 0755); err != nil {
		return err
	}
	if err := os.Mkdir(rootfs, 0755); err != nil {
		return fmt.Errorf("create rootfs: %w", err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(rootfs)
		}
	}()

	stream := mutate.Extract(image)
	defer stream.Close()
	cmd := exec.Command("tar", "--extract", "--file=-", "--directory="+rootfs,
		"--preserve-permissions", "--numeric-owner", "--delay-directory-restore")
	cmd.Stdin = stream
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("extract rootfs: %w", err)
	}
	complete = true
	return nil
}

func initContainer(containerID string, commandOverride []string) error {
	container, err := NewContainerRepository().GetByID(containerID)
	if err != nil {
		return fmt.Errorf("load container %q: %w", containerID, err)
	}
	command, err := containerCommand(container, commandOverride)
	if err != nil {
		return err
	}
	if err := setContainerEnvironment(container.Env); err != nil {
		return err
	}

	hostname := container.Hostname
	if hostname == "" {
		hostname = container.ID
	}

	// Keep namespace-sensitive setup and exec on the same OS thread.
	runtime.LockOSThread()

	if err := syscall.Sethostname([]byte(hostname)); err != nil {
		return fmt.Errorf("set hostname: %w", err)
	}

	if err := syscall.Chroot(container.Rootfs); err != nil {
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

	workingDir := container.WorkingDir
	if workingDir == "" {
		workingDir = "/"
	}
	if err := os.Chdir(workingDir); err != nil {
		return fmt.Errorf("change to working directory %q: %w", workingDir, err)
	}
	if err := applyContainerUser(container.User); err != nil {
		return err
	}

	path, err := exec.LookPath(command[0])
	if err != nil {
		return err
	}

	// Replaces the running container-init program with the configured command.
	return syscall.Exec(path, command, os.Environ())
}

func containerCommand(container ContainerRecord, commandOverride []string) ([]string, error) {
	entrypoint, err := decodeCommand("entrypoint", container.Entrypoint)
	if err != nil {
		return nil, err
	}
	command := commandOverride
	if len(command) == 0 {
		command, err = decodeCommand("command", container.Cmd)
		if err != nil {
			return nil, err
		}
	}
	command = append(append([]string{}, entrypoint...), command...)
	if len(command) == 0 {
		return nil, fmt.Errorf("container %q has no command or entrypoint", container.ID)
	}
	return command, nil
}

func decodeCommand(field, value string) ([]string, error) {
	var command []string
	if err := json.Unmarshal([]byte(value), &command); err != nil {
		return nil, fmt.Errorf("decode container %s: %w", field, err)
	}
	return command, nil
}

func setContainerEnvironment(encoded string) error {
	var environment []string
	if err := json.Unmarshal([]byte(encoded), &environment); err != nil {
		return fmt.Errorf("decode container environment: %w", err)
	}
	os.Clearenv()
	hasPath := false
	for _, item := range environment {
		name, value, found := strings.Cut(item, "=")
		if !found || name == "" {
			return fmt.Errorf("invalid container environment entry %q", item)
		}
		if name == "PATH" {
			hasPath = true
		}
		if err := os.Setenv(name, value); err != nil {
			return fmt.Errorf("set container environment %q: %w", name, err)
		}
	}
	if !hasPath {
		return os.Setenv("PATH", "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	}
	return nil
}

// applyContainerUser supports the OCI/Docker User forms uid, uid:gid,
// username, and username:group. Name lookups occur after chroot, so they use
// the container's own /etc/passwd and /etc/group files.
func applyContainerUser(spec string) error {
	if spec == "" {
		return nil
	}
	userPart, groupPart, hasGroup := strings.Cut(spec, ":")
	if userPart == "" {
		return fmt.Errorf("invalid container user %q", spec)
	}
	uid, gid, err := lookupUser(userPart)
	if err != nil {
		return err
	}
	if hasGroup && groupPart != "" {
		gid, err = lookupGroup(groupPart)
		if err != nil {
			return err
		}
	}
	if err := syscall.Setgroups([]int{gid}); err != nil {
		return fmt.Errorf("set container groups: %w", err)
	}
	if err := syscall.Setgid(gid); err != nil {
		return fmt.Errorf("set container gid: %w", err)
	}
	if err := syscall.Setuid(uid); err != nil {
		return fmt.Errorf("set container uid: %w", err)
	}
	return nil
}

func lookupUser(value string) (int, int, error) {
	if uid, err := parseID(value); err == nil {
		return uid, 0, nil
	}
	file, err := os.Open("/etc/passwd")
	if err != nil {
		return 0, 0, fmt.Errorf("open container passwd file: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ":")
		if len(fields) >= 4 && fields[0] == value {
			uid, uidErr := parseID(fields[2])
			gid, gidErr := parseID(fields[3])
			if uidErr != nil || gidErr != nil {
				return 0, 0, fmt.Errorf("invalid container user %q", value)
			}
			return uid, gid, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, fmt.Errorf("read container passwd file: %w", err)
	}
	return 0, 0, fmt.Errorf("container user %q not found", value)
}

func lookupGroup(value string) (int, error) {
	if gid, err := parseID(value); err == nil {
		return gid, nil
	}
	file, err := os.Open("/etc/group")
	if err != nil {
		return 0, fmt.Errorf("open container group file: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ":")
		if len(fields) >= 3 && fields[0] == value {
			gid, err := parseID(fields[2])
			if err != nil {
				return 0, fmt.Errorf("invalid container group %q", value)
			}
			return gid, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("read container group file: %w", err)
	}
	return 0, fmt.Errorf("container group %q not found", value)
}

func parseID(value string) (int, error) {
	id, err := strconv.ParseUint(value, 10, 31)
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

func runContainter(container *ContainerRecord, commandOverride []string) error {
	// Re-exec this launcher before chroot, so it need not exist in the image.
	args := append([]string{"container-init", container.ID}, commandOverride...)
	cmd := exec.Command("/proc/self/exe", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// CLONE_NEWUTS: creates a new UTS (Unix Time-Sharing) namespace This isolates the hostname and domain name

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS,
		// Go also makes inherited mounts recursively private for CLONE_NEWNS.
		Unshareflags: syscall.CLONE_NEWNS,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start container: %w", err)
	}
	container.PID = cmd.Process.Pid
	container.Status = "running"
	if err := NewContainerRepository().Save(*container); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("save running container: %w", err)
	}

	err := cmd.Wait()

	container.PID = 0
	container.Status = "exited"
	if saveErr := NewContainerRepository().Save(*container); saveErr != nil && err == nil {
		return saveErr
	}
	return err
}
