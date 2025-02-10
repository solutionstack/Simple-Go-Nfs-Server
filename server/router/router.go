package router

import (
	"go-fs/server/handlers"
	"net/http"
)

func RouterMux(handlers *handlers.HttpHandler) *http.ServeMux {
	var Mux = http.NewServeMux()

	Mux.Handle("POST /", http.HandlerFunc(handlers.PostHandler))
	Mux.Handle("GET /byname", http.HandlerFunc(handlers.GetHandler))
	Mux.Handle("/", http.NotFoundHandler())

	return Mux
}
