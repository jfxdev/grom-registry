package upstream

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/jfxdev/grom/backend/internal/constants"
	registryapp "github.com/jfxdev/grom/backend/internal/registry/application"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestCopierPreservesOCIContentAndDigests(t *testing.T) {
	for _, test := range []struct {
		name      string
		index     bool
		reference string
	}{
		{name: "image tag", reference: "latest"},
		{name: "image digest", reference: "digest"},
		{name: "multi-platform index", index: true, reference: "latest"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := httptest.NewServer(registry.New())
			defer source.Close()
			destinationRegistry := registry.New()
			destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer internal-token" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				destinationRegistry.ServeHTTP(w, r)
			}))
			defer destination.Close()
			sourceURL, err := url.Parse(source.URL)
			if err != nil {
				t.Fatal(err)
			}
			transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
				clone := request.Clone(request.Context())
				if request.URL.Host == "registry-1.docker.io" || request.URL.Host == "ghcr.io" {
					clone.URL.Scheme = "http"
					clone.URL.Host = sourceURL.Host
				}
				return http.DefaultTransport.RoundTrip(clone)
			})

			copier, err := NewCopier(destination.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			copier.transport = transport
			sourceRef, err := name.ParseReference("registry-1.docker.io/library/alpine:latest", name.WeakValidation)
			if err != nil {
				t.Fatal(err)
			}
			sourceOptions := []remote.Option{remote.WithContext(context.Background()), remote.WithTransport(transport)}
			var expectedDigest string
			if test.index {
				index, err := random.Index(256, 1, 2)
				if err != nil {
					t.Fatal(err)
				}
				if err := remote.WriteIndex(sourceRef, index, sourceOptions...); err != nil {
					t.Fatal(err)
				}
				digest, err := index.Digest()
				if err != nil {
					t.Fatal(err)
				}
				expectedDigest = digest.String()
			} else {
				image, err := random.Image(256, 1)
				if err != nil {
					t.Fatal(err)
				}
				if err := remote.Write(sourceRef, image, sourceOptions...); err != nil {
					t.Fatal(err)
				}
				digest, err := image.Digest()
				if err != nil {
					t.Fatal(err)
				}
				expectedDigest = digest.String()
			}
			reference := test.reference
			if reference == "digest" {
				reference = expectedDigest
			}

			resolved, err := copier.Resolve(context.Background(), constants.ProjectUpstreamDockerHub, "library/alpine", reference)
			if err != nil || resolved != expectedDigest {
				t.Fatalf("resolve: digest=%q expected=%q err=%v", resolved, expectedDigest, err)
			}
			copied, err := copier.Copy(context.Background(), constants.ProjectUpstreamDockerHub, "library/alpine", "dockerhub/alpine", reference, "internal-token")
			if err != nil || copied != expectedDigest {
				t.Fatalf("copy: digest=%q expected=%q err=%v", copied, expectedDigest, err)
			}

			destinationOptions := []remote.Option{
				remote.WithContext(context.Background()),
				remote.WithTransport(&bearerTransport{base: transport, token: "internal-token"}),
			}
			destinationRef, err := name.ParseReference(destinationURL(t, destination.URL)+"/dockerhub/alpine"+referenceSeparator(reference)+reference, name.WeakValidation)
			if err != nil {
				t.Fatal(err)
			}
			assertSameManifestTree(t, sourceRef, destinationRef, sourceOptions, destinationOptions)
		})
	}
}

func destinationURL(t *testing.T, raw string) string {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.Host
}

func assertSameManifestTree(t *testing.T, sourceRef, destinationRef name.Reference, sourceOptions, destinationOptions []remote.Option) {
	t.Helper()
	source, err := remote.Get(sourceRef, sourceOptions...)
	if err != nil {
		t.Fatal(err)
	}
	destination, err := remote.Get(destinationRef, destinationOptions...)
	if err != nil {
		t.Fatal(err)
	}
	if source.Digest != destination.Digest || source.MediaType != destination.MediaType {
		t.Fatalf("manifest metadata changed: source=%s/%s destination=%s/%s", source.Digest, source.MediaType, destination.Digest, destination.MediaType)
	}
	sourceRaw, err := source.RawManifest()
	if err != nil {
		t.Fatal(err)
	}
	destinationRaw, err := destination.RawManifest()
	if err != nil {
		t.Fatal(err)
	}
	if string(sourceRaw) != string(destinationRaw) {
		t.Fatal("manifest bytes changed during copy")
	}
	if source.MediaType.IsIndex() {
		index, err := source.ImageIndex()
		if err != nil {
			t.Fatal(err)
		}
		manifest, err := index.IndexManifest()
		if err != nil {
			t.Fatal(err)
		}
		for _, child := range manifest.Manifests {
			assertSameManifestTree(t,
				sourceRef.Context().Digest(child.Digest.String()),
				destinationRef.Context().Digest(child.Digest.String()),
				sourceOptions, destinationOptions,
			)
		}
		return
	}
	image, err := destination.Image()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := image.ConfigFile(); err != nil {
		t.Fatalf("read copied config: %v", err)
	}
	layers, err := image.Layers()
	if err != nil {
		t.Fatal(err)
	}
	for _, layer := range layers {
		contents, err := layer.Compressed()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, contents); err != nil {
			t.Fatalf("read copied layer: %v", err)
		}
		if err := contents.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCopierClassifiesMissingUpstreamManifest(t *testing.T) {
	source := httptest.NewServer(registry.New())
	defer source.Close()
	destination := httptest.NewServer(registry.New())
	defer destination.Close()
	sourceURL, err := url.Parse(source.URL)
	if err != nil {
		t.Fatal(err)
	}
	copier, err := NewCopier(destination.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	copier.transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		clone := request.Clone(request.Context())
		clone.URL.Scheme = "http"
		clone.URL.Host = sourceURL.Host
		return http.DefaultTransport.RoundTrip(clone)
	})

	_, err = copier.Resolve(context.Background(), constants.ProjectUpstreamGHCR, "owner/missing", "latest")
	if !errors.Is(err, registryapp.ErrUpstreamManifestNotFound) {
		t.Fatalf("expected manifest-not-found classification, got %v", err)
	}
}

func TestNewCopierRejectsInvalidDistributionURLs(t *testing.T) {
	for _, raw := range []string{"ftp://registry.test", "https://registry.test/path", "://bad"} {
		if _, err := NewCopier(raw, nil); err == nil {
			t.Fatalf("expected %q to be rejected", raw)
		}
	}
}

var _ v1.Image
