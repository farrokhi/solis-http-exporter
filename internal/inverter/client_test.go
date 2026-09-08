package inverter

import (
	"cmp"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testPassword = "hunter2-wifi-password"

func clientFor(t *testing.T, srv *httptest.Server, timeout time.Duration) *Client {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	return New(Options{
		Scheme:   "http",
		Address:  u.Hostname(),
		Port:     port,
		Path:     "/inverter.cgi",
		Username: "admin",
		Password: testPassword,
		Timeout:  timeout,
	})
}

func TestFetchAuthenticates(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		user, pass, ok := r.BasicAuth()
		if !ok || user != "admin" || pass != testPassword {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write(load(t, "inverter-normal.txt"))
	}))
	defer srv.Close()

	status, err := clientFor(t, srv, time.Second).Fetch(t.Context())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if gotPath != "/inverter.cgi" {
		t.Errorf("path = %q", gotPath)
	}
	if status.Serial != "1802020228090133" || *status.PowerWatts != 900 {
		t.Errorf("unexpected status %+v", status)
	}
}

func TestFetchErrors(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		timeout time.Duration
		want    string
	}{
		{
			name:    "unauthorized",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) },
			want:    "401",
		},
		{
			name:    "server error",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
			want:    "500",
		},
		{
			name: "redirect not followed",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "http://example.com/", http.StatusFound)
			},
			want: "302",
		},
		{
			name: "oversized",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte(strings.Repeat("x", 8<<10)))
			},
			want: "larger than",
		},
		{
			name: "timeout",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(300 * time.Millisecond)
			},
			timeout: 50 * time.Millisecond,
			want:    "context deadline exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			_, err := clientFor(t, srv, cmp.Or(tt.timeout, time.Second)).Fetch(t.Context())
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestFetchErrorsHideCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	srv.Close() // connection refused, the noisiest error shape

	err := func() error {
		_, err := clientFor(t, srv, time.Second).Fetch(t.Context())
		return err
	}()
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), testPassword) || strings.Contains(strings.ToLower(err.Error()), "authorization") {
		t.Errorf("error leaks credentials: %v", err)
	}
}
