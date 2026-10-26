package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// Parámetros de argon2id (OWASP 2024: 19 MiB, 2 iteraciones, 1 hilo).
const (
	argonTime    = 2
	argonMemory  = 19 * 1024
	argonThreads = 1
	argonKeyLen  = 32
	saltLen      = 16
)

// HashPassword devuelve el hash argon2id en formato PHC:
// $argon2id$v=19$m=19456,t=2,p=1$<sal b64>$<hash b64>.
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("contraseña vacía")
	}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	b64 := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads, b64(salt), b64(key)), nil
}

// VerifyPassword compara en tiempo constante. Acepta los tres formatos que la
// casa tiene en producción, para migrar sin forzar un cambio de contraseña:
//   - $argon2id$... (este paquete)
//   - pbkdf2$sha256$<iter>$<sal>$<hash> (template de Workers)
//   - $2a$/$2b$ bcrypt
//
// needsRehash es true cuando el hash no es argon2id con los parámetros
// actuales: tras un login correcto, vuelve a guardar con HashPassword.
func VerifyPassword(password, stored string) (ok, needsRehash bool) {
	switch {
	case strings.HasPrefix(stored, "$argon2id$"):
		ok, current := verifyArgon(password, stored)
		return ok, ok && !current
	case strings.HasPrefix(stored, "pbkdf2$"):
		return verifyPBKDF2(password, stored), true
	case strings.HasPrefix(stored, "$2a$"), strings.HasPrefix(stored, "$2b$"), strings.HasPrefix(stored, "$2y$"):
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(password)) == nil, true
	}
	return false, false
}

func verifyArgon(password, stored string) (ok, current bool) {
	parts := strings.Split(stored, "$")
	if len(parts) != 6 {
		return false, false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, false
	}
	var mem, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &t, &p); err != nil {
		return false, false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[4])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[5])
	if err1 != nil || err2 != nil || len(want) == 0 {
		return false, false
	}
	got := argon2.IDKey([]byte(password), salt, t, mem, p, uint32(len(want)))
	ok = subtle.ConstantTimeCompare(got, want) == 1
	current = mem == argonMemory && t == argonTime && p == argonThreads
	return ok, current
}

func verifyPBKDF2(password, stored string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 5 || parts[1] != "sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[2])
	if err != nil || iter < 10_000 || iter > 2_000_000 {
		return false
	}
	salt, err1 := base64.StdEncoding.DecodeString(parts[3])
	want, err2 := base64.StdEncoding.DecodeString(parts[4])
	if err1 != nil || err2 != nil || len(want) == 0 {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// ConstantTimeEqual compara dos textos sin revelar por tiempo cuánto coinciden.
func ConstantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
