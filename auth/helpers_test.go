package auth

import (
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/base64"
)

func pbkdf2Hex(password, salt string, iter int) string {
	k, _ := pbkdf2.Key(sha256.New, password, []byte(salt), iter, 32)
	return base64.StdEncoding.EncodeToString(k)
}
