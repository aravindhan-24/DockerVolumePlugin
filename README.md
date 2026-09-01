# VolumePlugin

Docker volume plugin for creating XFS-backed volumes with project quota limits and optional blkio throttling.

## What it does

This project implements a Docker volume driver that:

- creates per-volume directories under an XFS-backed mount
- assigns a unique XFS project ID to each volume
- applies disk and inode quota limits via XFS project quotas
- persists metadata in a local state file for volume tracking
- optionally applies read/write BPS and IOPS limits through cgroup blkio
- exposes a Docker plugin socket for volume lifecycle operations

The driver is implemented in Go and is designed to work as a Docker volume plugin using the Docker plugin helper APIs.

## Requirements

- Linux host with Docker installed
- XFS filesystem support
- root or sudo access for setup and service installation
- `xfs_quota`, `mkfs.xfs`, and mount support
- systemd is used by the included service files

## Repository layout

- `src/` - Go source code for the plugin
- `product_package/bin/` - systemd unit files
- `product_package/config/config.json` - default plugin configuration
- `src/Makefile` - build, install, xfs setup, and service helpers

## Default configuration

The plugin reads its configuration from `/etc/dockerplugins/VolumePlugin/config.json` by default. The shipped config looks like this:

```json
{
  "driver": "VolumePlugin",
  "statepath": "/var/lib/dockerplugins/VolumePlugin/state.json",
  "isUnixSocket": true,
  "socketAddress": "/run/docker/plugins/",
  "defaultXFSMountPoint": "/mnt/network_drive/plugin/xfs",
  "defaultBlockHardLimit": "10g",
  "defaultBlockSoftLimit": "10g",
  "defaultInodeHardLimit": "500000",
  "defaultInodeSoftLimit": "500000",
  "defaultReadBPS": "1m",
  "defaultWriteBPS": "1m",
  "defaultReadIOPS": "100",
  "defaultWriteIOPS": "100",
  "mntptDirMode": "0755",
  "ownerOfMountPoint": "<YOU_MOUNT-POINT-OWNER-NAME>"
}
```

Key notes:

- `defaultXFSMountPoint` is the XFS filesystem mount used for project quotas.
- `MountPoint` is supplied per volume and must be inside the XFS mount tree.
- `ownerOfMountPoint` is the Linux user/group owner that should own created volume directories.
- `isUnixSocket` enables UNIX socket mode; otherwise the plugin can expose a TCP socket.

## Build and install

From the project root:

```bash
cd src
make deps
make build
sudo make install
```

The install target does the following:

- builds the binary
- creates the backing XFS image and mounts it with `pquota`
- installs the systemd service and socket
- copies the config file to `/etc/dockerplugins/VolumePlugin/config.json`
- reloads and enables the Docker plugin service

You can also run the setup steps manually:

```bash
cd src
make xfs-setup
make xfs-mount
make xfs-umount
```

## Run the plugin

The plugin binary is installed as `/usr/bin/VolumePlugin` and is started by the systemd unit:

```bash
sudo systemctl daemon-reload
sudo systemctl enable VolumePlugin.socket
sudo systemctl enable VolumePlugin.service
sudo systemctl start VolumePlugin.socket
sudo systemctl start VolumePlugin.service
```

To inspect logs:

```bash
sudo journalctl -u VolumePlugin -f
```

## Volume creation

Create a Docker volume using the plugin driver:

```bash
docker volume create -d VolumePlugin --name demo \
  -o MountPoint=/mnt/network_drive/plugin/xfs/demo \
  -o XFSDiskMountPoint=/mnt/network_drive/plugin/xfs \
  -o BlockHardLimit=2g \
  -o BlockSoftLimit=1g \
  -o InodeHardLimit=200000 \
  -o InodeSoftLimit=150000
```

The plugin validates that the requested mount path is inside the configured XFS mount and then creates the directory, assigns a project ID, and applies quotas.

You can also set cgroup blkio throttling options:

```bash
docker volume create -d VolumePlugin --name throttled \
  -o MountPoint=/mnt/network_drive/plugin/xfs/throttled \
  -o XFSDiskMountPoint=/mnt/network_drive/plugin/xfs \
  -o ReadBPS=10m \
  -o WriteBPS=10m \
  -o ReadIOPS=200 \
  -o WriteIOPS=200
```

Supported values are byte-size strings like `1g`, `500m`, `10k` for BPS, and integer values for IOPS.

## Volume management

Common Docker commands:

```bash
docker volume ls
docker volume inspect demo
docker volume rm demo
```

The plugin also exposes `List`, `Get`, `Path`, `Mount`, `Unmount`, and `Remove` operations through the Docker volume plugin interface.

## Service and cleanup

To stop and remove the installed plugin:

```bash
cd src
make clean
```

The clean target removes the service files, unmounts the XFS mount, deletes the state directory, and removes any plugin-managed Docker volumes.

## Notes

- This plugin expects a valid XFS-backed backing store and appropriate filesystem privileges.
- The project ID allocator tracks project IDs in the plugin state store.
- The volume driver is designed for local scope and is intended for Docker hosts with direct access to the XFS mount.

## Troubleshooting

If the plugin fails to start:

- confirm the XFS mount exists at the configured `defaultXFSMountPoint`
- verify the backing image was created with `mkfs.xfs`
- ensure the plugin config file matches the actual host paths
- check the journal logs with `sudo journalctl -u VolumePlugin -n 200 --no-pager`
- confirm the plugin socket path is present under `/run/docker/plugins/`

## Source

The main entry point is in `src/main.go`, with the volume driver logic in `src/handler/driver.go` and the CLI in `src/cmd/cmg.go`.
