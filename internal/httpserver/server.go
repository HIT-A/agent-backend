package httpserver

import (
	"net/http"
)

type Server struct {
	addr    string
	handler http.Handler
}

func New(addr string) *Server {
	return &Server{addr: addr, handler: NewRouter()}
}

func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(s.addr, s.handler)
}
