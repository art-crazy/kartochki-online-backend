package http

import (
	"io/fs"
	stdhttp "net/http"

	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	openapi "kartochki-online-backend/api/openapi"
)

const (
	openAPIPrefixPath        = "/openapi/"
	openAPISpecPath          = "/openapi/openapi.yaml"
	swaggerIndexPath         = "/swagger/index.html"
	swaggerRoutePath         = "/swagger"
	swaggerTrailingSlashPath = "/swagger/"
	swaggerAssetsPath        = "/swagger/*"
)

// registerDocsRoutes подключает раздачу OpenAPI-файла и Swagger UI.
func registerDocsRoutes(router chi.Router) {
	openapiFS, err := fs.Sub(openapi.Files, ".")
	if err != nil {
		// Если встроенная спецификация недоступна, UI тоже не имеет смысла публиковать.
		return
	}

	// UI читает ту же встроенную спецификацию, которую мы отдаём как исходный YAML.
	router.Handle(openAPIPrefixPath+"*", stdhttp.StripPrefix(openAPIPrefixPath, stdhttp.FileServer(stdhttp.FS(openapiFS))))

	// Отдельный редирект делает адрес короче и избавляет от ручного ввода index.html.
	redirectToSwaggerIndex := func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		stdhttp.Redirect(w, r, swaggerIndexPath, stdhttp.StatusMovedPermanently)
	}

	router.Get(swaggerRoutePath, redirectToSwaggerIndex)
	router.Get(swaggerTrailingSlashPath, redirectToSwaggerIndex)
	router.Get(swaggerAssetsPath, httpSwagger.Handler(
		httpSwagger.URL(openAPISpecPath),
		httpSwagger.DocExpansion("list"),
		httpSwagger.DeepLinking(true),
	))
}
