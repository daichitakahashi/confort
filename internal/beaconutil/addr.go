package beaconutil

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"
)

// LockFile is default file name of lock file.
const LockFile = ".confort.lock"

// LockFilePath returns file name of lock file.
// If CFT_LOCKFILE is set, return its value, or else return LockFile.
func LockFilePath() string {
	v, ok := os.LookupEnv(LockFileEnv)
	if ok {
		return v
	}
	return LockFile
}

// Address returns address of beacon server.
// It returns value of CFT_BEACON_ADDR if exists.
// If not exists, read from lockFile.
func Address(ctx context.Context, lockFile string) (string, error) {
	addr := os.Getenv(AddressEnv)
	if addr != "" {
		return addr, nil
	}

	for i := 0; i < 10; i++ {
		select {
		case <-time.After(200 * time.Millisecond):
			data, err := os.ReadFile(lockFile)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return "", err
			}
			return string(data), nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return "", fmt.Errorf("failed to read lock file: %s", lockFile)
}

func StoreAddressToLockFile(lockFile, addr string) error {
	return os.WriteFile(lockFile, []byte(addr), 0644)
}

func DeleteLockFile(lockFile string) error {
	_, err := os.Stat(lockFile)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return os.Remove(lockFile)
}
