package util

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
)

const (
	LabelIdentifier = "daichitakahashi.confort.beacon.identifier"
)

func Identifier(s string) string {
	if id := os.Getenv(IdentifierEnv); id != "" {
		return id
	}
	hash := sha256.Sum256([]byte(s))
	return hex.EncodeToString(hash[:])
}
