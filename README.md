# containerR

`containerR` is a small Linux container-runtime experiment written in Go. It
imports Docker images as root filesystems, then runs a command inside a
chroot with isolated PID, UTS, and mount namespaces.

## Usage

Build the launcher:

```sh
go build -o containerR .
```

Pull an image, view locally imported images, and run a command:

```sh
sudo ./containerR pull alpine:latest
sudo ./containerR images
sudo ./containerR run alpine:latest /bin/sh
```

Imported image files and metadata are stored under `/lib/containerR`.

## About

This project is intended for learning and experimentation, not as a
production container runtime. It uses Docker only to obtain image root
filesystems; process isolation is performed directly with Linux primitives.

## Future work

I hope to find time to keep improving containerR. Planned areas include:

- Image and container lifecycle management
- Replacing the Docker CLI dependency with [`go-containerregistry`](https://github.com/google/go-containerregistry)
- Network namespace support and basic container networking
- Safer isolation defaults and clearer error reporting
