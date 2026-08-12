package distribution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	registryapp "github.com/jfxdev/grom/backend/internal/registry/application"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
)

type Client struct {
	baseURL *url.URL
	http    *http.Client
	tokens  *registryapp.TokenService
}

type RepositoryList struct {
	Repositories []string `json:"repositories"`
}

type TagList struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

type TagPage struct {
	Name       string
	Tags       []string
	NextMarker string
}

type responseStatusError struct {
	statusCode int
	status     string
}

func (e *responseStatusError) Error() string {
	return "distribution returned " + e.status
}

type ProjectRepositoryPage struct {
	Repositories []string
	NextMarker   string
}

type manifestDocument struct {
	MediaType    string `json:"mediaType"`
	ArtifactType string `json:"artifactType"`
	Config       struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
		Size      int64  `json:"size"`
	} `json:"config"`
	Layers []struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
		Size      int64  `json:"size"`
	} `json:"layers"`
	Manifests []struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
		Size      int64  `json:"size"`
		Platform  struct {
			OS           string `json:"os"`
			Architecture string `json:"architecture"`
			Variant      string `json:"variant"`
		} `json:"platform"`
	} `json:"manifests"`
	Subject *struct {
		Digest string `json:"digest"`
	} `json:"subject"`
}

type imageConfigDocument struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	Variant      string `json:"variant"`
}

type referrersResponse struct {
	Manifests []struct {
		Digest       string `json:"digest"`
		MediaType    string `json:"mediaType"`
		ArtifactType string `json:"artifactType"`
		Size         int64  `json:"size"`
	} `json:"manifests"`
}

func NewClient(rawURL string, tokens *registryapp.TokenService) (*Client, error) {
	baseURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 15 * time.Second},
		tokens:  tokens,
	}, nil
}

func (c *Client) Available(ctx context.Context) bool {
	endpoint := c.baseURL.ResolveReference(&url.URL{Path: "/v2/"})
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return false
	}
	response, err := c.http.Do(request)
	if err != nil {
		return false
	}
	defer func() { _ = response.Body.Close() }()
	return response.StatusCode == http.StatusOK || response.StatusCode == http.StatusUnauthorized
}

func (c *Client) ListProjectRepositories(ctx context.Context, project string) ([]string, error) {
	prefix := project + "/"
	result := make([]string, 0)
	nextPath := "/v2/_catalog?n=1000"
	for nextPath != "" {
		token, err := c.tokens.IssueCatalog("grom-internal")
		if err != nil {
			return nil, err
		}
		var response RepositoryList
		nextPath, err = c.getWithLink(ctx, nextPath, token, &response)
		if err != nil {
			return nil, err
		}
		for _, name := range response.Repositories {
			if strings.HasPrefix(name, prefix) {
				result = append(result, strings.TrimPrefix(name, prefix))
			}
		}
	}
	return result, nil
}

// ListProjectRepositoriesPage keeps Distribution's catalog Link private while
// exposing only a project-relative continuation marker to the control plane.
func (c *Client) ListProjectRepositoriesPage(ctx context.Context, project string, limit int, marker string) (*ProjectRepositoryPage, error) {
	if limit < 1 {
		return nil, fmt.Errorf("catalog page limit must be positive")
	}
	page := &ProjectRepositoryPage{}
	prefix := project + "/"
	for {
		token, err := c.tokens.IssueCatalog("grom-internal")
		if err != nil {
			return nil, err
		}
		query := url.Values{"n": []string{fmt.Sprintf("%d", limit)}}
		if marker != "" {
			query.Set("last", marker)
		}
		var response RepositoryList
		nextPath, err := c.getWithLink(ctx, "/v2/_catalog?"+query.Encode(), token, &response)
		if err != nil {
			return nil, err
		}
		page.Repositories = page.Repositories[:0]
		for _, name := range response.Repositories {
			if strings.HasPrefix(name, prefix) {
				page.Repositories = append(page.Repositories, strings.TrimPrefix(name, prefix))
			}
		}
		page.NextMarker = ""
		if nextPath != "" {
			nextURL, parseErr := url.Parse(nextPath)
			if parseErr != nil {
				return nil, parseErr
			}
			page.NextMarker = nextURL.Query().Get("last")
			if page.NextMarker == "" {
				return nil, fmt.Errorf("distribution pagination link has no catalog marker")
			}
		}
		if len(page.Repositories) > 0 || page.NextMarker == "" {
			return page, nil
		}
		marker = page.NextMarker
	}
}

func (c *Client) ListTags(ctx context.Context, repository string) (*TagList, error) {
	token, err := c.issueRepositoryPullToken(repository)
	if err != nil {
		return nil, err
	}
	return c.listTags(ctx, repository, token)
}

func (c *Client) listTags(ctx context.Context, repository, token string) (*TagList, error) {
	var response TagList
	if err := c.get(ctx, "/v2/"+repository+"/tags/list", token, &response); err != nil {
		var statusError *responseStatusError
		if errors.As(err, &statusError) && statusError.statusCode == http.StatusNotFound {
			// Distribution removes its tag index once the last manifest is deleted.
			// Reconciliation must treat that as an empty repository inventory.
			return &TagList{Name: repository, Tags: []string{}}, nil
		}
		return nil, err
	}
	if response.Tags == nil {
		response.Tags = []string{}
	}
	return &response, nil
}

// ListTagsPage follows Distribution's native lexical tag pagination but
// returns only its marker, never its private Link header or URL.
func (c *Client) ListTagsPage(ctx context.Context, repository string, limit int, marker string) (*TagPage, error) {
	token, err := c.issueRepositoryPullToken(repository)
	if err != nil {
		return nil, err
	}
	return c.listTagsPage(ctx, repository, limit, marker, token)
}

func (c *Client) listTagsPage(ctx context.Context, repository string, limit int, marker, token string) (*TagPage, error) {
	var response TagList
	query := url.Values{"n": []string{fmt.Sprintf("%d", limit)}}
	if marker != "" {
		query.Set("last", marker)
	}
	nextPath, err := c.getWithLink(ctx, "/v2/"+repository+"/tags/list?"+query.Encode(), token, &response)
	if err != nil {
		var statusError *responseStatusError
		if errors.As(err, &statusError) && statusError.statusCode == http.StatusNotFound {
			// Distribution removes its tag index once the last manifest is deleted.
			// The logical repository still exists in Grom, so expose that state as an
			// empty tag page instead of treating it as a registry outage.
			return &TagPage{Name: repository, Tags: []string{}}, nil
		}
		return nil, err
	}
	tags := response.Tags
	if tags == nil {
		tags = []string{}
	}
	page := &TagPage{Name: response.Name, Tags: tags}
	if nextPath != "" {
		nextURL, parseErr := url.Parse(nextPath)
		if parseErr != nil {
			return nil, parseErr
		}
		page.NextMarker = nextURL.Query().Get("last")
		if page.NextMarker == "" {
			return nil, fmt.Errorf("distribution pagination link has no tag marker")
		}
	}
	return page, nil
}

func (c *Client) ListRepositoryTags(ctx context.Context, repository string) ([]string, error) {
	token, err := c.issueRepositoryPullToken(repository)
	if err != nil {
		return nil, err
	}
	tags, err := c.listTags(ctx, repository, token)
	if err != nil {
		return nil, err
	}
	return c.filterLiveTags(ctx, repository, tags.Tags, token)
}

// ListLiveTagsPage omits dangling tag links. Distribution can retain a tag in
// its tag index after the referenced manifest has been deleted by digest.
func (c *Client) ListLiveTagsPage(ctx context.Context, repository string, limit int, marker string) (*TagPage, error) {
	token, err := c.issueRepositoryPullToken(repository)
	if err != nil {
		return nil, err
	}
	result := &TagPage{Name: repository, Tags: []string{}}
	currentMarker := marker
	for len(result.Tags) < limit {
		page, err := c.listTagsPage(ctx, repository, limit-len(result.Tags), currentMarker, token)
		if err != nil {
			return nil, err
		}
		live, err := c.filterLiveTags(ctx, repository, page.Tags, token)
		if err != nil {
			return nil, err
		}
		result.Name = page.Name
		result.Tags = append(result.Tags, live...)
		if page.NextMarker == "" {
			result.NextMarker = ""
			return result, nil
		}
		if page.NextMarker == currentMarker {
			return nil, fmt.Errorf("distribution tag pagination did not advance")
		}
		currentMarker = page.NextMarker
		result.NextMarker = currentMarker
	}
	return result, nil
}

func (c *Client) filterLiveTags(ctx context.Context, repository string, tags []string, token string) ([]string, error) {
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		_, exists, err := c.resolveManifest(ctx, repository, tag, token)
		if err != nil {
			return nil, err
		}
		if exists {
			result = append(result, tag)
		}
	}
	return result, nil
}

func (c *Client) FetchManifest(ctx context.Context, repository, reference string) (*registryapp.ManifestMetadata, error) {
	token, err := c.tokens.IssueInternal("grom-internal", []registryapp.Access{{
		Type: "repository", Name: repository, Actions: []string{"pull"},
	}})
	if err != nil {
		return nil, err
	}
	return c.fetchManifestTree(ctx, repository, reference, token, map[string]bool{})
}

func (c *Client) fetchManifestTree(ctx context.Context, repository, reference, token string, visiting map[string]bool) (*registryapp.ManifestMetadata, error) {
	if visiting[reference] {
		return nil, fmt.Errorf("manifest graph contains a cycle at %s", reference)
	}
	visiting[reference] = true
	defer delete(visiting, reference)
	targetURL := c.baseURL.ResolveReference(&url.URL{Path: "/v2/" + repository + "/manifests/" + reference})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", manifestAccept)
	response, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("distribution returned %s", response.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var document manifestDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	digest := response.Header.Get("Docker-Content-Digest")
	if digest == "" {
		digest, _, err = c.ResolveManifest(ctx, repository, reference)
		if err != nil {
			return nil, err
		}
	}
	subject := ""
	if document.Subject != nil {
		subject = document.Subject.Digest
	}
	layerMediaTypes := make([]string, 0, len(document.Layers))
	descriptors := []registrydomain.Descriptor{{Digest: digest, SizeBytes: int64(len(raw)), MediaType: mediaTypeOrHeader(document.MediaType, response.Header.Get("Content-Type")), Role: "manifest"}}
	if document.Config.Digest != "" {
		descriptors = append(descriptors, registrydomain.Descriptor{Digest: document.Config.Digest, SizeBytes: document.Config.Size, MediaType: document.Config.MediaType, Role: "config"})
	}
	for _, layer := range document.Layers {
		layerMediaTypes = append(layerMediaTypes, layer.MediaType)
		descriptors = append(descriptors, registrydomain.Descriptor{Digest: layer.Digest, SizeBytes: layer.Size, MediaType: layer.MediaType, Role: "layer"})
	}
	descriptorMediaTypes := make([]string, 0, len(document.Manifests))
	platforms := make([]registrydomain.ManifestPlatform, 0, len(document.Manifests))
	children := make([]registryapp.ManifestMetadata, 0, len(document.Manifests))
	for _, descriptor := range document.Manifests {
		descriptorMediaTypes = append(descriptorMediaTypes, descriptor.MediaType)
		descriptors = append(descriptors, registrydomain.Descriptor{Digest: descriptor.Digest, SizeBytes: descriptor.Size, MediaType: descriptor.MediaType, Role: "child_manifest"})
		child, fetchErr := c.fetchManifestTree(ctx, repository, descriptor.Digest, token, visiting)
		if fetchErr != nil {
			return nil, fmt.Errorf("fetch child manifest %s: %w", descriptor.Digest, fetchErr)
		}
		platform := registrydomain.ManifestPlatform{
			OS: descriptor.Platform.OS, Architecture: descriptor.Platform.Architecture,
			Variant: descriptor.Platform.Variant, Digest: descriptor.Digest,
			CompressedSize: logicalCompressedSize(child),
		}
		if platform.OS == "" && platform.Architecture == "" && len(child.Platforms) == 1 {
			platform.OS = child.Platforms[0].OS
			platform.Architecture = child.Platforms[0].Architecture
			platform.Variant = child.Platforms[0].Variant
		}
		platforms = append(platforms, platform)
		children = append(children, *child)
	}
	if len(document.Manifests) == 0 && isImageConfigMediaType(document.Config.MediaType) && document.Config.Digest != "" {
		platform, platformErr := c.fetchImagePlatform(ctx, repository, document.Config.Digest, token)
		if platformErr != nil {
			return nil, platformErr
		}
		platform.Digest = digest
		platform.CompressedSize = compressedContentSize(document)
		platforms = append(platforms, platform)
	}
	mediaType := document.MediaType
	if mediaType == "" {
		mediaType = response.Header.Get("Content-Type")
	}
	return &registryapp.ManifestMetadata{
		Digest: digest, MediaType: mediaType, ArtifactType: document.ArtifactType,
		SubjectDigest: subject, ManifestSize: int64(len(raw)),
		ConfigMediaType: document.Config.MediaType, LayerMediaTypes: layerMediaTypes,
		DescriptorMediaTypes: descriptorMediaTypes,
		Platforms:            platforms,
		Children:             children,
		Descriptors:          descriptors,
	}, nil
}

func mediaTypeOrHeader(mediaType, header string) string {
	if mediaType != "" {
		return mediaType
	}
	return header
}

func (c *Client) fetchImagePlatform(ctx context.Context, repository, digest, token string) (registrydomain.ManifestPlatform, error) {
	targetURL := c.baseURL.ResolveReference(&url.URL{Path: "/v2/" + repository + "/blobs/" + digest})
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL.String(), nil)
	if err != nil {
		return registrydomain.ManifestPlatform{}, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := c.http.Do(request)
	if err != nil {
		return registrydomain.ManifestPlatform{}, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return registrydomain.ManifestPlatform{}, fmt.Errorf("fetch image config: distribution returned %s", response.Status)
	}
	var config imageConfigDocument
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&config); err != nil {
		return registrydomain.ManifestPlatform{}, fmt.Errorf("decode image config: %w", err)
	}
	return registrydomain.ManifestPlatform{OS: config.OS, Architecture: config.Architecture, Variant: config.Variant}, nil
}

func compressedContentSize(document manifestDocument) int64 {
	total := document.Config.Size
	for _, layer := range document.Layers {
		if layer.Size > 0 {
			total += layer.Size
		}
	}
	return total
}

func logicalCompressedSize(metadata *registryapp.ManifestMetadata) int64 {
	var total int64
	for _, platform := range metadata.Platforms {
		total += platform.CompressedSize
	}
	return total
}

func isImageConfigMediaType(mediaType string) bool {
	return mediaType == "application/vnd.oci.image.config.v1+json" ||
		mediaType == "application/vnd.docker.container.image.v1+json"
}

func (c *Client) ListReferrers(ctx context.Context, repository, digest string) ([]registryapp.ManifestDescriptor, error) {
	token, err := c.tokens.IssueInternal("grom-internal", []registryapp.Access{{
		Type: "repository", Name: repository, Actions: []string{"pull"},
	}})
	if err != nil {
		return nil, err
	}
	targetURL := c.baseURL.ResolveReference(&url.URL{Path: "/v2/" + repository + "/referrers/" + digest})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.oci.image.index.v1+json")
	response, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusNotFound {
		return []registryapp.ManifestDescriptor{}, nil
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("distribution returned %s", response.Status)
	}
	var payload referrersResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, err
	}
	result := make([]registryapp.ManifestDescriptor, 0, len(payload.Manifests))
	for _, descriptor := range payload.Manifests {
		result = append(result, registryapp.ManifestDescriptor{
			Digest: descriptor.Digest, MediaType: descriptor.MediaType,
			ArtifactType: descriptor.ArtifactType, Size: descriptor.Size,
		})
	}
	return result, nil
}

func (c *Client) RepositoryExists(ctx context.Context, repository string) (bool, error) {
	token, err := c.tokens.IssueInternal("grom-internal", []registryapp.Access{{
		Type: "repository", Name: repository, Actions: []string{"pull"},
	}})
	if err != nil {
		return false, err
	}
	targetURL := c.baseURL.ResolveReference(&url.URL{Path: "/v2/" + repository + "/tags/list"})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL.String(), nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := c.http.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = response.Body.Close() }()
	switch response.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("distribution returned %s", response.Status)
	}
}

func (c *Client) ManifestExists(ctx context.Context, repository, reference string) (bool, error) {
	_, exists, err := c.ResolveManifest(ctx, repository, reference)
	return exists, err
}

func (c *Client) ResolveManifest(ctx context.Context, repository, reference string) (string, bool, error) {
	token, err := c.issueRepositoryPullToken(repository)
	if err != nil {
		return "", false, err
	}
	return c.resolveManifest(ctx, repository, reference, token)
}

func (c *Client) issueRepositoryPullToken(repository string) (string, error) {
	return c.tokens.IssueInternal("grom-internal", []registryapp.Access{{
		Type: "repository", Name: repository, Actions: []string{"pull"},
	}})
}

func (c *Client) resolveManifest(ctx context.Context, repository, reference, token string) (string, bool, error) {
	targetURL := c.baseURL.ResolveReference(&url.URL{Path: "/v2/" + repository + "/manifests/" + reference})
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, targetURL.String(), nil)
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", manifestAccept)
	response, err := c.http.Do(req)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = response.Body.Close() }()
	switch response.StatusCode {
	case http.StatusOK:
		digest := response.Header.Get("Docker-Content-Digest")
		if digest == "" {
			return "", false, fmt.Errorf("distribution omitted manifest digest")
		}
		return digest, true, nil
	case http.StatusNotFound:
		return "", false, nil
	default:
		return "", false, fmt.Errorf("distribution returned %s", response.Status)
	}
}

var manifestAccept = strings.Join([]string{
	"application/vnd.oci.image.index.v1+json",
	"application/vnd.oci.image.manifest.v1+json",
	"application/vnd.oci.artifact.manifest.v1+json",
	"application/vnd.docker.distribution.manifest.list.v2+json",
	"application/vnd.docker.distribution.manifest.v2+json",
}, ", ")

func (c *Client) DeleteManifest(ctx context.Context, repository, digest string) error {
	token, err := c.tokens.IssueInternal("grom-internal", []registryapp.Access{{
		Type: "repository", Name: repository, Actions: []string{"delete"},
	}})
	if err != nil {
		return err
	}
	targetURL := c.baseURL.ResolveReference(&url.URL{Path: "/v2/" + repository + "/manifests/" + digest})
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, targetURL.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusAccepted && response.StatusCode != http.StatusNotFound {
		return fmt.Errorf("distribution returned %s", response.Status)
	}
	return nil
}

func (c *Client) get(ctx context.Context, path, token string, target any) error {
	_, err := c.getWithLink(ctx, path, token, target)
	return err
}

func (c *Client) getWithLink(ctx context.Context, path, token string, target any) (string, error) {
	reference, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	targetURL := c.baseURL.ResolveReference(reference)
	if targetURL.Scheme != c.baseURL.Scheme || targetURL.Host != c.baseURL.Host {
		return "", fmt.Errorf("distribution pagination link points to a different origin")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return "", &responseStatusError{statusCode: response.StatusCode, status: response.Status}
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return "", err
	}
	nextPath, err := nextLinkPath(response.Header.Get("Link"))
	if err != nil {
		return "", err
	}
	return nextPath, nil
}

func nextLinkPath(header string) (string, error) {
	for _, rawLink := range strings.Split(header, ",") {
		parts := strings.Split(rawLink, ";")
		if len(parts) < 2 {
			continue
		}
		isNext := false
		for _, parameter := range parts[1:] {
			if strings.TrimSpace(parameter) == `rel="next"` {
				isNext = true
				break
			}
		}
		if !isNext {
			continue
		}
		rawTarget := strings.TrimSpace(parts[0])
		if len(rawTarget) < 2 || rawTarget[0] != '<' || rawTarget[len(rawTarget)-1] != '>' {
			return "", fmt.Errorf("distribution returned an invalid pagination link")
		}
		target, err := url.Parse(rawTarget[1 : len(rawTarget)-1])
		if err != nil {
			return "", fmt.Errorf("parse distribution pagination link: %w", err)
		}
		return target.String(), nil
	}
	return "", nil
}
