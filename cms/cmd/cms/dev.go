//go:build dev

package main

import (
	"net/http"
	"os"

	"github.com/filip-be/folk-ms/cms/internal/adapters/devoidc"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

func init() {
	devIdentity = func(string) (ports.IdentityProvider, http.Handler) {
		p := devoidc.New("/auth/dev/login")
		return p, p.Handler(os.Getenv("SUPER_ADMIN_EMAIL"))
	}
}
