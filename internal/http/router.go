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
	rateLimitCfg config.RateLimitConfig,
	logger zerolog.Logger,
	healthHandler handlers.HealthHandler,
	authHandler handlers.AuthHandler,
	dashboardHandler handlers.DashboardHandler,
	blogHandler handlers.BlogHandler,
	projectsHandler handlers.ProjectsHandler,
	generationHandler handlers.GenerationHandler,
	billingHandler handlers.BillingHandler,
	billingWebhookHandler handlers.BillingWebhookHandler,
	settingsHandler handlers.SettingsHandler,
	authService *auth.Service,
	storagePublicPath string,
	storageRootDir string,
) stdhttp.Handler {
	router := chi.NewRouter()
	authMiddleware := newAuthMiddleware(authService)
	authRateLimiter := newRateLimiter(rateLimitCfg)

	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(cfg.RequestTimeout))
	router.Use(requestctx.WithLogger(logger))
	router.Use(requestLogger(logger))
	router.Use(corsMiddleware(cfg.CORSAllowedOrigins))

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
		// Rate limiting применяется только к auth-эндпоинтам: они доступны без токена
		// и являются основной целью brute force и credential stuffing атак.
		api.Route("/auth", func(authRouter chi.Router) {
			authRouter.Use(authRateLimiter.Middleware())

			authRouter.Post("/register", authHandler.Register)
			authRouter.Post("/register/verify", authHandler.VerifyRegister)
			authRouter.Post("/register/resend", authHandler.ResendRegisterCode)
			authRouter.Post("/login", authHandler.Login)
			authRouter.Post("/telegram/login", authHandler.TelegramLogin)
			authRouter.With(authMiddleware.RequireUser).Post("/logout", authHandler.Logout)
			authRouter.Post("/forgot-password", authHandler.ForgotPassword)
			authRouter.Post("/reset-password", authHandler.ResetPassword)
			authRouter.Post("/vk/oauth", authHandler.VKOAuth)
			authRouter.Post("/vk/widget", authHandler.VKWidget)
			authRouter.Post("/yandex/widget", authHandler.YandexWidget)
		})

		// Все маршруты внутри этой группы требуют авторизации через Bearer-токен.
		api.Group(func(protected chi.Router) {
			protected.Use(authMiddleware.RequireUser)

			protected.Get("/me", authHandler.Me)
			protected.Get("/dashboard", dashboardHandler.Get)

			protected.Get("/projects", projectsHandler.List)
			protected.Post("/projects", projectsHandler.Create)
			protected.Get("/projects/{id}", projectsHandler.Get)
			protected.Patch("/projects/{id}", projectsHandler.Patch)
			protected.Delete("/projects/{id}", projectsHandler.Delete)

			protected.Get("/generate/config", generationHandler.GetConfig)
			protected.Post("/uploads/images", generationHandler.UploadImage)
			protected.Post("/generations", generationHandler.CreateGeneration)
			protected.Get("/generations/{id}", generationHandler.GetGenerationStatus)

			protected.Get("/billing", billingHandler.Get)
			protected.Post("/billing/checkout", billingHandler.CreateCheckout)
			protected.Post("/billing/addons", billingHandler.PurchaseAddon)
			protected.Post("/billing/cancel", billingHandler.CancelSubscription)

			protected.Get("/settings", settingsHandler.Get)
			protected.Patch("/settings/profile", settingsHandler.PatchProfile)
			protected.Patch("/settings/defaults", settingsHandler.PatchDefaults)
			protected.Post("/settings/change-password", settingsHandler.ChangePassword)
			protected.Patch("/settings/notifications", settingsHandler.PatchNotifications)
			protected.Delete("/settings/sessions/{id}", settingsHandler.DeleteSession)
			protected.Post("/settings/api-key/rotate", settingsHandler.RotateAPIKey)
			protected.Post("/settings/export", settingsHandler.ExportData)
			protected.Delete("/settings/account", settingsHandler.DeleteAccount)
		})

		// Webhook не требует авторизации пользователя — подпись проверяется внутри handler.
		api.Post("/billing/webhook", billingWebhookHandler.Handle)

		api.Route("/public", func(publicRouter chi.Router) {
			publicRouter.Get("/blog", blogHandler.List)
			publicRouter.Get("/blog/{slug}", blogHandler.GetBySlug)
		})
	})

	registerDocsRoutes(router)

	return router
}
