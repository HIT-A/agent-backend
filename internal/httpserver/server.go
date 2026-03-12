package httpserver

import (
	"net/http"
)

type Server struct {
	addr    string
	handler http.Handler
}

func New(addr string, opts Options) *Server {
	return &Server{addr: addr, handler: NewRouter(opts)}
}

func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(s.addr, s.handler)
}
