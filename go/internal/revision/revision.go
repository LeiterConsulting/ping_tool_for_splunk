package revision

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
)

// File returns a stable content revision suitable for optimistic concurrency
// checks. It deliberately ignores timestamps so equivalent content has the
// same revision after an atomic rewrite or restore.
func File(path string) (string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(contents)
	return hex.EncodeToString(sum[:]), nil
}
