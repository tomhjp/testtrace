package testtrace

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestJSONRoundTrip(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "test-output.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var roundTripped []byte
	dec := json.NewDecoder(bytes.NewReader(b))
	for dec.More() {
		var te test2JSONEvent
		if err := dec.Decode(&te); err != nil && err != io.EOF {
			t.Fatal(err)
		}
		teJSON, err := json.Marshal(te)
		if err != nil {
			t.Fatal(err)
		}
		roundTripped = append(roundTripped, teJSON...)
		roundTripped = append(roundTripped, '\n')
	}

	if !bytes.Equal(b, roundTripped) {
		t.Fatalf("expected round trip to maintain the output; wanted:\n%s\ngot:\n%s", b, roundTripped)
	}
}
