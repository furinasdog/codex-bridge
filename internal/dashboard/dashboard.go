package dashboard

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed dist/*
var files embed.FS

func Register(router *gin.Engine) {
	dist, err := fs.Sub(files, "dist")
	if err != nil {
		panic(err)
	}
	assets, err := fs.Sub(dist, "assets")
	if err != nil {
		panic(err)
	}
	router.StaticFS("/assets", http.FS(assets))
	router.GET("/", serveFile(dist, "index.html", "text/html; charset=utf-8"))
	router.GET("/favicon.svg", serveFile(dist, "favicon.svg", "image/svg+xml"))
}

func serveFile(fileSystem fs.FS, name, contentType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := fs.ReadFile(fileSystem, name)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Data(http.StatusOK, contentType, data)
	}
}
