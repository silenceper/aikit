package web

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed static/*
var staticFS embed.FS

func NewServer(host string, port int) *http.Server {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	sub, _ := fs.Sub(staticFS, "static")
	indexHTML, _ := fs.ReadFile(staticFS, "static/index.html")

	r.GET("/", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
	})

	assetsSub, _ := fs.Sub(sub, "assets")
	r.StaticFS("/assets", http.FS(assetsSub))

	api := r.Group("/api")
	{
		api.GET("/catalog", handleGetCatalog)
		api.POST("/catalog/discover", handleDiscover)
		api.POST("/catalog/add-batch", handleBatchAdd)
		api.POST("/catalog/:kind", handleAddEntry)
		api.PUT("/catalog/:kind/:name", handleUpdateEntry)
		api.DELETE("/catalog/:kind/:name", handleDeleteEntry)
	}

	return &http.Server{
		Addr:    fmt.Sprintf("%s:%d", host, port),
		Handler: r,
	}
}
