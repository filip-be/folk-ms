package aesgcm

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	c, err := New(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	ct, err := c.Encrypt([]byte("page-token"), []byte("orgs/a/social"))
	if err != nil {
		t.Fatal(err)
	}
	pt, err := c.Decrypt(ct, []byte("orgs/a/social"))
	if err != nil || string(pt) != "page-token" {
		t.Fatalf("%s %v", pt, err)
	}
	if _, err := c.Decrypt(ct, []byte("orgs/b/social")); err == nil {
		t.Fatal("aad must bind the ciphertext")
	}
	if _, err := New(base64.StdEncoding.EncodeToString([]byte("short"))); err == nil {
		t.Fatal("short key must fail")
	}
}
