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
sudo ./containerR pull alpine:latest
sudo ./containerR images
sudo ./containerR run alpine:latest /bin/sh
```

The local image store and extracted container filesystems are stored under
`/lib/containerR`:

| Path | Purpose |
| --- | --- |
| `/lib/containerR/images/oci-layout` | Identifies the OCI layout version. |
| `/lib/containerR/images/index.json` | References saved images. |
| `/lib/containerR/images/blobs/sha256/` | Stores manifests, configurations, and layers. |
| `/lib/containerR/containers/<id>/rootfs/` | Holds one container's extracted filesystem. |

### For Macos

Use to create a Linux VM

```
limactl start --name=containerr ./lima/containerr.yaml

limactl edit --mount-writable containerr
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
