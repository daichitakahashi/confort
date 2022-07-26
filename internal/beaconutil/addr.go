package beaconutil

import (
	"bytes"
	"fmt"
	"os"
)

const LockFile = ".confort.lock"

const addrEnv = "CFT_BEACON_ADDR"

func Address(lockFile string) (string, error) {
	addr := os.Getenv(addrEnv)
	if addr != "" {
		return addr, nil
	}

	data, err := os.ReadFile(lockFile)
	if err != nil {
		return "", fmt.Errorf("cannot read lock file %q: %w", lockFile, err)
	}
	return string(bytes.TrimSpace(data)), nil
}

func StoreAddressToLockFile(lockFile, addr string) error {
	return os.WriteFile(lockFile, []byte(addr), 0644)
}

func StoreAddressToEnv(addr string) error {
	return os.Setenv(addrEnv, addr)
}
