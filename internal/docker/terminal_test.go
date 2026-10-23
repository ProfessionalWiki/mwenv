package docker

import (
	"os"
	"testing"
)

func TestDevNullIsNotATerminal(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()

	if isTerminal(devNull) {
		t.Error("expected /dev/null not to be a terminal")
	}
}
