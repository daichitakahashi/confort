package beaconutil

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"
)

const LockFile = ".confort.lock"

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
