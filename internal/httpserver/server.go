package httpserver

import (
	"context"
	"net"
	"net/http"

	"kartochki-online-backend/internal/config"
)

type Server struct {
	httpServer *http.Server
	address    string
}

func New(cfg config.HTTPConfig, handler http.Handler) *Server {
	addr := net.JoinHostPort(cfg.Host, cfg.Port)

	return &Server{
		address: addr,
		httpServer: &http.Server{
			Addr:         addr,
			Handler:      handler,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			IdleTimeout:  cfg.IdleTimeout,
		},
	}
}

func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) Address() string {
	return s.address
}

