package main

import (
	"bytes"
	"io"
	"testing"
)

func TestPathTransformFunc(t *testing.T) {
	key := "keyforimage"
	path := CASPathTransformFunc(key)
	expectedPathname := "917e9/58f55/1206b/ae6ad/e690a/955d0/7f1fe/ff303"
	expectedFilename := "917e958f551206bae6ade690a955d07f1feff303"

	if path.PathName != expectedPathname {
		t.Error("pathname mismatch")
	}

	if path.FileName != expectedFilename {
		t.Error("original key mismatch")
	}
}

func TestStore(t *testing.T) {
	s := newTestStore()

	defer tearDown(t, s)

	key := "keyforimage"
	data := []byte("some jpg bytes")

	if err := s.writeStream(key, bytes.NewReader(data)); err != nil {
		t.Error(err)
	}

	r, err := s.readStream(key)
	if err != nil {
		t.Error(err)
	}

	b, err := io.ReadAll(r)
	if err != nil {
		t.Error(err)
	}

	if !bytes.Equal(b, data) {
		t.Error("data mismatch")
	}

}

func TestDelete(t *testing.T) {
	s := newTestStore()

	defer tearDown(t, s)

	key := "keyforimage"
	data := []byte("some jpg bytes")

	if err := s.writeStream(key, bytes.NewReader(data)); err != nil {
		t.Error(err)
	}

	if err := s.Delete(key); err != nil {
		t.Error(err)
	}
}

func TestHas(t *testing.T) {
	s := newTestStore()

	defer tearDown(t, s)

	key := "keyforimage"
	data := []byte("some jpg bytes")

	if s.Has(key) {
		t.Error("expected to not have key")
	}

	if err := s.writeStream(key, bytes.NewReader(data)); err != nil {
		t.Error(err)
	}

	if !s.Has(key) {
		t.Error("expected to have key")
	}
}

func newTestStore() *Store {
	opts := StoreOpts{
		Root:              "tests",
		PathTransformFunc: CASPathTransformFunc,
	}
	return NewStore(opts)
}

func tearDown(t *testing.T, s *Store) {
	if err := s.Clear(); err != nil {
		t.Error(err)
	}
}
