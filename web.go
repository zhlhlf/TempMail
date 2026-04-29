package main

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed frontend/index.html frontend/css/style.css frontend/js/app.js
var embeddedFrontend embed.FS

func registerFrontendRoutes(r *gin.Engine) {
	frontendFS, err := fs.Sub(embeddedFrontend, "frontend")
	if err != nil {
		panic(err)
	}

	cssServer := http.StripPrefix("/css/", http.FileServer(http.FS(mustSubFS("frontend/css"))))
	jsServer := http.StripPrefix("/js/", http.FileServer(http.FS(mustSubFS("frontend/js"))))
	_ = frontendFS

	r.GET("/", serveIndex)
	r.GET("/index.html", serveIndex)
	r.GET("/css/*filepath", staticFileHandler(cssServer, true))
	r.GET("/js/*filepath", staticFileHandler(jsServer, true))
}

func serveIndex(c *gin.Context) {
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	c.Header("X-Content-Type-Options", "nosniff")
	data, err := embeddedFrontend.ReadFile("frontend/index.html")
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}

func staticFileHandler(fileServer http.Handler, cache bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if cache {
			c.Header("Cache-Control", "public, max-age=604800, immutable")
		}
		c.Header("X-Content-Type-Options", "nosniff")
		fileServer.ServeHTTP(c.Writer, c.Request)
	}
}

func mustSubFS(dir string) fs.FS {
	sub, err := fs.Sub(embeddedFrontend, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
