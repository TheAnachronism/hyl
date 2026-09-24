package server

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/markbeep/hyl/internal/apperr"
)

// tilesHandler serves the operator-supplied .pmtiles basemap. pmtiles issues
// plain `Range: bytes=off-end` GETs and reads ETag/Cache-Control, so the file
// goes through http.ServeContent untouched — no compression middleware may be
// registered on this route, because a Content-Encoding writer would break
// ServeContent's fixed-length contract.
func tilesHandler(tilesDir string) echo.HandlerFunc {
	return func(c echo.Context) error {
		name := c.Param("name")
		if name == "" || strings.ContainsAny(name, `/\`) || !strings.HasSuffix(name, ".pmtiles") {
			return apperr.NotFound("no such tile file")
		}

		file, err := os.Open(filepath.Join(tilesDir, name))
		if errors.Is(err, fs.ErrNotExist) {
			return apperr.NotFound("no such tile file")
		}
		if err != nil {
			return err
		}
		defer func() { _ = file.Close() }()

		info, err := file.Stat()
		if err != nil {
			return err
		}
		if info.IsDir() {
			return apperr.NotFound("no such tile file")
		}

		header := c.Response().Header()
		header.Set("Content-Type", "application/octet-stream")
		header.Set("ETag", fmt.Sprintf("%q",
			strconv.FormatInt(info.ModTime().UnixNano(), 16)+"-"+strconv.FormatInt(info.Size(), 16)))
		header.Set("Cache-Control", "public, max-age=86400, immutable")

		http.ServeContent(c.Response(), c.Request(), name, info.ModTime(), file)
		return nil
	}
}
