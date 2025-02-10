package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/rs/zerolog"
	"go-fs/nfs"
	"go-fs/service"
	"golang.org/x/sync/errgroup"
	"io"
	"net/http"
	"regexp"
	"strings"
)

type HttpHandler struct {
	svc *service.HttpService
	Log *zerolog.Logger
}

func NewHttpHandler(svc *service.HttpService, logger *zerolog.Logger) *HttpHandler {
	return &HttpHandler{
		svc: svc,
		Log: logger,
	}
}

var (
	MAX_POST_FILE_SIZE = 1073741824 //1GB
)

func (h *HttpHandler) PostHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println(r.ContentLength)
	r.Body = http.MaxBytesReader(w, r.Body, int64(MAX_POST_FILE_SIZE))
	//enforce multipart
	if !strings.Contains(r.Header.Get("Content-Type"), "multipart/") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"Content-Type must be multipart/form-data"}`))
		return
	}
	//enforce body length
	if r.ContentLength > int64(MAX_POST_FILE_SIZE) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		w.Write([]byte(`{"error":"Content-Length must be less than 1GB"}`))
		return
	}

	//we'd try to sniff the nfs information from the first 512 bytes
	headerBytes := make([]byte, 512)
	_, err := r.Body.Read(headerBytes)
	if err != nil && err != io.EOF {
		panic(err)
	}

	filename, ct, err := multiPartFileInfo(bytes.NewReader(headerBytes))
	if err != nil || filename == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"Cannot handle nfs"}`))
	}

	g, _ := errgroup.WithContext(r.Context())

	g.Go(func() error {
		if err := h.svc.HandleFilePost(&r.Body, filename, ct); err != nil {
			return err
		}
		w.WriteHeader(http.StatusOK)
		return nil
	})

	if err := g.Wait(); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		if strings.Contains(err.Error(), "connection refused") {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error":"` + err.Error() + `"}`))
			return
		}

		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"` + err.Error() + `"}`))
		return
	}

}

func (h *HttpHandler) GetHandler(w http.ResponseWriter, r *http.Request) {
	filename := (r.URL.Query())["filename"]
	if len(filename) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return

	}

	g, _ := errgroup.WithContext(r.Context())

	g.Go(func() error {
		var (
			fd  *nfs.StreamResponseReader
			err error
			srv *nfs.FileServer
		)

		if fd, srv, err = h.svc.HandleFileFetch(filename[0]); err != nil {
			w.Header().Set("Content-Type", "application/json")

			if strings.Contains(err.Error(), "not available") {
				w.WriteHeader(http.StatusNotFound)
				return nil
			}
			return err
		}

		if fd != nil {
			buf := make([]byte, 65536)
			w.Header().Set("Content-Disposition", "attachment; filename="+filename[0])
			w.Header().Set("Content-Type", "application/octet-stream")
			for {
				n, err := fd.Read(buf)
				if err != nil {
					return err
				}

				if n > 0 {

					//we send an end-of-stream byte sequence of length 3, so watch out for that
					if n >= 3 && bytes.Compare(buf[n-3:n], []byte{0xc, 0xc, 0xa}) == 0 {

						w.Write(buf[:n-3])
						srv.Stop() //we're done stop the nfs server
						break
					}

					w.Write(buf[:n])
					buf = make([]byte, 65536)
				}

			}
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		if strings.Contains(err.Error(), "connection refused") {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error":"` + err.Error() + `"}`))
			return
		}

		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"` + err.Error() + `"}`))
		return

	}

}

// multiPartFileInfo try to extract the nfs name and content-type from the nfs bytes
func multiPartFileInfo(b *bytes.Reader) (filename, contentType string, err error) {
	r, err := io.ReadAll(b)
	if err != nil {
		return
	}
	multiPartData := strings.Split(string(r), "\n")

	for _, v := range multiPartData {
		v = strings.TrimSpace(v)
		if strings.Contains(v, "filename") {
			var re = regexp.MustCompile(`[="]`)
			filename = re.ReplaceAllString(strings.Split(v, "filename")[1], "")
		}
		if strings.Contains(v, "Content-Type") {
			contentType = strings.TrimSpace(strings.Split(v, ":")[1])
		}
	}

	filename = strings.Replace(filename, " ", "_", -1)
	return

}
