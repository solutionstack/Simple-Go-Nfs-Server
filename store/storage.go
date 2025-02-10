package store

import (
	"errors"
	"fmt"
	"github.com/rs/zerolog"
	"io"
	"os"
	"strings"
	"time"
)

var defaultBasPath = "store-temp-path"
var fdMap map[string]*os.File = map[string]*os.File{}

type PathTransformVFields struct {
	Pathname string
	Filename string
}
type PathTransformFunc func(path, fileKey string) PathTransformVFields

var DefaultPathTransformFunc = func(path, fileKey string) PathTransformVFields {
	//TODO. add more realistic path transformation logic for the path key

	//for now we just remove trailing and prefix slashes
	if strings.HasSuffix(path, "/") {
		path = path[:len(path)-1]
	}
	if strings.HasPrefix(path, "/") {
		path = path[1:]
	}
	return PathTransformVFields{
		Pathname: path,
		Filename: fileKey,
	}
}

type StoreOpts struct {
	PathTransformFunc PathTransformFunc
	BasePath          string
}
type Store struct {
	StoreOpts StoreOpts
	Log       *zerolog.Logger
}

func NewStore(opts StoreOpts, Log *zerolog.Logger) *Store {
	return &Store{
		StoreOpts: opts,
		Log:       Log,
	}
}

func (s *Store) Delete(path string) error {
	return os.RemoveAll(path)
}
func (s *Store) Write(key string, r io.Reader) error {

	return s.writeStream(key, r)
}
func (s *Store) Has(key string) error {
	_, _, err := s.openReadStream(key)
	if err != nil {
		return fmt.Errorf("nfs with key %s not available for reading", key)
	}

	return nil
}
func (s *Store) OpenFd(key string) (*os.File, *os.FileInfo, error) {
	f, fi, err := s.openReadStream(key)
	if err != nil {
		return nil, nil, err
	}

	fdMap[(*fi).Name()] = f
	return f, fi, err
}
func (s *Store) CloseFd(key string) error {
	return (*fdMap[key]).Close()
}
func (s *Store) openReadStream(key string) (*os.File, *os.FileInfo, error) {

	base := s.StoreOpts.BasePath
	if len(base) == 0 {
		base = defaultBasPath
	}
	pathTransformed := s.StoreOpts.PathTransformFunc(base, key)
	pathAndFileName := pathTransformed.Pathname + "/" + pathTransformed.Filename

	file, err := os.Open(pathAndFileName)
	if err != nil {
		return nil, nil, err
	}
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, nil, err
	}

	return file, &fileInfo, nil

}
func (s *Store) writeStream(key string, r io.Reader) error {
	base := s.StoreOpts.BasePath
	if len(base) == 0 {
		base = defaultBasPath
	}
	pathTransformed := s.StoreOpts.PathTransformFunc(base, key)

	fullPathName := fmt.Sprintf("%s", pathTransformed.Pathname)
	fullPathNameAndFileKey := fmt.Sprintf("%s/%s", fullPathName, pathTransformed.Filename)
	if err := os.MkdirAll(fullPathName, os.ModePerm); err != nil {
		return err
	}

	fd, err := os.Create(fullPathNameAndFileKey)
	if err != nil {
		return err
	}
	defer fd.Close()

	var (
		written            = 0
		readZeroCount      = 0
		deadlineErrorCount = 0
	)

	b := make([]byte, 65536)

	for {
		n, err := r.Read(b)
		if n > 0 {
			_, err2 := fd.Write(b[:n])
			if err2 != nil {
				return err2
			}
			written += n
			readZeroCount = 0
		}

		if err == io.EOF {
			break // End of input
		}

		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				deadlineErrorCount++
				if deadlineErrorCount == 2 {
					break
				}
				time.Sleep(200 * time.Millisecond) // Optional delay
				// Handle deadline exceeded if necessary
				continue
			}
			// For other errors, break the loop
			break
		}

		if n == 0 {
			readZeroCount++
			if readZeroCount >= 3 {
				break // Break after 3 consecutive zero reads
			}
			time.Sleep(100 * time.Millisecond) // Optional delay
		}
	}

	if err := fd.Sync(); err != nil {
		return err
	}

	if written == 0 {
		return errors.New("nfs upload failed: 0 write length")
	}
	s.Log.Trace().Msgf("written %d bytes of data to disk; path %s\n", written, fullPathNameAndFileKey)
	return err
}
