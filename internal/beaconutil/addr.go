package beaconutil

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
)

const LockFile = ".confort.lock"

func Address(lockFile string) (string, error) {
	addr := os.Getenv(AddressEnv)
	if addr != "" {
		return addr, nil
	}

	data, err := os.ReadFile(lockFile)
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(data)), nil
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

func StoreAddressToEnv(addr string) error {
	return os.Setenv(AddressEnv, addr)
}
