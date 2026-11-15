package gcs_test

import (
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/option"
)

type fakeBucket struct {
	retention  bool
	versioning bool
	policy     time.Duration
}

type fakeRetention struct {
	Mode            string    `json:"mode"`
	RetainUntilTime time.Time `json:"retainUntilTime"`
}

type fakeVersion struct {
	generation int64
	data       []byte
	live       bool
	retention  *fakeRetention
}

type fakeGCS struct {
	mu         sync.Mutex
	bucket     string
	settings   fakeBucket
	objects    map[string][]*fakeVersion
	generation int64
}

func newFakeGCS(t *testing.T, bucket string, settings fakeBucket) (*fakeGCS, *storage.Client) {
	t.Helper()

	fake := &fakeGCS{bucket: bucket, settings: settings, objects: map[string][]*fakeVersion{}}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	client, err := storage.NewClient(t.Context(),
		option.WithEndpoint(server.URL+"/storage/v1/"),
		option.WithoutAuthentication(),
		storage.WithJSONReads(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	return fake, client
}

func (f *fakeGCS) generations(name string) []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	generations := make([]int64, 0)
	for _, version := range f.objects[name] {
		generations = append(generations, version.generation)
	}
	return generations
}

func (f *fakeGCS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	bucketPath := "/storage/v1/b/" + f.bucket
	escaped := r.URL.EscapedPath()
	switch {
	case r.Method == http.MethodPost && escaped == "/upload"+bucketPath+"/o":
		f.insert(w, r)
	case r.Method == http.MethodGet && escaped == bucketPath:
		f.writeJSON(w, f.bucketResource())
	case r.Method == http.MethodGet && escaped == bucketPath+"/o":
		f.list(w, r)
	case strings.HasPrefix(escaped, bucketPath+"/o/"):
		name, err := url.PathUnescape(strings.TrimPrefix(escaped, bucketPath+"/o/"))
		if err != nil {
			f.fail(w, http.StatusBadRequest)
			return
		}
		f.object(w, r, name)
	default:
		f.fail(w, http.StatusNotImplemented)
	}
}

func (f *fakeGCS) bucketResource() map[string]any {
	resource := map[string]any{"name": f.bucket, "versioning": map[string]bool{"enabled": f.settings.versioning}}
	if f.settings.retention {
		resource["objectRetention"] = map[string]string{"mode": "Enabled"}
	}
	if f.settings.policy > 0 {
		resource["retentionPolicy"] = map[string]string{
			"retentionPeriod": strconv.FormatInt(int64(f.settings.policy.Seconds()), 10),
			"effectiveTime":   time.Now().UTC().Format(time.RFC3339),
		}
	}
	return resource
}

func (f *fakeGCS) insert(w http.ResponseWriter, r *http.Request) {
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		f.fail(w, http.StatusBadRequest)
		return
	}
	parts := multipart.NewReader(r.Body, params["boundary"])
	metadataPart, err := parts.NextPart()
	if err != nil {
		f.fail(w, http.StatusBadRequest)
		return
	}
	var metadata struct {
		Name      string         `json:"name"`
		Retention *fakeRetention `json:"retention"`
	}
	if err := json.NewDecoder(metadataPart).Decode(&metadata); err != nil {
		f.fail(w, http.StatusBadRequest)
		return
	}
	mediaPart, err := parts.NextPart()
	if err != nil {
		f.fail(w, http.StatusBadRequest)
		return
	}
	data, err := io.ReadAll(mediaPart)
	if err != nil {
		f.fail(w, http.StatusBadRequest)
		return
	}

	live := f.live(metadata.Name)
	if r.URL.Query().Get("ifGenerationMatch") == "0" && live != nil {
		f.fail(w, http.StatusPreconditionFailed)
		return
	}
	if live != nil && !f.settings.versioning && retained(live) {
		f.fail(w, http.StatusForbidden)
		return
	}
	if live != nil {
		if f.settings.versioning {
			live.live = false
		} else {
			f.remove(metadata.Name, live.generation)
		}
	}
	f.generation++
	version := &fakeVersion{generation: f.generation, data: data, live: true, retention: metadata.Retention}
	f.objects[metadata.Name] = append(f.objects[metadata.Name], version)
	f.writeJSON(w, f.objectResource(metadata.Name, version))
}

func (f *fakeGCS) object(w http.ResponseWriter, r *http.Request, name string) {
	version := f.find(name, r.URL.Query().Get("generation"))
	if version == nil {
		f.fail(w, http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if r.URL.Query().Get("alt") == "media" {
			_, _ = w.Write(version.data)
			return
		}
		f.writeJSON(w, f.objectResource(name, version))
	case http.MethodPatch:
		var update struct {
			Retention *fakeRetention `json:"retention"`
		}
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			f.fail(w, http.StatusBadRequest)
			return
		}
		if version.retention != nil && update.Retention != nil && update.Retention.RetainUntilTime.Before(version.retention.RetainUntilTime) {
			f.fail(w, http.StatusForbidden)
			return
		}
		version.retention = update.Retention
		f.writeJSON(w, f.objectResource(name, version))
	case http.MethodDelete:
		if r.URL.Query().Get("generation") == "" && f.settings.versioning {
			version.live = false
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if retained(version) {
			f.fail(w, http.StatusForbidden)
			return
		}
		f.remove(name, version.generation)
		w.WriteHeader(http.StatusNoContent)
	default:
		f.fail(w, http.StatusNotImplemented)
	}
}

func (f *fakeGCS) list(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("prefix")
	versions := r.URL.Query().Get("versions") == "true"
	items := make([]map[string]any, 0)
	for name, objectVersions := range f.objects {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		for _, version := range objectVersions {
			if versions || version.live {
				items = append(items, f.objectResource(name, version))
			}
		}
	}
	f.writeJSON(w, map[string]any{"kind": "storage#objects", "items": items})
}

func (f *fakeGCS) live(name string) *fakeVersion {
	for _, version := range f.objects[name] {
		if version.live {
			return version
		}
	}
	return nil
}

func (f *fakeGCS) find(name, generation string) *fakeVersion {
	if generation == "" {
		return f.live(name)
	}
	for _, version := range f.objects[name] {
		if strconv.FormatInt(version.generation, 10) == generation {
			return version
		}
	}
	return nil
}

func (f *fakeGCS) remove(name string, generation int64) {
	versions := f.objects[name]
	for i, version := range versions {
		if version.generation == generation {
			f.objects[name] = append(versions[:i], versions[i+1:]...)
			break
		}
	}
	if len(f.objects[name]) == 0 {
		delete(f.objects, name)
	}
}

func (f *fakeGCS) objectResource(name string, version *fakeVersion) map[string]any {
	resource := map[string]any{
		"kind":       "storage#object",
		"bucket":     f.bucket,
		"name":       name,
		"generation": strconv.FormatInt(version.generation, 10),
		"size":       strconv.Itoa(len(version.data)),
	}
	if version.retention != nil {
		resource["retention"] = version.retention
	}
	return resource
}

func (f *fakeGCS) writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func (f *fakeGCS) fail(w http.ResponseWriter, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": http.StatusText(code)}})
}

func retained(version *fakeVersion) bool {
	return version.retention != nil && version.retention.RetainUntilTime.After(time.Now())
}
