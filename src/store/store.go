// $Id$
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

// ErrVolumeNotExists is returned when a volume lookup finds no matching entry.
var ErrVolumeNotExists = errors.New("volume does not exist")

// DB holds the fully loaded state in memory and the path to the backing
// state.json file. It implements the Datastore interface.
type DB struct {
	mu       sync.RWMutex
	filePath string
	data     stateData
}

// NewDB loads the state from filePath. If the file does not yet exist a fresh
// empty state is written to disk and returned.
func NewDB(filePath string) (*DB, error) {
	db := &DB{filePath: filePath}

	raw, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		if err2 := db.save(); err2 != nil {
			return nil, fmt.Errorf("store: create state file %q: %w", filePath, err2)
		}
		return db, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: read state file %q: %w", filePath, err)
	}
	if err = json.Unmarshal(raw, &db.data); err != nil {
		return nil, fmt.Errorf("store: parse state file %q: %w", filePath, err)
	}
	return db, nil
}

// InitStateFile ensures the parent directory and state file both exist.
// Called once at process startup before NewDB.
func InitStateFile(filePath string) {
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		fmt.Println("store: cannot create state directory:", err)
		os.Exit(1)
	}
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		empty := stateData{}
		raw, _ := json.MarshalIndent(empty, "", "  ")
		if err2 := os.WriteFile(filePath, raw, 0600); err2 != nil {
			fmt.Println("store: cannot create state file:", err2)
			os.Exit(1)
		}
	}
}

// save writes the current in-memory state to disk atomically via a
// write-to-tmp + rename pattern. Caller must hold db.mu (write lock).
func (db *DB) save() error {
	raw, err := json.MarshalIndent(db.data, "", "  ")
	if err != nil {
		return fmt.Errorf("store: marshal state: %w", err)
	}
	tmp := db.filePath + ".tmp"
	if err = os.WriteFile(tmp, raw, 0600); err != nil {
		return fmt.Errorf("store: write tmp state: %w", err)
	}
	if err = os.Rename(tmp, db.filePath); err != nil {
		return fmt.Errorf("store: rename state file: %w", err)
	}
	return nil
}

// findByName returns the slice index of the entry with the given VolumeName,
// or -1 if not found. Caller must hold at least a read lock.
func (db *DB) findByName(volumeName string) int {
	for i, v := range db.data.Volumes {
		if v.VolumeName == volumeName {
			return i
		}
	}
	return -1
}

// findByVolumeID returns the slice index of the entry with the given VolumeID,
// or -1 if not found. Caller must hold at least a read lock.
func (db *DB) findByVolumeID(volumeID int64) int {
	for i, v := range db.data.Volumes {
		if v.VolumeID == volumeID {
			return i
		}
	}
	return -1
}

/***********************************************************************************************************************
#   Property (formerly the Property SQL table) — stores the rolling ProjectID counter
************************************************************************************************************************/

// InitializePropertyProjectID sets the project-ID counter to 1 on first run.
// It is a no-op if the counter is already initialised (value >= 1).
func (db *DB) InitializePropertyProjectID() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.data.NextProjectID >= 1 {
		return nil
	}
	db.data.NextProjectID = 1
	return db.save()
}

// GetPropertyProjectIDValue returns the current project-ID counter value.
func (db *DB) GetPropertyProjectIDValue() (int, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.data.NextProjectID, nil
}

// SetPropertyProjectID updates the project-ID counter to the given string value.
func (db *DB) SetPropertyProjectID(projectID string) error {
	id, err := strconv.Atoi(projectID)
	if err != nil {
		return fmt.Errorf("store: invalid project ID %q: %w", projectID, err)
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	db.data.NextProjectID = id
	return db.save()
}

/***********************************************************************************************************************
#   DockerVolume (formerly the DockerVolume SQL table)
************************************************************************************************************************/

// AddDockerVolume inserts a new volume with the given name and path.
func (db *DB) AddDockerVolume(volumeName string, volumePath string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	db.data.NextVolumeID++
	entry := volumeEntry{
		VolumeID:   db.data.NextVolumeID,
		VolumeName: volumeName,
		VolumePath: volumePath,
	}
	db.data.Volumes = append(db.data.Volumes, entry)
	return db.save()
}

// GetDockerVolumeCount returns the total number of volumes.
func (db *DB) GetDockerVolumeCount() (int, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return len(db.data.Volumes), nil
}

// GetDockerVolumeCountByName returns 1 if the volume exists, 0 otherwise.
func (db *DB) GetDockerVolumeCountByName(volumeName string) (int, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.findByName(volumeName) >= 0 {
		return 1, nil
	}
	return 0, nil
}

// GetDockerVolumeByName returns the DockerVolume for the given name.
func (db *DB) GetDockerVolumeByName(volumeName string) (DockerVolume, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	i := db.findByName(volumeName)
	if i < 0 {
		return DockerVolume{}, ErrVolumeNotExists
	}
	v := db.data.Volumes[i]
	return DockerVolume{
		VolumeID:   v.VolumeID,
		VolumeName: v.VolumeName,
		VolumePath: v.VolumePath,
	}, nil
}

// GetDockerVolumePathByName returns the mount path for the given volume name.
func (db *DB) GetDockerVolumePathByName(volumeName string) (string, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	i := db.findByName(volumeName)
	if i < 0 {
		return "", ErrVolumeNotExists
	}
	return db.data.Volumes[i].VolumePath, nil
}

// GetDockerVolumeIDByName returns the internal VolumeID for the given name,
// or -1 if not found.
func (db *DB) GetDockerVolumeIDByName(volumeName string) (int, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	i := db.findByName(volumeName)
	if i < 0 {
		return -1, nil
	}
	return int(db.data.Volumes[i].VolumeID), nil
}

// CheckIfMountPointAvailable returns true if no existing volume uses mountpoint.
func (db *DB) CheckIfMountPointAvailable(mountpoint string) (bool, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	for _, v := range db.data.Volumes {
		if v.VolumePath == mountpoint {
			return false, nil
		}
	}
	return true, nil
}

// RemoveDockerVolumeByName deletes the volume entry with the given name.
// Returns the number of records removed (0 or 1).
func (db *DB) RemoveDockerVolumeByName(volumeName string) (int64, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	i := db.findByName(volumeName)
	if i < 0 {
		return 0, nil
	}
	db.data.Volumes = append(db.data.Volumes[:i], db.data.Volumes[i+1:]...)
	if err := db.save(); err != nil {
		return 0, err
	}
	return 1, nil
}

/***********************************************************************************************************************
#   XFSProject (formerly the XFSProject SQL table)
************************************************************************************************************************/

// AddXFSProject updates the volume entry identified by volumeID (the VolumeID
// from AddDockerVolume / GetDockerVolumeIDByName) with the XFS quota fields
// supplied in xfsVolume.
//
// Note: the parameter named "projectID" here is in fact the VolumeID (the FK
// from the old schema). This matches the original call-sites in driver.go.
func (db *DB) AddXFSProject(xfsVolume XFSVolDetails, projectID int) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	i := db.findByVolumeID(int64(projectID))
	if i < 0 {
		return fmt.Errorf("store: volume with ID %d not found", projectID)
	}
	e := &db.data.Volumes[i]
	e.CreatedAt = xfsVolume.CreatedAt
	e.ProjectID = xfsVolume.ProjectID
	e.BlockHardLimit = xfsVolume.BlockHardLimit
	e.BlockSoftLimit = xfsVolume.BlockSoftLimit
	e.InodeHardLimit = xfsVolume.InodeHardLimit
	e.InodeSoftLimit = xfsVolume.InodeSoftLimit
	e.ReadBPS = xfsVolume.ReadBPS
	e.WriteBPS = xfsVolume.WriteBPS
	e.ReadIOPS = xfsVolume.ReadIOPS
	e.WriteIOPS = xfsVolume.WriteIOPS
	return db.save()
}

// GetXFSProjectByID returns the XFSProject for the volume with the given
// VolumeID. (The SQL implementation queried "WHERE VolumeID = ?" despite the
// parameter being named projectID — this method preserves that behaviour.)
func (db *DB) GetXFSProjectByID(volumeID int64) (XFSProject, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	i := db.findByVolumeID(volumeID)
	if i < 0 {
		return XFSProject{}, nil
	}
	v := db.data.Volumes[i]
	return XFSProject{
		CreatedAt:      v.CreatedAt,
		ProjectID:      v.ProjectID,
		VolumeID:       int(v.VolumeID),
		BlockHardLimit: v.BlockHardLimit,
		BlockSoftLimit: v.BlockSoftLimit,
		InodeHardLimit: v.InodeHardLimit,
		InodeSoftLimit: v.InodeSoftLimit,
		ReadBPS:        v.ReadBPS,
		WriteBPS:       v.WriteBPS,
		ReadIOPS:       v.ReadIOPS,
		WriteIOPS:      v.WriteIOPS,
	}, nil
}

// CheckIfXFSProjectIDExists returns true if any volume entry carries the
// given XFS ProjectID.
func (db *DB) CheckIfXFSProjectIDExists(projectID int) (bool, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	for _, v := range db.data.Volumes {
		if v.ProjectID == projectID {
			return true, nil
		}
	}
	return false, nil
}

/***********************************************************************************************************************
#   Combined queries (formerly SQL JOINs)
************************************************************************************************************************/

// GetALLDetailsByName returns both the DockerVolume and XFSProject records for
// the volume with the given name in a single lookup.
func (db *DB) GetALLDetailsByName(volumeName string) (DockerVolume, XFSProject, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	i := db.findByName(volumeName)
	if i < 0 {
		return DockerVolume{}, XFSProject{}, ErrVolumeNotExists
	}
	v := db.data.Volumes[i]
	dv := DockerVolume{VolumeID: v.VolumeID, VolumeName: v.VolumeName, VolumePath: v.VolumePath}
	xp := XFSProject{
		CreatedAt:      v.CreatedAt,
		ProjectID:      v.ProjectID,
		VolumeID:       int(v.VolumeID),
		BlockHardLimit: v.BlockHardLimit,
		BlockSoftLimit: v.BlockSoftLimit,
		InodeHardLimit: v.InodeHardLimit,
		InodeSoftLimit: v.InodeSoftLimit,
		ReadBPS:        v.ReadBPS,
		WriteBPS:       v.WriteBPS,
		ReadIOPS:       v.ReadIOPS,
		WriteIOPS:      v.WriteIOPS,
	}
	return dv, xp, nil
}

// GetALLVolumes returns all volumes as a slice of XFSVolDetails.
func (db *DB) GetALLVolumes() ([]XFSVolDetails, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	result := make([]XFSVolDetails, 0, len(db.data.Volumes))
	for _, v := range db.data.Volumes {
		result = append(result, XFSVolDetails{
			VolumeName:     v.VolumeName,
			VolumePath:     v.VolumePath,
			CreatedAt:      v.CreatedAt,
			ProjectID:      v.ProjectID,
			BlockHardLimit: v.BlockHardLimit,
			BlockSoftLimit: v.BlockSoftLimit,
			InodeHardLimit: v.InodeHardLimit,
			InodeSoftLimit: v.InodeSoftLimit,
			ReadBPS:        v.ReadBPS,
			WriteBPS:       v.WriteBPS,
			ReadIOPS:       v.ReadIOPS,
			WriteIOPS:      v.WriteIOPS,
		})
	}
	return result, nil
}
