package application

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/jfxdev/grom/backend/internal/constants"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
)

func TestDeletionChildDigestsIncludesOnlyUnreferencedLiveDescendants(t *testing.T) {
	tests := []struct {
		name      string
		inventory []registrydomain.ManifestInventory
		want      []string
	}{
		{
			name: "nested orphaned children",
			inventory: []registrydomain.ManifestInventory{
				manifestWithChildren("sha256:index", "sha256:child"),
				manifestWithChildren("sha256:child", "sha256:grandchild"),
				liveManifest("sha256:grandchild"),
			},
			want: []string{"sha256:child", "sha256:grandchild"},
		},
		{
			name: "shared child and its descendants are preserved",
			inventory: []registrydomain.ManifestInventory{
				manifestWithChildren("sha256:index", "sha256:child"),
				manifestWithChildren("sha256:other-index", "sha256:child"),
				manifestWithChildren("sha256:child", "sha256:grandchild"),
				liveManifest("sha256:grandchild"),
			},
			want: []string{},
		},
		{
			name: "tagged child is preserved",
			inventory: []registrydomain.ManifestInventory{
				manifestWithChildren("sha256:index", "sha256:child"),
				func() registrydomain.ManifestInventory {
					child := liveManifest("sha256:child")
					child.Tags = []string{"platform-amd64"}
					return child
				}(),
			},
			want: []string{},
		},
		{
			name: "child with OCI referrer is preserved",
			inventory: []registrydomain.ManifestInventory{
				manifestWithChildren("sha256:index", "sha256:child"),
				liveManifest("sha256:child"),
				{Digest: "sha256:signature", SubjectDigest: "sha256:child", State: constants.InventoryStateUntagged},
			},
			want: []string{},
		},
		{
			name: "child that is itself an OCI referrer is preserved",
			inventory: []registrydomain.ManifestInventory{
				manifestWithChildren("sha256:index", "sha256:signature"),
				{Digest: "sha256:signature", SubjectDigest: "sha256:subject", State: constants.InventoryStateUntagged},
			},
			want: []string{},
		},
		{
			name: "historical child is not sent to Distribution",
			inventory: []registrydomain.ManifestInventory{
				manifestWithChildren("sha256:index", "sha256:child"),
				{Digest: "sha256:child", State: constants.InventoryStateMissing},
			},
			want: []string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := deletionChildDigests("sha256:index", test.inventory); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("deletion children = %#v, want %#v", got, test.want)
			}
		})
	}
}

func manifestWithChildren(digest string, children ...string) registrydomain.ManifestInventory {
	manifest := liveManifest(digest)
	for _, child := range children {
		manifest.Platforms = append(manifest.Platforms, registrydomain.ManifestPlatform{Digest: child})
	}
	return manifest
}

func liveManifest(digest string) registrydomain.ManifestInventory {
	return registrydomain.ManifestInventory{Digest: digest, State: constants.InventoryStateUntagged}
}

type changingIndexDistribution struct {
	listCalls int
	deleted   []string
}

func (d *changingIndexDistribution) ListRepositoryTags(context.Context, string) ([]string, error) {
	d.listCalls++
	if d.listCalls > 1 {
		return []string{"target", "moved"}, nil
	}
	return []string{"target"}, nil
}

func (d *changingIndexDistribution) ResolveManifest(_ context.Context, _, reference string) (string, bool, error) {
	switch reference {
	case "target":
		return "sha256:target", true, nil
	case "moved":
		return "sha256:child", true, nil
	case "sha256:target", "sha256:child":
		return reference, true, nil
	default:
		return "", false, nil
	}
}

func (d *changingIndexDistribution) FetchManifest(_ context.Context, _, reference string) (*ManifestMetadata, error) {
	metadata := &ManifestMetadata{Digest: reference}
	if reference == "sha256:target" {
		metadata.Platforms = []registrydomain.ManifestPlatform{{Digest: "sha256:child"}}
	}
	return metadata, nil
}

func (d *changingIndexDistribution) ListReferrers(context.Context, string, string) ([]ManifestDescriptor, error) {
	return nil, nil
}

func (d *changingIndexDistribution) DeleteManifest(_ context.Context, _, digest string) error {
	d.deleted = append(d.deleted, digest)
	return nil
}

func TestDeleteManifestTreeRefreshesLiveTagsBeforeEachChild(t *testing.T) {
	distribution := &changingIndexDistribution{}
	inventory := []registrydomain.ManifestInventory{
		manifestWithChildren("sha256:target", "sha256:child"),
		liveManifest("sha256:child"),
	}

	deleted, err := deleteManifestTree(
		context.Background(), distribution, "project/api", "sha256:target",
		[]string{"sha256:child"}, inventory,
	)

	if err == nil || !strings.Contains(err.Error(), "child manifest sha256:child gained tag moved") {
		t.Fatalf("expected the moved tag to block child deletion, got %v", err)
	}
	if !reflect.DeepEqual(deleted, []string{"sha256:target"}) || !reflect.DeepEqual(distribution.deleted, deleted) {
		t.Fatalf("child deletion was not stopped: returned=%#v deleted=%#v", deleted, distribution.deleted)
	}
	if distribution.listCalls != 2 {
		t.Fatalf("live tags were listed %d times, want initial and pre-child snapshots", distribution.listCalls)
	}
}
