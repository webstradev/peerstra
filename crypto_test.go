package main

import (
	"bytes"
	"testing"
)

func TestNewEncryptionKey(t *testing.T) {
	key := newEncryptionKey()
	for i := 0; i < len(key); i++ {
		if key[i] == 0x0 {
			t.Error("0 bytes")
		}
	}
}

func TestCopyEncryptDecrypt(t *testing.T) {
	payload := "Foo not Bar"

	src := bytes.NewReader([]byte(payload))
	dst := bytes.NewBuffer(nil)
	key := newEncryptionKey()

	_, err := copyEncrypt(key, src, dst)
	if err != nil {
		t.Error(err)
	}

	out := bytes.NewBuffer(nil)
	nw, err := copyDecrypt(key, dst, out)
	if err != nil {
		t.Error(err)
	}

	if nw != 16+len(payload) {
		t.Error("Incorrect number of bytes written")
	}

	if out.String() != payload {
		t.Error("Decrypted data does not match payload")
	}
}
