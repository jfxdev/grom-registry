package distribution

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	registryapp "github.com/jfxdev/grom/backend/internal/registry/application"
	"github.com/jfxdev/grom/backend/internal/registry/infrastructure/signing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestListProjectRepositoriesFollowsCatalogPagination(t *testing.T) {
	var requests atomic.Int32
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		if r.Header.Get("Authorization") == "" {
			t.Error("expected catalog authorization")
		}
		header := make(http.Header)
		body := ""
		switch r.URL.Query().Get("last") {
		case "":
			header.Set("Link", `</v2/_catalog?n=1000&last=other%2Fignored>; rel="next"`)
			body = `{"repositories":["project/one","other/ignored"]}`
		case "other/ignored":
			body = `{"repositories":["project/two"]}`
		default:
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Status:     "400 Bad Request",
				Header:     header,
				Body:       io.NopCloser(strings.NewReader("unexpected page")),
				Request:    r,
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     header,
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	})

	temp := t.TempDir()
	signer, err := signing.LoadOrCreate(temp+"/key.pem", temp+"/cert.pem")
	if err != nil {
		t.Fatal(err)
	}
	tokens := registryapp.NewTokenService(nil, nil, signer, time.Minute)
	client, err := NewClient("http://distribution.local", tokens)
	if err != nil {
		t.Fatal(err)
	}
	client.http.Transport = transport

	repositories, err := client.ListProjectRepositories(context.Background(), "project")
	if err != nil {
		t.Fatal(err)
	}
	if len(repositories) != 2 || repositories[0] != "one" || repositories[1] != "two" {
		t.Fatalf("unexpected repositories: %#v", repositories)
	}
	if requests.Load() != 2 {
		t.Fatalf("expected two catalog requests, got %d", requests.Load())
	}
}

func TestListProjectRepositoriesPageFiltersAndReturnsOnlyOpaqueMarker(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.URL.Query().Get("n"); got != "2" {
			t.Errorf("page size = %q", got)
		}
		if got := r.URL.Query().Get("last"); got != "project/previous" {
			t.Errorf("marker = %q", got)
		}
		header := make(http.Header)
		header.Set("Link", `</v2/_catalog?n=2&last=project%2Ftwo>; rel="next"`)
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: header, Body: io.NopCloser(strings.NewReader(`{"repositories":["project/one","other/ignored"]}`)), Request: r}, nil
	})
	temp := t.TempDir()
	signer, err := signing.LoadOrCreate(temp+"/key.pem", temp+"/cert.pem")
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient("http://distribution.local", registryapp.NewTokenService(nil, nil, signer, time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	client.http.Transport = transport

	page, err := client.ListProjectRepositoriesPage(context.Background(), "project", 2, "project/previous")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Repositories) != 1 || page.Repositories[0] != "one" || page.NextMarker != "project/two" {
		t.Fatalf("unexpected page: %#v", page)
	}
}

func TestListProjectRepositoriesPageSkipsUnrelatedCatalogPages(t *testing.T) {
	var requests atomic.Int32
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		header := make(http.Header)
		if got := r.URL.Query().Get("n"); got != "2" {
			return &http.Response{StatusCode: http.StatusBadRequest, Status: "400 Bad Request", Header: header, Body: io.NopCloser(strings.NewReader("unexpected page")), Request: r}, nil
		}
		switch r.URL.Query().Get("last") {
		case "":
			header.Set("Link", `</v2/_catalog?n=2&last=other%2Ftwo>; rel="next"`)
			return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: header, Body: io.NopCloser(strings.NewReader(`{"repositories":["other/one","other/two"]}`)), Request: r}, nil
		case "other/two":
			header.Set("Link", `</v2/_catalog?n=2&last=project%2Ftwo>; rel="next"`)
			return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: header, Body: io.NopCloser(strings.NewReader(`{"repositories":["project/one","project/two"]}`)), Request: r}, nil
		default:
			return &http.Response{StatusCode: http.StatusBadRequest, Status: "400 Bad Request", Header: header, Body: io.NopCloser(strings.NewReader("unexpected page")), Request: r}, nil
		}
	})
	temp := t.TempDir()
	signer, err := signing.LoadOrCreate(temp+"/key.pem", temp+"/cert.pem")
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient("http://distribution.local", registryapp.NewTokenService(nil, nil, signer, time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	client.http.Transport = transport

	page, err := client.ListProjectRepositoriesPage(context.Background(), "project", 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := page.Repositories; len(got) != 2 || got[0] != "one" || got[1] != "two" || page.NextMarker != "project/two" {
		t.Fatalf("unexpected page: %#v", page)
	}
	if requests.Load() != 2 {
		t.Fatalf("expected two catalog requests, got %d", requests.Load())
	}
}

func TestListTagsPageReturnsOnlyTheRequestedDistributionPage(t *testing.T) {
	var requests atomic.Int32
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		if got := r.URL.Query().Get("n"); got != "2" {
			t.Errorf("expected page size 2, got %q", got)
		}
		if got := r.URL.Query().Get("last"); got != "stable" {
			t.Errorf("expected marker stable, got %q", got)
		}
		header := make(http.Header)
		header.Set("Link", `</v2/project/api/tags/list?n=2&last=v2>; rel="next"`)
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: header, Body: io.NopCloser(strings.NewReader(`{"name":"project/api","tags":["v1","v2"]}`)), Request: r}, nil
	})
	temp := t.TempDir()
	signer, err := signing.LoadOrCreate(temp+"/key.pem", temp+"/cert.pem")
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient("http://distribution.local", registryapp.NewTokenService(nil, nil, signer, time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	client.http.Transport = transport

	page, err := client.ListTagsPage(context.Background(), "project/api", 2, "stable")
	if err != nil {
		t.Fatal(err)
	}
	if page.Name != "project/api" || len(page.Tags) != 2 || page.NextMarker != "v2" {
		t.Fatalf("unexpected tag page: %#v", page)
	}
	if requests.Load() != 1 {
		t.Fatalf("expected one request, got %d", requests.Load())
	}
}

func TestListTagsPageTreatsMissingDistributionTagIndexAsEmpty(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Status:     "404 Not Found",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"errors":[{"code":"NAME_UNKNOWN"}]}`)),
			Request:    r,
		}, nil
	})
	temp := t.TempDir()
	signer, err := signing.LoadOrCreate(temp+"/key.pem", temp+"/cert.pem")
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient("http://distribution.local", registryapp.NewTokenService(nil, nil, signer, time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	client.http.Transport = transport

	page, err := client.ListTagsPage(context.Background(), "project/api", 20, "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Name != "project/api" || page.Tags == nil || len(page.Tags) != 0 || page.NextMarker != "" {
		t.Fatalf("unexpected empty tag page: %#v", page)
	}
}

func TestListTagsTreatsMissingDistributionTagIndexAsEmpty(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Status:     "404 Not Found",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"errors":[{"code":"NAME_UNKNOWN"}]}`)),
			Request:    r,
		}, nil
	})
	temp := t.TempDir()
	signer, err := signing.LoadOrCreate(temp+"/key.pem", temp+"/cert.pem")
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient("http://distribution.local", registryapp.NewTokenService(nil, nil, signer, time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	client.http.Transport = transport

	tags, err := client.ListTags(context.Background(), "project/api")
	if err != nil {
		t.Fatal(err)
	}
	if tags.Name != "project/api" || tags.Tags == nil || len(tags.Tags) != 0 {
		t.Fatalf("unexpected empty tag list: %#v", tags)
	}
}

func TestListRepositoryTagsOmitsDanglingTagLinks(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/project/api/tags/list":
			return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"name":"project/api","tags":["live","dangling"]}`)), Request: r}, nil
		case r.Method == http.MethodHead && r.URL.Path == "/v2/project/api/manifests/live":
			header := make(http.Header)
			header.Set("Docker-Content-Digest", "sha256:live")
			return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: header, Body: http.NoBody, Request: r}, nil
		case r.Method == http.MethodHead && r.URL.Path == "/v2/project/api/manifests/dangling":
			return &http.Response{StatusCode: http.StatusNotFound, Status: "404 Not Found", Header: make(http.Header), Body: http.NoBody, Request: r}, nil
		default:
			return &http.Response{StatusCode: http.StatusBadRequest, Status: "400 Bad Request", Header: make(http.Header), Body: http.NoBody, Request: r}, nil
		}
	})
	temp := t.TempDir()
	signer, err := signing.LoadOrCreate(temp+"/key.pem", temp+"/cert.pem")
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient("http://distribution.local", registryapp.NewTokenService(nil, nil, signer, time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	client.http.Transport = transport

	tags, err := client.ListRepositoryTags(context.Background(), "project/api")
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0] != "live" {
		t.Fatalf("expected only the live tag, got %#v", tags)
	}
}

func TestListLiveTagsPageFillsPagePastDanglingLinks(t *testing.T) {
	var pageRequests atomic.Int32
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/project/api/tags/list":
			pageRequests.Add(1)
			header := make(http.Header)
			switch r.URL.Query().Get("last") {
			case "":
				header.Set("Link", `</v2/project/api/tags/list?n=2&last=dangling>; rel="next"`)
				return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: header, Body: io.NopCloser(strings.NewReader(`{"name":"project/api","tags":["alpha","dangling"]}`)), Request: r}, nil
			case "dangling":
				return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: header, Body: io.NopCloser(strings.NewReader(`{"name":"project/api","tags":["beta"]}`)), Request: r}, nil
			default:
				return &http.Response{StatusCode: http.StatusBadRequest, Status: "400 Bad Request", Header: header, Body: http.NoBody, Request: r}, nil
			}
		case r.Method == http.MethodHead && r.URL.Path == "/v2/project/api/manifests/alpha":
			header := make(http.Header)
			header.Set("Docker-Content-Digest", "sha256:alpha")
			return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: header, Body: http.NoBody, Request: r}, nil
		case r.Method == http.MethodHead && r.URL.Path == "/v2/project/api/manifests/beta":
			header := make(http.Header)
			header.Set("Docker-Content-Digest", "sha256:beta")
			return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: header, Body: http.NoBody, Request: r}, nil
		case r.Method == http.MethodHead && r.URL.Path == "/v2/project/api/manifests/dangling":
			return &http.Response{StatusCode: http.StatusNotFound, Status: "404 Not Found", Header: make(http.Header), Body: http.NoBody, Request: r}, nil
		default:
			return &http.Response{StatusCode: http.StatusBadRequest, Status: "400 Bad Request", Header: make(http.Header), Body: http.NoBody, Request: r}, nil
		}
	})
	temp := t.TempDir()
	signer, err := signing.LoadOrCreate(temp+"/key.pem", temp+"/cert.pem")
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient("http://distribution.local", registryapp.NewTokenService(nil, nil, signer, time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	client.http.Transport = transport

	page, err := client.ListLiveTagsPage(context.Background(), "project/api", 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Tags) != 2 || page.Tags[0] != "alpha" || page.Tags[1] != "beta" || page.NextMarker != "" {
		t.Fatalf("unexpected live tag page: %#v", page)
	}
	if pageRequests.Load() != 2 {
		t.Fatalf("expected two Distribution pages, got %d", pageRequests.Load())
	}
}

func TestListTagsPageNormalizesNullTagsToAnEmptyArray(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"name":"project/api","tags":null}`)),
			Request:    r,
		}, nil
	})
	temp := t.TempDir()
	signer, err := signing.LoadOrCreate(temp+"/key.pem", temp+"/cert.pem")
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient("http://distribution.local", registryapp.NewTokenService(nil, nil, signer, time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	client.http.Transport = transport

	page, err := client.ListTagsPage(context.Background(), "project/api", 20, "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Tags == nil || len(page.Tags) != 0 {
		t.Fatalf("null tags must be normalized to an empty array: %#v", page)
	}
}

func TestFetchManifestCalculatesLogicalPlatformSizesAndPersistsChildren(t *testing.T) {
	indexJSON := `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:amd64","size":321,"platform":{"os":"linux","architecture":"amd64"}},{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:arm64","size":322,"platform":{"os":"linux","architecture":"arm64","variant":"v8"}}]}`
	amd64JSON := `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.image.config.v1+json","digest":"sha256:config-amd64","size":100},"layers":[{"mediaType":"application/vnd.oci.image.layer.v1.tar+gzip","digest":"sha256:layer-amd64","size":900}]}`
	arm64JSON := `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.image.config.v1+json","digest":"sha256:config-arm64","size":200},"layers":[{"mediaType":"application/vnd.oci.image.layer.v1.tar+gzip","digest":"sha256:layer-arm64","size":1800}]}`
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, digest := "", ""
		switch r.URL.Path {
		case "/v2/project/api/manifests/latest":
			body, digest = indexJSON, "sha256:index"
		case "/v2/project/api/manifests/sha256:amd64":
			body, digest = amd64JSON, "sha256:amd64"
		case "/v2/project/api/manifests/sha256:arm64":
			body, digest = arm64JSON, "sha256:arm64"
		case "/v2/project/api/blobs/sha256:config-amd64":
			body = `{"os":"linux","architecture":"amd64"}`
		case "/v2/project/api/blobs/sha256:config-arm64":
			body = `{"os":"linux","architecture":"arm64","variant":"v8"}`
		default:
			return &http.Response{StatusCode: http.StatusNotFound, Status: "404 Not Found", Header: make(http.Header), Body: http.NoBody, Request: r}, nil
		}
		header := make(http.Header)
		if digest != "" {
			header.Set("Docker-Content-Digest", digest)
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: header, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	temp := t.TempDir()
	signer, err := signing.LoadOrCreate(temp+"/key.pem", temp+"/cert.pem")
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient("http://distribution.local", registryapp.NewTokenService(nil, nil, signer, time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	client.http.Transport = transport

	metadata, err := client.FetchManifest(context.Background(), "project/api", "latest")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Digest != "sha256:index" || metadata.ManifestSize != int64(len(indexJSON)) {
		t.Fatalf("unexpected index metadata: %#v", metadata)
	}
	if len(metadata.Platforms) != 2 || metadata.Platforms[0].CompressedSize != 1000 || metadata.Platforms[1].CompressedSize != 2000 {
		t.Fatalf("platform sizes must include config and layers: %#v", metadata.Platforms)
	}
	if metadata.Platforms[1].Variant != "v8" || len(metadata.Children) != 2 {
		t.Fatalf("unexpected platform children: %#v", metadata)
	}
	if metadata.Children[0].Digest != "sha256:amd64" || metadata.Children[0].ManifestSize != int64(len(amd64JSON)) || metadata.Children[0].Platforms[0].Digest != "sha256:amd64" {
		t.Fatalf("unexpected amd64 child metadata: %#v", metadata.Children[0])
	}
	if metadata.Children[1].Digest != "sha256:arm64" || metadata.Children[1].Platforms[0].CompressedSize != 2000 {
		t.Fatalf("unexpected arm64 child metadata: %#v", metadata.Children[1])
	}
}

func TestFetchManifestTreeRejectsExcessiveTraversal(t *testing.T) {
	client := &Client{}
	for name, traversal := range map[string]*manifestTraversal{
		"depth": {visiting: map[string]bool{}, depth: maxManifestTraversalDepth + 1},
		"nodes": {visiting: map[string]bool{}, nodes: maxManifestTraversalNodes},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := client.fetchManifestTree(context.Background(), "project/api", "latest", "token", traversal)
			if err == nil || !strings.Contains(err.Error(), "maximum") {
				t.Fatalf("expected traversal limit error, got %v", err)
			}
		})
	}
}
