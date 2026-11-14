package webdav_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bruli-lab/stowmark/internal/infra/webdav"
	"github.com/stretchr/testify/require"
)

func TestParseWebDAVURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		url          string
		expectedBase string
		expectedPath string
		contains     string
	}{
		{name: "with the webdav scheme, then it connects over HTTP", url: "webdav://localhost:18080/backups/stowmark/", expectedBase: "http://localhost:18080", expectedPath: "backups/stowmark"},
		{name: "with the webdavs scheme, then it connects over HTTPS", url: "webdavs://dav.example.com/backups", expectedBase: "https://dav.example.com", expectedPath: "backups"},
		{name: "with the https scheme, then it keeps HTTPS", url: "HTTPS://dav.example.com/backups?x=1#y", expectedBase: "https://dav.example.com", expectedPath: "backups"},
		{name: "with the http scheme, then it keeps HTTP", url: "http://user@localhost/backups", expectedBase: "http://user@localhost", expectedPath: "backups"},
		{name: "with another scheme, then it returns an unsupported scheme error", url: "ftp://host/backups", contains: `unsupported WebDAV scheme "ftp"`},
		{name: "without host, then it returns a host error", url: "webdav:///backups", contains: "requires a host"},
		{name: "with an invalid URL, then it returns a parse error", url: "webdav://host:port/backups", contains: "parse WebDAV URL"},
	}
	for _, tt := range tests {
		t.Run(`Given a WebDAV repository URL,
		when ParseWebDAVURL is called `+tt.name, func(t *testing.T) {
			t.Parallel()

			location, err := webdav.ParseWebDAVURL(tt.url)

			if tt.contains != "" {
				require.ErrorContains(t, err, tt.contains)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expectedBase, location.BaseURL)
			require.Equal(t, tt.expectedPath, location.Path)
		})
	}
}

func TestNewClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  int
		wantErr bool
	}{
		{name: "that accepts the credentials, then it returns a client", status: http.StatusOK},
		{name: "that rejects the credentials, then it returns a connection error", status: http.StatusUnauthorized, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(`Given a WebDAV server `+tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				user, password, ok := r.BasicAuth()
				if tt.status == http.StatusOK && (!ok || user != "user" || password != "password") {
					w.Header().Set("WWW-Authenticate", `Basic realm="dav"`)
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				if tt.status != http.StatusOK {
					w.Header().Set("WWW-Authenticate", `Basic realm="dav"`)
				}
				w.WriteHeader(tt.status)
			}))
			t.Cleanup(server.Close)

			client, err := webdav.NewClient(server.URL, "user", "password")

			if tt.wantErr {
				require.ErrorContains(t, err, "connect to WebDAV server")
				return
			}
			require.NoError(t, err)
			require.NotNil(t, client)
		})
	}
}
