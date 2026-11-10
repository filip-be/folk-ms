//go:build dev

package main

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/filip-be/folk-ms/cms/internal/adapters/devoidc"
	"github.com/filip-be/folk-ms/cms/internal/domain"
	httpapi "github.com/filip-be/folk-ms/cms/internal/http"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// practice is the Autofill practice page; harness.js and fieldmap.json are
// copied in by cms/admin/build.ts.
//
//go:embed all:practice
var practice embed.FS

func init() {
	devIdentity = func(string) (ports.IdentityProvider, http.Handler) {
		p := devoidc.New("/auth/dev/login")
		polish := func(r *http.Request) bool { return httpapi.RequestLang(r) == domain.PL }
		return p, p.Handler(os.Getenv("SUPER_ADMIN_EMAIL"), polish)
	}
	devPages = func() http.Handler {
		sub, _ := fs.Sub(practice, "practice")
		files := http.FileServerFS(sub)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := strings.TrimPrefix(r.URL.Path, "/dev/facebook")
			if strings.HasPrefix(p, "/events/") {
				// The create page and the "created" address both show the form.
				http.ServeFileFS(w, r, sub, "index.html")
				return
			}
			r2 := r.Clone(r.Context())
			r2.URL.Path = p
			files.ServeHTTP(w, r2)
		})
	}
}
