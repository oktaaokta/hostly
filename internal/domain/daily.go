package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

func DailyKey(secret, date string) string {
	sum := sha256.Sum256([]byte(secret + ":" + date))
	return hex.EncodeToString(sum[:])[:16]
}

func GenerateSecret() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}
