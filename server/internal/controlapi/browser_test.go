package controlapi

import (
	"crypto/tls"
	"net/http/httptest"
	"testing"
)

func TestBrowserOrigin(t *testing.T) {
	for _, tc := range []struct {
		name, origin, site string
		tls, want          bool
	}{
		{"http dev", "http://localhost:3000", "same-origin", false, true},
		{"TLS proxy", "https://localhost:3000", "same-origin", false, true},
		{"missing", "", "", false, false},
		{"null", "null", "", false, false},
		{"other port", "http://localhost:3001", "", false, false},
		{"suffix attack", "http://localhost:3000.evil.test", "", false, false},
		{"userinfo", "http://user@localhost:3000", "", false, false},
		{"path", "http://localhost:3000/path", "", false, false},
		{"downgrade", "http://localhost:3000", "", true, false},
		{"cross site", "http://localhost:3000", "cross-site", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "http://localhost:3000/v1/browser/login", nil)
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Sec-Fetch-Site", tc.site)
			if tc.tls {
				r.TLS = &tls.ConnectionState{}
			}
			if got := sameBrowserOrigin(r); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestBrowserLoginRejectsCrossOriginBeforeAuthentication(t *testing.T) {
	router := NewRouter(NewHandler(nil, nil, nil))
	for _, origin := range []string{"", "null", "http://evil.test"} {
		request := httptest.NewRequest("POST", "http://localhost/v1/browser/login", nil)
		request.Header.Set("Origin", origin)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != 403 {
			t.Fatalf("origin %q: status %d", origin, recorder.Code)
		}
	}
}
