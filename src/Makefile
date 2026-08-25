BINARY_NAME    = VolumePlugin
INSTALL_PATH   = /usr/bin
SERVICE_NAME   = VolumePlugin
CONFIG_DIR     = /etc/dockerplugins/$(SERVICE_NAME)
STATE_DIR      = /var/lib/dockerplugins/$(SERVICE_NAME)
PKG_DIR        = ../product_package
SERVICE_FILE   = $(PKG_DIR)/bin/$(SERVICE_NAME).service
SOCKET_FILE    = $(PKG_DIR)/bin/$(SERVICE_NAME).socket
CONFIG_FILE    = $(PKG_DIR)/conf/config.json

# XFS backing image — path must match defaultXFSMountPoint in config.json
XFS_IMG        = $(STATE_DIR)/xfs.img
XFS_MOUNT      = /mnt/network_drive/plugin/xfs

.PHONY: all build install clean test deps run logs \
        xfs-setup xfs-mount xfs-umount \
        debug-build debug-run

all: build

build:
	go build -buildvcs=false -o $(BINARY_NAME) .

# xfs-setup: create and format the backing image on first run, then mount it.
# Idempotent: skips dd/mkfs if the image already exists, skips mount if already mounted.
xfs-setup:
	@if [ ! -f $(XFS_IMG) ]; then \
		echo "--- Creating 10 GB XFS image at $(XFS_IMG)..."; \
		sudo mkdir -p $(STATE_DIR); \
		sudo dd if=/dev/zero of=$(XFS_IMG) bs=1M count=10240 status=progress \
			|| { sudo rm -f $(XFS_IMG); exit 1; }; \
		sudo mkfs.xfs -f $(XFS_IMG) \
			|| { echo "--- mkfs.xfs failed, removing image."; sudo rm -f $(XFS_IMG); exit 1; }; \
		echo "--- XFS image created and formatted."; \
	else \
		echo "--- XFS image already exists, skipping dd/mkfs."; \
	fi
	@if ! mountpoint -q $(XFS_MOUNT) 2>/dev/null; then \
		sudo mkdir -p $(XFS_MOUNT); \
		sudo mount -o loop,pquota $(XFS_IMG) $(XFS_MOUNT) \
			|| { echo "--- mount failed. Run: dmesg | tail -20"; exit 1; }; \
		echo "--- XFS image mounted at $(XFS_MOUNT) with pquota."; \
	else \
		echo "--- $(XFS_MOUNT) is already mounted."; \
	fi

# xfs-mount: re-mount the existing image after a reboot (no dd/mkfs).
xfs-mount:
	@if ! mountpoint -q $(XFS_MOUNT) 2>/dev/null; then \
		sudo mkdir -p $(XFS_MOUNT); \
		sudo mount -o loop,pquota $(XFS_IMG) $(XFS_MOUNT); \
		echo "--- XFS image mounted at $(XFS_MOUNT)."; \
	else \
		echo "--- $(XFS_MOUNT) is already mounted."; \
	fi

# xfs-umount: unmount the XFS image.
xfs-umount:
	sudo umount $(XFS_MOUNT) 2>/dev/null || true

install: build xfs-setup
	sudo cp $(BINARY_NAME) $(INSTALL_PATH)/
	sudo chmod +x $(INSTALL_PATH)/$(BINARY_NAME)
	sudo cp $(SERVICE_FILE) /etc/systemd/system/
	sudo cp $(SOCKET_FILE) /etc/systemd/system/
	sudo mkdir -p $(CONFIG_DIR)
	sudo cp $(CONFIG_FILE) $(CONFIG_DIR)/config.json
	sudo mkdir -p $(STATE_DIR)
	sudo systemctl daemon-reload
	sudo systemctl enable $(SERVICE_NAME).socket
	sudo systemctl enable $(SERVICE_NAME).service
	sudo systemctl restart $(SERVICE_NAME).service

clean:
	rm -f $(BINARY_NAME)
	# Remove all volumes managed by this plugin so Docker releases its mounts first.
	@docker volume ls -q --filter driver=$(SERVICE_NAME) | xargs -r docker volume rm 2>/dev/null || \
		echo "Warning: some plugin volumes could not be removed (containers still running?)"
	sudo systemctl stop $(SERVICE_NAME).service 2>/dev/null || true
	sudo systemctl stop $(SERVICE_NAME).socket 2>/dev/null || true
	sudo systemctl disable $(SERVICE_NAME).service 2>/dev/null || true
	sudo systemctl disable $(SERVICE_NAME).socket 2>/dev/null || true
	sudo rm -f /etc/systemd/system/$(SERVICE_NAME).service
	sudo rm -f /etc/systemd/system/$(SERVICE_NAME).socket
	sudo rm -f $(INSTALL_PATH)/$(BINARY_NAME)
	sudo rm -rf $(CONFIG_DIR)
	# Unmount and detach the loop device in one step (-d).
	@if mountpoint -q $(XFS_MOUNT) 2>/dev/null; then \
		sudo umount -d $(XFS_MOUNT) && echo "--- Unmounted $(XFS_MOUNT)." \
		|| echo "WARNING: umount failed — check: sudo fuser -mv $(XFS_MOUNT)"; \
	else \
		echo "--- $(XFS_MOUNT) is not mounted, skipping umount."; \
	fi
	# Detach any remaining loop device backed by the image (belt-and-suspenders).
	@sudo losetup -j $(XFS_IMG) 2>/dev/null | cut -d: -f1 | xargs -r sudo losetup -d && true
	sudo rm -rf $(STATE_DIR)
	sudo systemctl daemon-reload

test:
	go test -v ./...

deps:
	go mod download
	go mod tidy

run:
	sudo ./$(BINARY_NAME)

logs:
	sudo journalctl -u $(SERVICE_NAME) -f

# Set your dlv path here. You can find it by running `which dlv` in your terminal.
# DLV := ""

debug-build:
	go build -buildvcs=false -gcflags="all=-N -l" -o $(BINARY_NAME) .

debug-run: debug-build
	sudo systemctl stop $(SERVICE_NAME).service 2>/dev/null || true
	sudo $(DLV) exec ./$(BINARY_NAME) --headless --listen=127.0.0.1:2345 --api-version=2 --accept-multiclient --only-same-user=false
