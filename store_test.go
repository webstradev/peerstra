package main

import (
	"bytes"
	"os"
	"testing"
)

func TestPathTransformFunc(t *testing.T) {
	key := "keyforimage"
	path := CASPathTransformFunc(key)
	expectedPathname := "917e9/58f55/1206b/ae6ad/e690a/955d0/7f1fe/ff303"
	expectedOriginalKey := "917e958f551206bae6ade690a955d07f1feff303"

	if path.PathName != expectedPathname {
		t.Fatal("pathname mismatch")
	}

	if path.Original != expectedOriginalKey {
		t.Fatal("original key mismatch")
	}
}

func TestStorre(t *testing.T) {
	defer func() {
		os.RemoveAll("77c47")
	}()

	opts := StoreOpts{
		PathTransformFunc: CASPathTransformFunc,
	}
	s := NewStore(opts)

	r := bytes.NewReader([]byte("some jpg bytes"))
	if err := s.writeStream("someimage", r); err != nil {
		t.Fatal(err)
	}
}
