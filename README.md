# containerR

`containerR` is a small Linux container-runtime experiment written in Go. It
pulls OCI images directly from registries into a local OCI Image Layout, then
runs a command inside an extracted root filesystem with isolated PID, UTS,
and mount namespaces.

## Usage

Build the launcher:

```sh
go build -o containerR .
```

Pull an image, view locally imported images, and run a command:

```sh
./containerR pull alpine:latest
./containerR images
./containerR run alpine:latest /bin/sh
# With no command override, run the image's configured entrypoint and command.
./containerR run alpine:latest
```

The local image store and extracted container filesystems are stored under
`~/.local/share/containerR`:

| Path | Purpose |
| --- | --- |
| `~/.local/share/containerR/images/oci-layout` | Identifies the OCI layout version. |
| `~/.local/share/containerR/images/index.json` | References saved images. |
| `~/.local/share/containerR/images/blobs/sha256/` | Stores manifests, configurations, and layers. |
| `~/.local/share/containerR/containerr.db` | SQLite database of created containers. Its schema is initialized from `schema.sql` when the database is opened. |
| `~/.local/share/containerR/containers/<id>/rootfs/` | Holds one container's extracted filesystem. |

Each `run` creates a container record before it starts. The internal
`container-init` command receives that record's ID (rather than a rootfs path)
and reads the rootfs location from the database before entering the chroot.

### For Macos

Use to create a Linux VM

```
limactl start --name=containerr ./lima/containerr.yaml
```

and then ran all the commands in the vm as for now we didn't add support for macos

## About

This project is intended for learning and experimentation, not as a
production container runtime. Image pulls use the registry API directly;
process isolation is performed with Linux primitives.

## Future work

I hope to find time to keep improving containerR. Planned areas include:

- Image and container lifecycle management
- Network namespace support and basic container networking
- Safer isolation defaults and clearer error reporting
