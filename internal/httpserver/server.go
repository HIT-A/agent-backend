package httpserver

import (
	"net/http"
)

type Server struct {
	addr    string
	handler http.Handler
}

func New(addr string, opts Options) *Server {
	router := NewRouter(opts)

	handler := RecoveryMiddleware(LoggingMiddleware(router))

	return &Server{addr: addr, handler: handler}
}

func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(s.addr, s.handler)
}
