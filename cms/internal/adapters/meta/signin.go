package meta

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// Issuer identifies Facebook sign-in identities (app-scoped user ids).
const Issuer = "https://www.facebook.com"

// SignInScopes are requested when Facebook is used to sign in to the CMS.
const SignInScopes = "email,public_profile"

// SignIn implements ports.IdentityProvider with Facebook Login on the same
// Meta app as posting: the authorization code flow with state, the code
// redeemed with the app secret, then the user's id, name and email from
// Graph. The user token is dropped at once. The PKCE parameters are sent too,
// though Meta documents enforcing them only for its OIDC variant (scope
// openid), which this flow does not use; so there is no ID token or nonce.
type SignIn struct{ C *Client }

// AuthCodeURL implements ports.IdentityProvider. auth_type=rerequest asks
// again for the email if it was declined before.
func (s SignIn) AuthCodeURL(_ context.Context, state, _, challenge, redirectURI string) (string, error) {
	q := url.Values{
		"client_id": {s.C.AppID}, "redirect_uri": {redirectURI}, "state": {state},
		"response_type": {"code"}, "scope": {SignInScopes}, "auth_type": {"rerequest"},
		"code_challenge": {challenge}, "code_challenge_method": {"S256"},
	}
	return s.C.DialogURL + "/" + s.C.Version + "/dialog/oauth?" + q.Encode(), nil
}

// Exchange implements ports.IdentityProvider. Meta returns the email only
// when the account has a valid primary email; it carries no verified flag,
// so the identity is marked NoSuperAdmin.
func (s SignIn) Exchange(ctx context.Context, code, verifier, redirectURI, _ string) (ports.Identity, error) {
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := s.C.call(ctx, http.MethodGet, "/oauth/access_token", url.Values{
		"client_id": {s.C.AppID}, "client_secret": {s.C.AppSecret}, "redirect_uri": {redirectURI},
		"code": {code}, "code_verifier": {verifier},
	}, "", &tok); err != nil {
		return ports.Identity{}, err
	}
	if tok.AccessToken == "" {
		return ports.Identity{}, errors.New("meta: no access token in the login answer")
	}
	var me struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := s.C.call(ctx, http.MethodGet, "/me", url.Values{"fields": {"id,name,email"}}, tok.AccessToken, &me); err != nil {
		return ports.Identity{}, err
	}
	if me.ID == "" {
		return ports.Identity{}, errors.New("meta: no user id")
	}
	return ports.Identity{Issuer: Issuer, Subject: me.ID, Email: me.Email, EmailVerified: me.Email != "",
		Name: me.Name, NoSuperAdmin: true}, nil
}
