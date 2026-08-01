package httpapi

import (
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

type staticHandler struct {
	files      fs.FS
	fileServer http.Handler
}

func newStaticHandler(files fs.FS) (*staticHandler, error) {
	if _, err := fs.Stat(files, "index.html"); err != nil {
		return nil, errors.New("httpapi: static filesystem must contain index.html")
	}
	return &staticHandler{
		files:      files,
		fileServer: http.FileServer(http.FS(files)),
	}, nil
}

func (s *staticHandler) serve(response http.ResponseWriter, request *http.Request) bool {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		return false
	}
	cleaned := path.Clean("/" + request.URL.Path)
	if cleaned != request.URL.Path && !(request.URL.Path == "" && cleaned == "/") {
		return false
	}

	fileName := strings.TrimPrefix(cleaned, "/")
	if fileName == "" {
		fileName = "index.html"
	}
	if fs.ValidPath(fileName) {
		if info, err := fs.Stat(s.files, fileName); err == nil && !info.IsDir() {
			if fileName == "index.html" {
				response.Header().Set("Cache-Control", "no-cache")
				cloned := request.Clone(request.Context())
				cloned.URL.Path = "/"
				s.fileServer.ServeHTTP(response, cloned)
				return true
			}
			s.fileServer.ServeHTTP(response, request)
			return true
		}
	}

	// Only extensionless browser navigations receive the SPA shell. Missing
	// assets and JSON/API requests remain real 404s.
	if path.Ext(fileName) != "" || !acceptsHTML(request.Header.Get("Accept")) {
		return false
	}
	response.Header().Set("Cache-Control", "no-cache")
	cloned := request.Clone(request.Context())
	cloned.URL.Path = "/"
	s.fileServer.ServeHTTP(response, cloned)
	return true
}

func acceptsHTML(accept string) bool {
	for _, part := range strings.Split(accept, ",") {
		mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(part))
		if err == nil && (mediaType == "text/html" || mediaType == "application/xhtml+xml") {
			return true
		}
	}
	return false
}
