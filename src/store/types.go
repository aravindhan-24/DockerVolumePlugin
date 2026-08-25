package store

// DockerVolume represents a Docker volume record.
type DockerVolume struct {
	VolumeID   int64
	VolumeName string
	VolumePath string
}

// Property represents a key/value configuration entry.
type Property struct {
	Key   string
	Value string
}

// XFSProject represents the XFS quota project record for a volume.
type XFSProject struct {
	CreatedAt      string
	ProjectID      int
	VolumeID       int
	BlockHardLimit string
	BlockSoftLimit string
	InodeHardLimit string
	InodeSoftLimit string
	// Cgroup blkio throttle limits (0 = unlimited).
	ReadBPS   uint64
	WriteBPS  uint64
	ReadIOPS  uint64
	WriteIOPS uint64
}

// XFSVolDetails is a flattened view that joins DockerVolume and XFSProject,
// used throughout the driver and utility layers.
type XFSVolDetails struct {
	VolumeName     string
	VolumePath     string
	CreatedAt      string
	ProjectID      int
	BlockHardLimit string
	BlockSoftLimit string
	InodeHardLimit string
	InodeSoftLimit string
	// Cgroup blkio throttle limits (0 = unlimited).
	ReadBPS   uint64
	WriteBPS  uint64
	ReadIOPS  uint64
	WriteIOPS uint64
}

// volumeEntry is a single record in state.json. It merges the DockerVolume
// and XFSProject fields so no JOIN step is required at query time.
type volumeEntry struct {
	VolumeID       int64  `json:"volumeID"`
	VolumeName     string `json:"volumeName"`
	VolumePath     string `json:"volumePath"`
	CreatedAt      string `json:"createdAt"`
	ProjectID      int    `json:"projectID"`
	BlockHardLimit string `json:"blockHardLimit"`
	BlockSoftLimit string `json:"blockSoftLimit"`
	InodeHardLimit string `json:"inodeHardLimit"`
	InodeSoftLimit string `json:"inodeSoftLimit"`
	// Cgroup blkio throttle limits (0 = unlimited).
	ReadBPS   uint64 `json:"readBPS"`
	WriteBPS  uint64 `json:"writeBPS"`
	ReadIOPS  uint64 `json:"readIOPS"`
	WriteIOPS uint64 `json:"writeIOPS"`
}

// stateData is the root object written to and read from state.json.
type stateData struct {
	NextVolumeID  int64         `json:"nextVolumeID"`
	NextProjectID int           `json:"nextProjectID"`
	Volumes       []volumeEntry `json:"volumes"`
}

// Datastore is the storage interface that the rest of the codebase depends on.
type Datastore interface {
	// DockerVolume operations
	AddDockerVolume(volumeName string, volumePath string) (err error)
	GetDockerVolumePathByName(volumeName string) (volumePath string, err error)
	GetDockerVolumeByName(volumeName string) (dockerVolume DockerVolume, err error)
	GetDockerVolumeIDByName(volumeName string) (volumeId int, err error)
	GetDockerVolumeCountByName(volumeName string) (count int, err error)
	GetDockerVolumeCount() (count int, err error)
	RemoveDockerVolumeByName(volumeName string) (count int64, err error)
	CheckIfMountPointAvailable(mountpoint string) (result bool, err error)

	// XFSProject operations
	AddXFSProject(xfsVolume XFSVolDetails, projectID int) (err error)
	GetXFSProjectByID(projectID int64) (xfsProject XFSProject, err error)
	CheckIfXFSProjectIDExists(projectID int) (result bool, err error)

	// Combined queries
	GetALLDetailsByName(volumeName string) (dockerVolume DockerVolume, xfsProject XFSProject, err error)
	GetALLVolumes() (xfsVolDetailsArr []XFSVolDetails, err error)

	// Property operations
	SetPropertyProjectID(projectID string) (err error)
	GetPropertyProjectIDValue() (propertyId int, err error)
	InitializePropertyProjectID() (err error)
}
