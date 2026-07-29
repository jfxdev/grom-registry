package domain

import (
	"testing"
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
)

func TestApplyInferredProfileEvolution(t *testing.T) {
	now := time.Now().UTC()
	repository := &Repository{
		Profile:           constants.RepositoryProfileUnknown,
		ProfileSource:     constants.ProfileSourceNone,
		ProfileConfidence: constants.ClassificationConfidenceNone,
	}
	if !ApplyInferredProfile(repository, constants.RepositoryProfileGenericOCI, constants.ClassificationConfidenceLow, now) {
		t.Fatal("expected initial inference")
	}
	if !ApplyInferredProfile(repository, constants.RepositoryProfileTerraform, constants.ClassificationConfidenceHigh, now.Add(time.Minute)) {
		t.Fatal("expected a stronger specific classification to replace generic OCI")
	}
	if repository.Profile != constants.RepositoryProfileTerraform || repository.ProfileNeedsReview {
		t.Fatalf("unexpected upgraded profile: %#v", repository)
	}
	if ApplyInferredProfile(repository, constants.RepositoryProfileGenericOCI, constants.ClassificationConfidenceLow, now.Add(2*time.Minute)) {
		t.Fatal("weak generic content must not replace a specific profile")
	}
	if !ApplyInferredProfile(repository, constants.RepositoryProfileContainerImage, constants.ClassificationConfidenceHigh, now.Add(3*time.Minute)) {
		t.Fatal("expected conflicting primary content to change the profile")
	}
	if repository.Profile != constants.RepositoryProfileMixed || !repository.ProfileNeedsReview {
		t.Fatalf("expected mixed profile requiring review: %#v", repository)
	}
}

func TestStrongerInferenceReplacesAnUncertainSpecificProfile(t *testing.T) {
	now := time.Now().UTC()
	repository := &Repository{
		Profile:           constants.RepositoryProfileContainerImage,
		ProfileSource:     constants.ProfileSourceInferred,
		ProfileConfidence: constants.ClassificationConfidenceMedium,
	}
	if !ApplyInferredProfile(repository, constants.RepositoryProfileTerraform, constants.ClassificationConfidenceHigh, now) {
		t.Fatal("expected stronger evidence to replace an uncertain inference")
	}
	if repository.Profile != constants.RepositoryProfileTerraform || repository.ProfileNeedsReview {
		t.Fatalf("unexpected corrected profile: %#v", repository)
	}
}
