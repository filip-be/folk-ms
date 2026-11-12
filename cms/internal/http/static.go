package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Go's built-in MIME table lacks web fonts; the system table may lack them too.
func init() { _ = mime.AddExtensionType(".woff2", "font/woff2") }

type asset struct {
	data []byte
	etag string
	ct   string
}

// staticFS serves the admin SPA under /app/: known files by name, every
// other path as index.html (client-side routing).
type staticFS struct{ files map[string]asset }

func newStaticFS(fsys fs.FS) (*staticFS, error) {
	s := &staticFS{files: map[string]asset{}}
	if fsys == nil {
		return s, nil
	}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasPrefix(path.Base(p), ".") {
			return err
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		ct := mime.TypeByExtension(path.Ext(p))
		if ct == "" {
			ct = "application/octet-stream"
		}
		s.files[p] = asset{data: data, etag: `"` + hex.EncodeToString(sum[:8]) + `"`, ct: ct}
		return nil
	})
	return s, err
}

func (s *staticFS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/app/")
	a, ok := s.files[name]
	if !ok {
		if path.Ext(name) != "" && name != "" {
			http.NotFound(w, r)
			return
		}
		a, ok = s.files["index.html"]
		if !ok {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("Admin SPA is not built: run `bun run build` in cms/admin.\n"))
			return
		}
	}
	h := w.Header()
	h.Set("Content-Type", a.ct)
	h.Set("ETag", a.etag)
	h.Set("Cache-Control", "no-cache")
	if r.Header.Get("If-None-Match") == a.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = w.Write(a.data)
}

// publicFiles serves the local public store (snapshots and media) with the
// cache headers and CORS that the blob container has in Azure.
func (s *Server) publicFiles() http.Handler {
	origins := map[string]bool{}
	for _, o := range s.App.Orgs() {
		for _, origin := range o.Site.Origins {
			origins[origin] = true
		}
	}
	fsrv := http.StripPrefix("/pub/", http.FileServer(noDirs{http.Dir(s.PublicDir)}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/pub/")
		if slices.Contains(strings.Split(p, "/"), "..") {
			http.NotFound(w, r)
			return
		}
		if o := r.Header.Get("Origin"); origins[o] {
			w.Header().Set("Access-Control-Allow-Origin", o)
			w.Header().Set("Vary", "Origin")
		}
		if strings.HasPrefix(p, "media/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=60")
		}
		fsrv.ServeHTTP(w, r)
	})
}

// noDirs hides directory listings and lock/temp files.
type noDirs struct{ http.FileSystem }

func (n noDirs) Open(name string) (http.File, error) {
	base := filepath.Base(name)
	if strings.HasSuffix(base, ".lock") || strings.HasPrefix(base, ".tmp-") {
		return nil, os.ErrNotExist
	}
	f, err := n.FileSystem.Open(name)
	if err != nil {
		return nil, err
	}
	if st, err := f.Stat(); err == nil && st.IsDir() {
		f.Close()
		return nil, os.ErrNotExist
	}
	return f, nil
}

// userscriptPlaceholder is replaced with the CMS origin when the Autofill
// userscript is served (autofill/lib.ts, ORIGIN_PLACEHOLDER).
const userscriptPlaceholder = "__FOLKMS_CMS_ORIGIN__"

// userscript serves the Autofill userscript built with the admin SPA, with
// this CMS's origin filled in, so one build serves every deployment.
func (s *Server) userscript(w http.ResponseWriter, r *http.Request) {
	a, ok := s.static.files["autofill.user.js"]
	if !ok {
		http.NotFound(w, r)
		return
	}
	body := strings.ReplaceAll(string(a.data), userscriptPlaceholder, s.BaseURL)
	h := w.Header()
	h.Set("Content-Type", "text/javascript; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(body))
}
