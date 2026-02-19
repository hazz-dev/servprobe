package dashboard

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed assets
var assets embed.FS

// Handler returns an HTTP handler that serves the embedded dashboard assets.
// index.html is served at /, other assets (style.css, app.js) at their paths.
func Handler() http.Handler {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		// Unreachable — "assets" is always present as it is embedded.
		panic(err)
	}
	return http.FileServer(http.FS(sub))
}
