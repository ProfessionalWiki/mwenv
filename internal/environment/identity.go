package environment

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

const (
	firstPort = 20000
	portCount = 10000
)

var nonIDCharacters = regexp.MustCompile(`[^a-z0-9]+`)

func ID(name string, worktree string) string {
	normalized := strings.Trim(nonIDCharacters.ReplaceAllString(strings.ToLower(name), "-"), "-")
	hash := sha256.Sum256([]byte(worktree))
	return normalized + "-" + hex.EncodeToString(hash[:4])
}

func StartPort(worktree string) int {
	hash := sha256.Sum256([]byte(worktree))
	return firstPort + int(binary.BigEndian.Uint32(hash[:4])%portCount)
}

func PickPort(start int, taken map[int]bool, isFree func(port int) bool) (int, error) {
	for offset := range portCount {
		port := firstPort + (start-firstPort+offset)%portCount

		if !taken[port] && isFree(port) {
			return port, nil
		}
	}

	return 0, fmt.Errorf("no free port in %d-%d", firstPort, firstPort+portCount-1)
}
