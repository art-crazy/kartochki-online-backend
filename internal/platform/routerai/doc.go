// Package routerai реализует HTTP-клиент для генерации изображений через RouterAI API.
// API совместим с форматом OpenAI: запрос идёт на /chat/completions с modalities: ["image", "text"],
// изображение возвращается как base64 data URL и декодируется в байты PNG.
// Клиент реализует интерфейс generation.ImageGenerator через адаптер routerAIAdapter в internal/app.
package routerai
