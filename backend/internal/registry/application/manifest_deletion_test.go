package application

import (
	"reflect"
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
