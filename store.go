package main

import (
	"crypto/sha1"
	"encoding/hex"
	"io"
	"log"
	"os"
	"path/filepath"
)

type PathTransformFunc func(key string) PathKey

type PathKey struct {
	PathName string
	Original string
}

func (p PathKey) Filename() string {
	return filepath.Join(p.PathName, p.Original)
}

var DefaultPathTransformFunc = func(key string) PathKey {
	return PathKey{
		PathName: key,
		Original: key,
	}
}

func CASPathTransformFunc(key string) PathKey {
	hash := sha1.Sum([]byte(key))
	// convert [20]byte => string []byte and encode to string
	hashStr := hex.EncodeToString(hash[:])

	blockSize := 5
	sliceLen := len(hashStr) / blockSize
	paths := make([]string, sliceLen)

	for i := 0; i < sliceLen; i++ {
		from, to := i*blockSize, (i+1)*blockSize
		paths[i] = hashStr[from:to]
	}

	return PathKey{
		PathName: filepath.Join(paths...),
		Original: hashStr,
	}
}

type StoreOpts struct {
	PathTransformFunc PathTransformFunc
}

type Store struct {
	StoreOpts
}

func NewStore(opts StoreOpts) *Store {
	return &Store{
		StoreOpts: opts,
	}
}

func (s *Store) writeStream(key string, r io.Reader) error {
	paths := s.PathTransformFunc(key)
	pathName := paths.PathName

	// Create necessary directories
	if err := os.MkdirAll(pathName, os.ModePerm); err != nil {
		return err
	}

	// Create the full file path
	fullPath := paths.Filename()

	// Create the file
	f, err := os.Create(fullPath)
	if err != nil {
		return err
	}

	defer f.Close()

	// Copy the buffer into the file
	n, err := io.Copy(f, r)
	if err != nil {
		return err
	}

	log.Printf("wrote %d bytes to disk at: %s", n, fullPath)

	return nil
}
