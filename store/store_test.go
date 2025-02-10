package store

import (
	"bytes"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"io"
	"os"
	"testing"
)

var Log = zerolog.New(os.Stderr).With().Timestamp().Logger()

func TestStore_Delete(t *testing.T) {
	opts := StoreOpts{
		PathTransformFunc: DefaultPathTransformFunc,
	}
	s := NewStore(opts, &Log)

	key := "speacialfile/speacialfile"
	data := []byte("some binary stuff")

	if err := s.writeStream(key, bytes.NewBuffer(data)); err != nil {
		t.Error(err)
	}

	if err := s.Delete(key); err != nil {
		t.Error(err)

		_, err := os.Stat(key)
		assert.NotNil(t, err)
	}
}

func TestStore(t *testing.T) {
	opts := StoreOpts{
		PathTransformFunc: DefaultPathTransformFunc,
	}
	s := NewStore(opts, &Log)

	key := "speacialfile/speacialfile"
	data := []byte("some binary stuff")

	if err := s.writeStream(key, bytes.NewBuffer(data)); err != nil {
		t.Error(err)
	}

	r, _, err := s.OpenFd(key)
	if err != nil {
		t.Error(err)
	}

	b, _ := io.ReadAll(r)

	if string(b) != string(data) {
		t.Errorf("want %s, got %s", string(data), string((b)))
	}
	if err := s.Delete(key + "/" + key); err != nil {
		t.Error(err)

		_, err := os.Stat(key)
		assert.NotNil(t, err)
	}

}
