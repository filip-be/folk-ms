//go:build dev

package main

import (
	"net/http"
	"os"

	"github.com/filip-be/folk-ms/cms/internal/adapters/devoidc"
	"github.com/filip-be/folk-ms/cms/internal/domain"
	httpapi "github.com/filip-be/folk-ms/cms/internal/http"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

func init() {
	devIdentity = func(string) (ports.IdentityProvider, ports.IdentityProvider, http.Handler) {
		p := devoidc.New("/auth/dev/login")
		polish := func(r *http.Request) bool { return httpapi.RequestLang(r) == domain.PL }
		return p, p.Facebook(), p.Handler(os.Getenv("SUPER_ADMIN_EMAIL"), polish)
	}
}
