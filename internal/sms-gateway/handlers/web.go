package handlers

import (
	"net/http"
	"strings"

	"github.com/android-sms-gateway/server/internal/webui"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
)

type webHandler struct{}

func newWebHandler() *webHandler {
	return &webHandler{}
}

func (h *webHandler) Register(app *fiber.App) {
	distFS, err := webui.DistFS()
	if err != nil {
		return
	}

	// SPA handler: serve static files, fallback to index.html
	app.Use(func(c *fiber.Ctx) error {
		path := c.Path()

		// Let API and health routes pass through
		if strings.HasPrefix(path, "/api/") ||
			strings.HasPrefix(path, "/health") ||
			strings.HasPrefix(path, "/3rdparty/") ||
			strings.HasPrefix(path, "/mobile/") ||
			strings.HasPrefix(path, "/upstream/") {
			return c.Next()
		}

		// Try serving static file from dist/
		fsHandler := filesystem.New(filesystem.Config{
			Root:         http.FS(distFS),
			Browse:       false,
			NotFoundFile: "index.html",
		})
		return fsHandler(c)
	})
}
