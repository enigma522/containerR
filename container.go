//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
)

func getContainers(running bool) error {
	containers, err := NewContainerRepository().All()

	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "CONTAINER ID\tIMAGE\tCOMMAND\tSTATUS\tPORTS\tNAMES")
	for _, c := range containers {
		if !running || c.Status == "running" {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", c.ID, c.ImageRef, c.Entrypoint, c.Status, "<still>", c.Name)
		}
	}
	return w.Flush()
}

func startContainer(id string) error {
	container, err := NewContainerRepository().GetByID(id)
	if err != nil {
		return err
	}
	if container.Status == "running" {
		return fmt.Errorf("container %q is already running", id)
	}
	return runContainter(&container, nil)
}

func deleteContainer(id string) error {
	repository := NewContainerRepository()
	container, err := repository.GetByID(id)
	if err != nil {
		return err
	}
	if container.Status == "running" {
		return fmt.Errorf("container %q is running; stop it before deleting it", id)
	}

	if id == "" || filepath.Base(id) != id {
		return fmt.Errorf("invalid container ID %q", id)
	}
	containerDir := filepath.Join(containersDir, id)
	expectedRootfs := filepath.Join(containerDir, "rootfs")
	if filepath.Clean(container.Rootfs) != expectedRootfs {
		return fmt.Errorf("container %q has an unexpected rootfs path %q", id, container.Rootfs)
	}

	if err := os.RemoveAll(containerDir); err != nil {
		return fmt.Errorf("remove container filesystem: %w", err)
	}
	if err := repository.DeleteByID(id); err != nil {
		return err
	}
	return nil
}

