package httpserver

import (
	"context"
	"net"
	"net/http"

	"kartochki-online-backend/internal/config"
)

// Server оборачивает net/http.Server и хранит вычисленный адрес запуска.
type Server struct {
	httpServer *http.Server
	address    string
}

// New создаёт HTTP-сервер из конфигурации приложения.
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

// Start запускает HTTP-сервер.
func (s *Server) Start() error {
	return s.httpServer.ListenAndServe()
}

// Shutdown останавливает HTTP-сервер с ожиданием активных запросов.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// Address возвращает адрес, на котором слушает сервер.
func (s *Server) Address() string {
	return s.address
}
