package schema_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/filip-be/folk-ms/cms/internal/schema"
)

const contractDir = "../../../contract"

func load(t *testing.T) *schema.Schema {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(contractDir, "events.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := schema.Compile(data)
	if err != nil {
		t.Fatalf("contract schema must use only supported keywords: %v", err)
	}
	return s
}

func TestFixtures(t *testing.T) {
	s := load(t)
	for _, kind := range []string{"valid", "invalid"} {
		files, _ := filepath.Glob(filepath.Join(contractDir, "fixtures", kind, "*.json"))
		if len(files) == 0 {
			t.Fatalf("no %s fixtures", kind)
		}
		for _, f := range files {
			t.Run(kind+"/"+filepath.Base(f), func(t *testing.T) {
				data, err := os.ReadFile(f)
				if err != nil {
					t.Fatal(err)
				}
				err = s.Validate(data)
				if kind == "valid" && err != nil {
					t.Fatalf("expected valid: %v", err)
				}
				if kind == "invalid" && err == nil {
					t.Fatal("expected invalid")
				}
			})
		}
	}
}

func TestRejectsUnsupportedKeyword(t *testing.T) {
	_, err := schema.Compile([]byte(`{"type":"object","oneOf":[]}`))
	if err == nil || !strings.Contains(err.Error(), "oneOf") {
		t.Fatalf("want unsupported keyword error, got %v", err)
	}
}

func TestRejectsUnresolvedRef(t *testing.T) {
	_, err := schema.Compile([]byte(`{"$ref":"#/$defs/nope"}`))
	if err == nil {
		t.Fatal("want error")
	}
}
