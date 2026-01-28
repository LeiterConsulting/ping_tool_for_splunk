package util

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// EventID mirrors v4 logic:
// SHA256("collector|target_ip|record_type|timestamp[|ping_number]") lower-case hex.
func EventID(collectorHost, targetIP, recordType, timestamp string, pingNumber int) string {
	input := ""
	if pingNumber >= 0 {
		input = fmt.Sprintf("%s|%s|%s|%s|%d", collectorHost, targetIP, recordType, timestamp, pingNumber)
	} else {
		input = fmt.Sprintf("%s|%s|%s|%s", collectorHost, targetIP, recordType, timestamp)
	}
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}
