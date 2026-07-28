package secretbox

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestSealAndOpen(t *testing.T) {
	box, err := Open(filepath.Join(t.TempDir(), "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]string{"token": "super-secret"}
	sealed, err := box.Seal(input)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("super-secret")) {
		t.Fatal("ciphertext contains plaintext")
	}
	var output map[string]string
	if err := box.Open(sealed, &output); err != nil {
		t.Fatal(err)
	}
	if output["token"] != input["token"] {
		t.Fatalf("got %q", output["token"])
	}
}
