package main

import (
	"bytes"
	"embed"
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

//go:embed all:frontend/dist
var frontendFS embed.FS

// staticHandler serves the embedded Vite build with an SPA fallback.
//
// Requests under /api/, /auth/, /media/, /tiles/ and /webhooks/ never reach this
// handler: those routes are registered before the catch-all. A path that looks
// like a static asset (it carries an extension) and is missing returns 404
// instead of the SPA shell, so a broken asset URL is loud rather than silent.
func staticHandler() (echo.HandlerFunc, error) {
	dist, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		return nil, err
	}
	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		return nil, err
	}
	files := http.FileServerFS(dist)

	return func(c echo.Context) error {
		name := path.Clean("/" + c.Param("*"))
		name = strings.TrimPrefix(name, "/")
		if name == "" || name == "." {
			name = "index.html"
		}

		if _, err := fs.Stat(dist, name); err == nil {
			c.Response().Header().Set("Cache-Control", cacheControl(name))
			files.ServeHTTP(c.Response(), c.Request())
			return nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}

		if path.Ext(name) != "" {
			return echo.NotFoundHandler(c)
		}

		c.Response().Header().Set("Cache-Control", "no-cache")
		c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(c.Response(), c.Request(), "index.html", time.Time{}, bytes.NewReader(index))
		return nil
	}, nil
}

// cacheControl marks Vite's content-hashed asset directory immutable and
// everything else revalidated.
func cacheControl(name string) string {
	if strings.HasPrefix(name, "assets/") {
		return "public, max-age=31536000, immutable"
	}
	return "no-cache"
}
