package httpserver

import (
	"io"
	"net/http"
)

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, "ok") //nolint:errcheck
}
