package upstream

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/jfxdev/grom/backend/internal/constants"
	registryapp "github.com/jfxdev/grom/backend/internal/registry/application"
)

type Credential struct {
	Username string
	Password string
}

type Copier struct {
	registry    name.Registry
	credentials map[string]Credential
	transport   http.RoundTripper
}

func NewCopier(rawRegistryURL string, credentials map[string]Credential) (*Copier, error) {
	target, err := url.Parse(rawRegistryURL)
	if err != nil {
		return nil, err
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return nil, fmt.Errorf("distribution URL must use http or https")
	}
	if target.Host == "" || strings.Trim(target.Path, "/") != "" {
		return nil, fmt.Errorf("distribution URL must contain only scheme and host")
	}
	options := []name.Option{name.WeakValidation}
	if target.Scheme == "http" {
		options = append(options, name.Insecure)
	}
	registry, err := name.NewRegistry(target.Host, options...)
	if err != nil {
		return nil, err
	}
	return &Copier{registry: registry, credentials: credentials, transport: remote.DefaultTransport}, nil
}

func (c *Copier) Resolve(ctx context.Context, upstream, repository, reference string) (string, error) {
	ref, err := sourceReference(upstream, repository, reference)
	if err != nil {
		return "", err
	}
	descriptor, err := remote.Get(ref, c.sourceOptions(ctx, upstream)...)
	if err != nil {
		return "", classifyRemoteError(err)
	}
	return descriptor.Digest.String(), nil
}

func (c *Copier) Copy(ctx context.Context, upstream, sourceRepository, localRepository, reference, token string) (string, error) {
	source, err := sourceReference(upstream, sourceRepository, reference)
	if err != nil {
		return "", err
	}
	descriptor, err := remote.Get(source, c.sourceOptions(ctx, upstream)...)
	if err != nil {
		return "", classifyRemoteError(err)
	}
	destination, err := destinationReference(c.registry, localRepository, reference)
	if err != nil {
		return "", err
	}
	options := []remote.Option{
		remote.WithContext(ctx),
		remote.WithTransport(&bearerTransport{base: c.transport, token: token}),
	}
	if descriptor.MediaType.IsIndex() {
		index, err := descriptor.ImageIndex()
		if err != nil {
			return "", err
		}
		if err := remote.WriteIndex(destination, index, options...); err != nil {
			return "", fmt.Errorf("write image index: %w", err)
		}
	} else {
		image, err := descriptor.Image()
		if err != nil {
			return "", err
		}
		if err := remote.Write(destination, image, options...); err != nil {
			return "", fmt.Errorf("write image: %w", err)
		}
	}
	return descriptor.Digest.String(), nil
}

func (c *Copier) sourceOptions(ctx context.Context, upstream string) []remote.Option {
	options := []remote.Option{remote.WithContext(ctx), remote.WithTransport(c.transport)}
	if credential, ok := c.credentials[upstream]; ok {
		options = append(options, remote.WithAuth(&authn.Basic{Username: credential.Username, Password: credential.Password}))
	}
	return options
}

func sourceReference(upstream, repository, reference string) (name.Reference, error) {
	registry := ""
	switch upstream {
	case constants.ProjectUpstreamDockerHub:
		registry = "registry-1.docker.io"
	case constants.ProjectUpstreamGHCR:
		registry = "ghcr.io"
	default:
		return nil, fmt.Errorf("unsupported upstream")
	}
	return name.ParseReference(registry+"/"+repository+referenceSeparator(reference)+reference, name.WeakValidation)
}

func destinationReference(registry name.Registry, repository, reference string) (name.Reference, error) {
	options := []name.Option{name.WeakValidation}
	if registry.Scheme() == "http" {
		options = append(options, name.Insecure)
	}
	return name.ParseReference(registry.Name()+"/"+repository+referenceSeparator(reference)+reference, options...)
}

func referenceSeparator(reference string) string {
	if strings.Contains(reference, ":") {
		return "@"
	}
	return ":"
}

func classifyRemoteError(err error) error {
	var remoteError *transport.Error
	if errors.As(err, &remoteError) && remoteError.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w", registryapp.ErrUpstreamManifestNotFound)
	}
	return err
}

type bearerTransport struct {
	base  http.RoundTripper
	token string
}

func (t *bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(clone)
}

var _ registryapp.UpstreamCopier = (*Copier)(nil)
