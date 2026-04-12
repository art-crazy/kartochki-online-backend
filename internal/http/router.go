package http

import (
	stdhttp "net/http"
	"os"

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
	projectsHandler handlers.ProjectsHandler,
	generationHandler handlers.GenerationHandler,
	settingsHandler handlers.SettingsHandler,
	authService *auth.Service,
	storagePublicPath string,
	storageRootDir string,
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
	router.Handle(storagePublicPath+"/*", stdhttp.StripPrefix(storagePublicPath+"/", stdhttp.FileServer(stdhttp.FS(os.DirFS(storageRootDir)))))

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
		api.With(authMiddleware.RequireUser).Get("/projects", projectsHandler.List)
		api.With(authMiddleware.RequireUser).Post("/projects", projectsHandler.Create)
		api.With(authMiddleware.RequireUser).Get("/projects/{id}", projectsHandler.Get)
		api.With(authMiddleware.RequireUser).Patch("/projects/{id}", projectsHandler.Patch)
		api.With(authMiddleware.RequireUser).Delete("/projects/{id}", projectsHandler.Delete)
		api.With(authMiddleware.RequireUser).Get("/generate/config", generationHandler.GetConfig)
		api.With(authMiddleware.RequireUser).Post("/uploads/images", generationHandler.UploadImage)
		api.With(authMiddleware.RequireUser).Post("/generations", generationHandler.CreateGeneration)
		api.With(authMiddleware.RequireUser).Get("/generations/{id}", generationHandler.GetGenerationStatus)
		api.With(authMiddleware.RequireUser).Get("/settings", settingsHandler.Get)
		api.With(authMiddleware.RequireUser).Patch("/settings/profile", settingsHandler.PatchProfile)
		api.With(authMiddleware.RequireUser).Patch("/settings/defaults", settingsHandler.PatchDefaults)
		api.With(authMiddleware.RequireUser).Post("/settings/change-password", settingsHandler.ChangePassword)
		api.With(authMiddleware.RequireUser).Patch("/settings/notifications", settingsHandler.PatchNotifications)
		api.With(authMiddleware.RequireUser).Delete("/settings/sessions/{id}", settingsHandler.DeleteSession)
		api.With(authMiddleware.RequireUser).Post("/settings/api-key/rotate", settingsHandler.RotateAPIKey)
		api.With(authMiddleware.RequireUser).Post("/settings/export", settingsHandler.ExportData)
		api.With(authMiddleware.RequireUser).Delete("/settings/account", settingsHandler.DeleteAccount)
	})

	registerDocsRoutes(router)

	return router
}
