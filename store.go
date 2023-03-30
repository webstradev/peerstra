package main

import (
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

func (s *Store) Read(key string) (int64, io.Reader, error) {
	return s.readStream(key)
}

func (s *Store) Write(key string, r io.Reader) (int64, error) {
	return s.writeStream(key, r)
}

func (s *Store) WriteDecrypt(encKey []byte, key string, r io.Reader) (int64, error) {
	f, err := s.openFileForWriting(key)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	n, err := copyDecrypt(encKey, r, f)
	return int64(n), err
}

func (s *Store) writeStream(key string, r io.Reader) (int64, error) {
	f, err := s.openFileForWriting(key)
	if err != nil {
		return 0, err
	}

	defer f.Close()

	// Copy the buffer into the file
	return io.Copy(f, r)
}

func (s *Store) openFileForWriting(key string) (*os.File, error) {
	pathKey := s.PathTransformFunc(key)
	pathName := filepath.Join(s.Root, pathKey.PathName)

	// Create necessary directories
	if err := os.MkdirAll(pathName, os.ModePerm); err != nil {
		return nil, err
	}

	// Create the full file path
	fullPath := filepath.Join(pathName, pathKey.FileName)

	// Create the file and return
	return os.Create(fullPath)
}

func (s *Store) readStream(key string) (int64, io.ReadCloser, error) {
	f, err := os.Open(s.fullPathWithRoot(key))
	if err != nil {
		return 0, nil, err
	}

	fi, err := f.Stat()
	if err != nil {
		return 0, nil, err
	}

	return fi.Size(), f, nil
}
