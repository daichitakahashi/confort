package beacon

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strings"
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

var ErrIntegrationDisabled = errors.New("the integration with beacon server is disabled")

// Address returns address of beacon server.
// It returns value of CFT_BEACON_ADDR if exists.
// If the value of CFT_BEACON_ADDR equals "disabled", this returns ErrIntegrationDisabled.
//
// If the variable not exists, Address try to read from lockFile.
func Address(ctx context.Context, lockFile string) (string, error) {
	addr := os.Getenv(AddressEnv)
	if addr != "" {
		if strings.EqualFold(addr, "disabled") {
			return "", ErrIntegrationDisabled
		}
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
	return "", nil
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
