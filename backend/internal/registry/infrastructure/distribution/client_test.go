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
