package http

import (
	stdhttp "net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	"kartochki-online-backend/internal/auth"
	"kartochki-online-backend/internal/config"
	"kartochki-online-backend/internal/http/handlers"
	"kartochki-online-backend/internal/http/requestctx"
	"kartochki-online-backend/internal/http/response"
)

// NewRouter собирает HTTP-маршруты и middleware для публичного API и служебных endpoint.
func NewRouter(
	cfg config.HTTPConfig,
	logger zerolog.Logger,
	healthHandler handlers.HealthHandler,
	authHandler handlers.AuthHandler,
	dashboardHandler handlers.DashboardHandler,
	authService *auth.Service,
) stdhttp.Handler {
	router := chi.NewRouter()
	authMiddleware := newAuthMiddleware(authService)

	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(cfg.RequestTimeout))
	router.Use(requestctx.WithLogger(logger))
	router.Use(requestLogger(logger))

	// Единый fallback нужен заранее, чтобы фронтенд не зависел от разных форматов ошибок.
	router.NotFound(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		response.WriteError(w, r, stdhttp.StatusNotFound, "not_found", "resource not found")
	})

	router.MethodNotAllowed(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		response.WriteError(
			w,
			r,
			stdhttp.StatusMethodNotAllowed,
			"method_not_allowed",
			"method is not allowed for this resource",
		)
	})

	router.Get("/health/live", healthHandler.Live)
	router.Get("/health/ready", healthHandler.Ready)

	router.Route("/api/v1", func(api chi.Router) {
		api.Route("/auth", func(authRouter chi.Router) {
			authRouter.Post("/register", authHandler.Register)
			authRouter.Post("/login", authHandler.Login)
			authRouter.Post("/telegram/login", authHandler.TelegramLogin)
			authRouter.With(authMiddleware.RequireUser).Post("/logout", authHandler.Logout)
			authRouter.Post("/forgot-password", authHandler.ForgotPassword)
			authRouter.Post("/reset-password", authHandler.ResetPassword)
			authRouter.Get("/vk/start", authHandler.VKStart)
			authRouter.Get("/vk/callback", authHandler.VKCallback)
			authRouter.Get("/yandex/start", authHandler.YandexStart)
			authRouter.Get("/yandex/callback", authHandler.YandexCallback)
		})

		api.With(authMiddleware.RequireUser).Get("/me", authHandler.Me)
		api.With(authMiddleware.RequireUser).Get("/dashboard", dashboardHandler.Get)
	})

	registerDocsRoutes(router)

	return router
}
