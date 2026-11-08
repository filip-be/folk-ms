// Command cms is the folk-ms CMS: one binary that runs as an Azure Functions
// custom handler (FUNCTIONS_CUSTOMHANDLER_PORT) or as a plain server. It is
// the only place that chooses adapters.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // Europe/Warsaw must not depend on the host image

	"github.com/filip-be/folk-ms/cms/admin"
	"github.com/filip-be/folk-ms/cms/internal/adapters/azblob"
	"github.com/filip-be/folk-ms/cms/internal/adapters/fs"
	"github.com/filip-be/folk-ms/cms/internal/adapters/google"
	"github.com/filip-be/folk-ms/cms/internal/adapters/img"
	"github.com/filip-be/folk-ms/cms/internal/adapters/meta"
	"github.com/filip-be/folk-ms/cms/internal/app"
	"github.com/filip-be/folk-ms/cms/internal/config"
	httpapi "github.com/filip-be/folk-ms/cms/internal/http"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// devIdentity is set by dev.go in builds with the "dev" tag.
var devIdentity func(baseURL string) (ports.IdentityProvider, http.Handler)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	// No default: a production app missing the setting must not silently
	// fall back to local disk storage.
	mode := os.Getenv("FOLKMS_ENV")
	if mode != "local" && mode != "azure" {
		return fmt.Errorf("FOLKMS_ENV must be set to local or azure")
	}
	port, underFunctions := os.LookupEnv("FUNCTIONS_CUSTOMHANDLER_PORT")
	if !underFunctions {
		port = env("PORT", "8080")
	}
	baseURL := strings.TrimRight(env("BASE_URL", "http://localhost:"+port), "/")
	sitesPath := env("SITES_CONFIG", defaultSitesPath())
	orgs, err := config.LoadSites(sitesPath)
	if err != nil {
		return err
	}
	a := app.New(orgs)
	a.Log = log
	a.Images = img.Processor{}
	a.SuperAdminEmail = os.Getenv("SUPER_ADMIN_EMAIL")

	srv := &httpapi.Server{App: a, BaseURL: baseURL, Admin: admin.FS(), Log: log}

	var sessionKey []byte
	switch mode {
	case "azure":
		if err := wireAzure(a, srv); err != nil {
			return err
		}
		if sessionKey, err = keyFromEnv("SESSION_KEY", 32); err != nil {
			return err
		}
		srv.Secure, srv.TrustProxy = true, true
	case "local":
		dataDir := env("DATA_DIR", ".data")
		if sessionKey, err = localKey(filepath.Join(dataDir, "session.key")); err != nil {
			return err
		}
		if err := wireLocal(a, srv, dataDir, baseURL); err != nil {
			return err
		}
		srv.Secure = strings.HasPrefix(baseURL, "https://")
	}
	if a.SuperAdminEmail == "" {
		log.Warn("SUPER_ADMIN_EMAIL is not set: nobody can administer access")
	}

	if id := os.Getenv("GOOGLE_CLIENT_ID"); id != "" {
		srv.IdP = google.New(id, os.Getenv("GOOGLE_CLIENT_SECRET"))
	} else if mode == "local" && devIdentity != nil {
		idp, h := devIdentity(baseURL)
		srv.IdP, srv.DevLogin = idp, h
		log.Warn("using the dev OIDC stub: any email can sign in")
	} else {
		return errors.New("GOOGLE_CLIENT_ID is required (or build with -tags dev for local sign-in)")
	}

	if id := os.Getenv("META_APP_ID"); id != "" {
		a.Social = meta.New(id, os.Getenv("META_APP_SECRET"), os.Getenv("META_GRAPH_VERSION"))
	} else if mode == "local" && devIdentity != nil {
		// Dev builds only: fake successes must never reach a real deployment.
		a.Social = &meta.Recorder{}
		log.Warn("META_APP_ID not set: Social Posts are recorded, not sent")
	} else {
		return errors.New("META_APP_ID is required (or build with -tags dev to record posts locally)")
	}

	if _, err := httpapi.NewServer(srv, sessionKey); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		// Make sure every Site has a snapshot (also repairs a lost one).
		rctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		if err := a.RebuildAll(rctx); err != nil {
			log.Error("initial snapshot rebuild", "err", err)
		}
	}()

	hs := &http.Server{
		Addr: ":" + port, Handler: srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 60 * time.Second, WriteTimeout: 120 * time.Second, IdleTimeout: 120 * time.Second,
		MaxHeaderBytes: 64 << 10,
	}
	errc := make(chan error, 1)
	go func() { errc <- hs.ListenAndServe() }()
	log.Info("listening", "port", port, "mode", mode, "functions", underFunctions, "baseURL", baseURL)
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return hs.Shutdown(sctx)
}

// defaultSitesPath finds config/sites.json next to the binary (deployment
// package) or in the repository (local runs from cms/).
func defaultSitesPath() string {
	for _, p := range []string{"sites.json", "../config/sites.json", "config/sites.json"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "sites.json")
	}
	return "sites.json"
}

func keyFromEnv(name string, min int) ([]byte, error) {
	k, err := base64.StdEncoding.DecodeString(os.Getenv(name))
	if err != nil || len(k) < min {
		return nil, fmt.Errorf("%s must be a base64 key of at least %d bytes", name, min)
	}
	return k, nil
}

// localKey loads or creates a random key file (local development only).
func localKey(path string) ([]byte, error) {
	if b, err := os.ReadFile(path); err == nil {
		return base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	}
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	return k, os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(k)), 0o600)
}

func wireLocal(a *app.App, srv *httpapi.Server, dataDir, baseURL string) error {
	if blobURL := os.Getenv("STORAGE_BLOB_URL"); blobURL != "" {
		// Local run against Azurite.
		return wireBlob(a, srv, blobURL)
	}
	pubDir := filepath.Join(dataDir, "pub")
	a.Docs = fs.NewDocumentStore(filepath.Join(dataDir, "cms"))
	a.Pub = fs.NewPublicStore(pubDir, baseURL+"/pub")
	a.Audit = fs.NewAppendLog(filepath.Join(dataDir, "audit"))
	srv.PublicDir = pubDir
	return nil
}

func wireAzure(a *app.App, srv *httpapi.Server) error {
	blobURL := os.Getenv("STORAGE_BLOB_URL")
	if blobURL == "" {
		return errors.New("STORAGE_BLOB_URL is required")
	}
	return wireBlob(a, srv, blobURL)
}

func wireBlob(a *app.App, srv *httpapi.Server, blobURL string) error {
	var auth azblob.Auth
	if key := os.Getenv("STORAGE_ACCOUNT_KEY"); key != "" {
		sk, err := azblob.NewSharedKey(env("STORAGE_ACCOUNT_NAME", "devstoreaccount1"), key)
		if err != nil {
			return err
		}
		auth = sk
	} else {
		ep, hdr := os.Getenv("IDENTITY_ENDPOINT"), os.Getenv("IDENTITY_HEADER")
		if ep == "" || hdr == "" {
			return errors.New("no STORAGE_ACCOUNT_KEY and no managed identity (IDENTITY_ENDPOINT)")
		}
		auth = &azblob.ManagedIdentity{Endpoint: ep, Header: hdr, ClientID: os.Getenv("STORAGE_IDENTITY_CLIENT_ID")}
	}
	client := azblob.NewClient(blobURL, auth)
	pubURL := env("STORAGE_PUBLIC_URL", client.BaseURL+"/pub")
	if os.Getenv("STORAGE_CREATE_CONTAINERS") == "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := client.CreateContainer(ctx, "cms", false); err != nil {
			return err
		}
		if err := client.CreateContainer(ctx, "pub", true); err != nil {
			return err
		}
	}
	a.Docs = azblob.NewDocumentStore(client, "cms", "data/")
	a.Pub = azblob.NewPublicStore(client, "pub", pubURL)
	a.Audit = azblob.NewAppendLog(client, "cms", "audit/")
	if u, err := url.Parse(pubURL); err == nil {
		srv.PublicOrigin = u.Scheme + "://" + u.Host
	}
	return nil
}
