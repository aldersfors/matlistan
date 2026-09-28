package importer

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/aldersfors/matlistan/internal/safenet"
)

// testFetcher trusts the httptest TLS server and treats 127.0.0.1 as public, so tests
// can reach it; every other rule stays in force.
func testFetcher(t *testing.T, srv *httptest.Server) *safeFetcher {
	t.Helper()
	tr := srv.Client().Transport.(*http.Transport).Clone()
	return newFetcher("Matlistan/test", func(a netip.Addr) bool {
		return a == netip.MustParseAddr("127.0.0.1") || safenet.PublicAddr(a)
	}, tr)
}

func TestFetchReturnsHTML(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.UserAgent(), "Matlistan/") || r.Header.Get("Accept-Language") == "" {
			t.Errorf("headers %v", r.Header)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><body>Köttbullar</body></html>"))
	}))
	defer srv.Close()
	p, err := testFetcher(t, srv).Fetch(context.Background(), srv.URL+"/recept")
	if err != nil || !strings.Contains(string(p.HTML), "Köttbullar") || p.URL != srv.URL+"/recept" {
		t.Fatalf("page = %+v, %v", p, err)
	}
}

// Review focus 1: nothing internal, whatever the name resolves to.
func TestFetchRefusesPrivateTargets(t *testing.T) {
	f := NewFetcher("Matlistan/test").(*safeFetcher)
	for _, u := range []string{"https://127.0.0.1/", "https://10.1.2.3/", "https://192.168.1.1/",
		"https://172.16.0.1/", "https://100.64.0.1/", "https://169.254.169.254/latest/meta-data",
		"https://[::1]/", "https://[fd00::1]/", "https://[fe80::1]/", "https://0.0.0.0/",
		"https://matlistan-db-rw.matlistan.svc/", "https://kubernetes.default.svc.cluster.local/",
		"https://nas.local/", "https://router.example.lan/", "https://localhost/"} {
		if _, err := f.Fetch(context.Background(), u); !errors.Is(err, ErrNotAllowed) {
			t.Errorf("%s: err = %v, want ErrNotAllowed", u, err)
		}
	}
}

func TestFetchRefusesRedirectToPrivate(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://10.0.0.5/secret", http.StatusFound)
	}))
	defer srv.Close()
	if _, err := testFetcher(t, srv).Fetch(context.Background(), srv.URL); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("err = %v, want ErrNotAllowed", err)
	}
}

func TestFetchLimits(t *testing.T) {
	big := strings.Repeat("a", 3<<20)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/big":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(big))
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		case "/gone":
			http.NotFound(w, r)
		case "/slow":
			time.Sleep(300 * time.Millisecond)
		}
	}))
	defer srv.Close()
	f := testFetcher(t, srv)
	f.timeout = 100 * time.Millisecond
	for path, want := range map[string]error{"/big": ErrTooLarge, "/json": ErrNotHTML,
		"/loop": ErrUnreachable, "/gone": ErrUnreachable, "/slow": ErrUnreachable} {
		if _, err := f.Fetch(context.Background(), srv.URL+path); !errors.Is(err, want) {
			t.Errorf("%s: err = %v, want %v", path, err, want)
		}
	}
}

func TestFetchURLRules(t *testing.T) {
	f := NewFetcher("Matlistan/test")
	for u, want := range map[string]error{"not a url": ErrInvalidURL, "ftp://x.se/r": ErrNotHTTPS,
		"https://user:pw@www.ica.se/": ErrInvalidURL} {
		if _, err := f.Fetch(context.Background(), u); !errors.Is(err, want) {
			t.Errorf("%q: err = %v, want %v", u, err, want)
		}
	}
}

// An http link is tried once as https.
func TestFetchUpgradesHTTP(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>ok</html>"))
	}))
	defer srv.Close()
	httpURL := "http://" + strings.TrimPrefix(srv.URL, "https://") + "/r"
	p, err := testFetcher(t, srv).Fetch(context.Background(), httpURL)
	if err != nil || !strings.HasPrefix(p.URL, "https://") {
		t.Fatalf("page = %+v, %v", p, err)
	}
}
