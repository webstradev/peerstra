package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
)

const (
	// DefaultRoot is the default root directory where the files will be stored
	DefaultRoot = "data"
)

type PathTransformFunc func(key string) PathKey

type PathKey struct {
	PathName string
	FileName string
}

func (p *PathKey) FilePath() string {
	return filepath.Join(p.PathName, p.FileName)
}

var DefaultPathTransformFunc = func(key string) PathKey {
	return PathKey{
		PathName: key,
		FileName: key,
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
		FileName: hashStr,
	}
}

type StoreOpts struct {
	// Root is the root directory where the files will be stored
	Root              string
	PathTransformFunc PathTransformFunc
}

type Store struct {
	StoreOpts
}

func NewStore(opts StoreOpts) *Store {
	if opts.PathTransformFunc == nil {
		opts.PathTransformFunc = DefaultPathTransformFunc
	}

	if opts.Root == "" {
		opts.Root = DefaultRoot
	}

	return &Store{
		StoreOpts: opts,
	}
}

func (s *Store) fullPathWithRoot(key string) string {
	pathKey := s.PathTransformFunc(key)
	return filepath.Join(s.Root, pathKey.FilePath())
}

func (s *Store) Clear() error {
	return os.RemoveAll(s.Root)
}

func (s *Store) Has(key string) bool {
	_, err := os.Stat(s.fullPathWithRoot(key))
	return !errors.Is(err, fs.ErrNotExist)
}

func (s *Store) Delete(key string) error {
	fullPath := s.fullPathWithRoot(key)

	defer func() {
		log.Printf("deleted [%s] from disk", fullPath)
	}()

	return os.RemoveAll(fullPath)
}

func (s *Store) Read(key string) (io.Reader, error) {
	f, err := s.readStream(key)
	if err != nil {
		return nil, err
	}

	defer f.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, f)

	return buf, err
}

func (s *Store) Write(key string, r io.Reader) (int64, error) {
	return s.writeStream(key, r)
}

func (s *Store) readStream(key string) (io.ReadCloser, error) {
	return os.Open(s.fullPathWithRoot(key))
}

func (s *Store) writeStream(key string, r io.Reader) (int64, error) {
	pathKey := s.PathTransformFunc(key)
	pathName := filepath.Join(s.Root, pathKey.PathName)

	// Create necessary directories
	if err := os.MkdirAll(pathName, os.ModePerm); err != nil {
		return 0, err
	}

	// Create the full file path
	fullPath := filepath.Join(pathName, pathKey.FileName)

	// Create the file
	f, err := os.Create(fullPath)
	if err != nil {
		return 0, err
	}

	defer f.Close()

	// Copy the buffer into the file
	n, err := io.Copy(f, r)
	if err != nil {
		return 0, err
	}

	log.Printf("wrote %d bytes to disk at: %s", n, fullPath)

	return n, nil
}
