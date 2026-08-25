package utils

import (
	"errors"
	"os"
	"regexp"
	"strconv"
)

// RmEmptyDir removes pathOfDir only if it exists and is empty.
func RmEmptyDir(pathOfDir string) error {
	if _, e := os.Lstat(pathOfDir); e != nil {
		return nil
	}
	files, err := os.ReadDir(pathOfDir)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return os.Remove(pathOfDir)
	}
	return nil
}

func getByteSize(OptedVolSize string) (uint64, error) {
	var conversionUnit uint64 = 1
	switch {
	case regexp.MustCompile(`^\d+g$`).MatchString(OptedVolSize):
		conversionUnit = 1024 * 1024 * 1024
	case regexp.MustCompile(`^\d+m$`).MatchString(OptedVolSize):
		conversionUnit = 1024 * 1024
	case regexp.MustCompile(`^\d+k$`).MatchString(OptedVolSize):
		conversionUnit = 1024
	case regexp.MustCompile(`^\d+$`).MatchString(OptedVolSize):
		volSize, _ := strconv.Atoi(OptedVolSize)
		return uint64(volSize), nil
	default:
		return 0, errors.New("improper size format: expected a number followed by g, m, k, or no unit")
	}
	convertedVolumeSize, err := strconv.Atoi(OptedVolSize[:len(OptedVolSize)-1])
	if err != nil {
		return 0, err
	}
	return uint64(convertedVolumeSize) * conversionUnit, nil
}
