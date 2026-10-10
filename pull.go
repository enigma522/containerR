package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"text/tabwriter"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/match"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

const refNameAnnotation = "org.opencontainers.image.ref.name"

// imageStore opens the one shared OCI Image Layout rooted at imagesDir.
func imageStore() (layout.Path, error) {
	store, err := layout.FromPath(imagesDir)
	if err == nil {
		return store, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("open OCI image store: %w", err)
	}
	if err := os.MkdirAll(containerRoot, 0755); err != nil {
		return "", err
	}
	store, err = layout.Write(imagesDir, empty.Index)
	if err != nil {
		return "", fmt.Errorf("create OCI image store: %w", err)
	}
	return store, nil
}

func imageForName(imageName string) (layout.Path, v1.Image, error) {
	store, err := imageStore()
	if err != nil {
		return "", nil, err
	}
	index, err := store.ImageIndex()
	if err != nil {
		return "", nil, fmt.Errorf("read OCI image index: %w", err)
	}
	manifest, err := index.IndexManifest()
	if err != nil {
		return "", nil, fmt.Errorf("read OCI image manifest: %w", err)
	}
	for _, descriptor := range manifest.Manifests {
		if descriptor.Annotations[refNameAnnotation] != imageName {
			continue
		}
		image, err := store.Image(descriptor.Digest)
		if err != nil {
			return "", nil, fmt.Errorf("read image %q: %w", imageName, err)
		}
		return store, image, nil
	}
	return store, nil, fmt.Errorf("image %q not found", imageName)
}

func pull(imageName string) error {
	store, existing, err := imageForName(imageName)
	if err == nil && existing != nil {
		return fmt.Errorf("image %q already exists", imageName)
	}
	if err != nil && store == "" {
		return err
	}

	ref, err := name.ParseReference(imageName)
	if err != nil {
		return fmt.Errorf("parse image reference: %w", err)
	}
	image, err := remote.Image(ref,
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithPlatform(v1.Platform{OS: "linux", Architecture: runtime.GOARCH}),
	)
	if err != nil {
		return fmt.Errorf("pull image: %w", err)
	}
	if err := store.AppendImage(image, layout.WithAnnotations(map[string]string{
		refNameAnnotation: imageName,
	})); err != nil {
		return fmt.Errorf("save image in OCI layout: %w", err)
	}
	return nil
}

func getImages() error {
	store, err := imageStore()
	if err != nil {
		return err
	}
	index, err := store.ImageIndex()
	if err != nil {
		return err
	}
	manifest, err := index.IndexManifest()
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tIMAGE ID")
	for _, descriptor := range manifest.Manifests {
		name := descriptor.Annotations[refNameAnnotation]
		if name == "" {
			name = "<unnamed>"
		}
		fmt.Fprintf(w, "%s\t%s\n", name, descriptor.Digest)
	}
	return w.Flush()
}

func deleteImage(imageName string) error {
	store, _, err := imageForName(imageName)
	if err != nil {
		return err
	}
	if err := store.RemoveDescriptors(match.Annotation(refNameAnnotation, imageName)); err != nil {
		return fmt.Errorf("remove image from OCI index: %w", err)
	}
	if _, err := store.GarbageCollect(); err != nil {
		return fmt.Errorf("garbage collect image blobs: %w", err)
	}
	return nil
}

func createContainerRootfs(imageName string) (string, error) {
	_, image, err := imageForName(imageName)
	if err != nil {
		return "", err
	}
	idBytes := make([]byte, 16)
	rand.Read(idBytes)
	rootfs := filepath.Join(containersDir, hex.EncodeToString(idBytes), "rootfs")
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
