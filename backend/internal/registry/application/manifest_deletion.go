package application

import (
	"context"
	"fmt"

	"github.com/jfxdev/grom/backend/internal/constants"
	registrydomain "github.com/jfxdev/grom/backend/internal/registry/domain"
)

func deletionChildDigests(targetDigest string, inventory []registrydomain.ManifestInventory) []string {
	byDigest := make(map[string]registrydomain.ManifestInventory, len(inventory))
	for _, manifest := range inventory {
		byDigest[manifest.Digest] = manifest
	}
	ordered := []string{targetDigest}
	closure := map[string]struct{}{targetDigest: {}}
	for index := 0; index < len(ordered); index++ {
		manifest, exists := byDigest[ordered[index]]
		if !exists {
			continue
		}
		for _, platform := range manifest.Platforms {
			if platform.Digest == "" || platform.Digest == manifest.Digest {
				continue
			}
			if _, exists := closure[platform.Digest]; exists {
				continue
			}
			closure[platform.Digest] = struct{}{}
			ordered = append(ordered, platform.Digest)
		}
	}

	for changed := true; changed; {
		changed = false
		for digest := range closure {
			if digest == targetDigest {
				continue
			}
			manifest, exists := byDigest[digest]
			if !exists || manifest.State == constants.InventoryStateDeleted || manifest.State == constants.InventoryStateMissing {
				delete(closure, digest)
				changed = true
				continue
			}
			if len(manifest.Tags) > 0 || manifest.SubjectDigest != "" || hasExternalManifestReference(digest, closure, inventory) {
				delete(closure, digest)
				changed = true
			}
		}
	}

	result := make([]string, 0, len(closure)-1)
	for _, digest := range ordered[1:] {
		if _, exists := closure[digest]; exists {
			result = append(result, digest)
		}
	}
	return result
}

func hasExternalManifestReference(
	digest string,
	deletionSet map[string]struct{},
	inventory []registrydomain.ManifestInventory,
) bool {
	for _, manifest := range inventory {
		if manifest.State == constants.InventoryStateDeleted || manifest.State == constants.InventoryStateMissing {
			continue
		}
		if manifest.SubjectDigest == digest {
			return true
		}
		if _, beingDeleted := deletionSet[manifest.Digest]; beingDeleted {
			continue
		}
		for _, platform := range manifest.Platforms {
			if platform.Digest == digest {
				return true
			}
		}
	}
	return false
}

func deleteManifestTree(
	ctx context.Context,
	distribution DistributionMetadata,
	repository, targetDigest string,
	children []string,
) ([]string, error) {
	if err := revalidateDeletionChildren(ctx, distribution, repository, children); err != nil {
		return nil, err
	}
	digests := append([]string{targetDigest}, children...)
	deleted := make([]string, 0, len(digests))
	for index, digest := range digests {
		resolved, exists, err := distribution.ResolveManifest(ctx, repository, digest)
		if err != nil {
			return deleted, err
		}
		if !exists || resolved != digest {
			if index == 0 {
				return deleted, fmt.Errorf("artifact changed or no longer exists")
			}
			continue
		}
		if err := distribution.DeleteManifest(ctx, repository, digest); err != nil {
			return deleted, err
		}
		deleted = append(deleted, digest)
	}
	return deleted, nil
}

func revalidateDeletionChildren(
	ctx context.Context,
	distribution DistributionMetadata,
	repository string,
	children []string,
) error {
	if len(children) == 0 {
		return nil
	}
	childSet := make(map[string]struct{}, len(children))
	for _, digest := range children {
		childSet[digest] = struct{}{}
	}
	tags, err := distribution.ListRepositoryTags(ctx, repository)
	if err != nil {
		return fmt.Errorf("revalidate child manifest tags: %w", err)
	}
	for _, tag := range tags {
		digest, exists, resolveErr := distribution.ResolveManifest(ctx, repository, tag)
		if resolveErr != nil {
			return fmt.Errorf("revalidate child manifest tag %s: %w", tag, resolveErr)
		}
		if _, planned := childSet[digest]; exists && planned {
			return fmt.Errorf("child manifest %s gained tag %s; review the deletion again", digest, tag)
		}
	}
	for _, digest := range children {
		metadata, fetchErr := distribution.FetchManifest(ctx, repository, digest)
		if fetchErr != nil {
			return fmt.Errorf("revalidate child manifest %s: %w", digest, fetchErr)
		}
		if metadata.SubjectDigest != "" {
			return fmt.Errorf("child manifest %s is an OCI referrer; review the deletion again", digest)
		}
		referrers, referrerErr := distribution.ListReferrers(ctx, repository, digest)
		if referrerErr != nil {
			return fmt.Errorf("revalidate child manifest referrers %s: %w", digest, referrerErr)
		}
		if len(referrers) > 0 {
			return fmt.Errorf("child manifest %s gained OCI referrers; review the deletion again", digest)
		}
	}
	return nil
}
