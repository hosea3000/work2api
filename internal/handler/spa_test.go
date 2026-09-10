package handler

import (
	"embed"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

//go:embed testdata
var testDist embed.FS

func TestAssetsRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	sub, _ := fs.Sub(testDist, "testdata")
	fileServer := http.StripPrefix("/assets", http.FileServer(http.FS(sub)))
	e.GET("/assets/*filepath", func(c *gin.Context) {
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
	for _, p := range []string{"/assets/sub/app.js", "/assets/index.html"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", p, nil)
		e.ServeHTTP(w, req)
		t.Logf("%s → %d", p, w.Code)
	}
}
