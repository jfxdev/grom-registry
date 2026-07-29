package domain

import (
	"time"

	"github.com/jfxdev/grom/backend/internal/constants"
)

func ApplyInferredProfile(repository *Repository, candidate, confidence string, inferredAt time.Time) bool {
	if candidate == "" || candidate == constants.RepositoryProfileUnknown ||
		repository.Profile == constants.RepositoryProfileMixed {
		return false
	}
	if repository.Profile == "" || repository.Profile == constants.RepositoryProfileUnknown {
		setInferredProfile(repository, candidate, confidence, inferredAt, false)
		return true
	}
	if repository.Profile == candidate {
		if confidenceRank(confidence) > confidenceRank(repository.ProfileConfidence) {
			setInferredProfile(repository, candidate, confidence, inferredAt, repository.ProfileNeedsReview)
			return true
		}
		return false
	}
	if confidenceRank(confidence) > confidenceRank(repository.ProfileConfidence) {
		setInferredProfile(repository, candidate, confidence, inferredAt, false)
		return true
	}
	if repository.Profile == constants.RepositoryProfileGenericOCI &&
		confidenceRank(repository.ProfileConfidence) < confidenceRank(confidence) {
		setInferredProfile(repository, candidate, confidence, inferredAt, false)
		return true
	}
	if candidate == constants.RepositoryProfileGenericOCI &&
		confidenceRank(confidence) <= confidenceRank(repository.ProfileConfidence) {
		return false
	}
	setInferredProfile(repository, constants.RepositoryProfileMixed, constants.ClassificationConfidenceHigh, inferredAt, true)
	return true
}

func setInferredProfile(repository *Repository, profile, confidence string, inferredAt time.Time, needsReview bool) {
	repository.Profile = profile
	repository.ProfileSource = constants.ProfileSourceInferred
	repository.ProfileConfidence = confidence
	repository.ProfileInferredAt = &inferredAt
	repository.ProfileNeedsReview = needsReview
	repository.UpdatedAt = inferredAt
}

func confidenceRank(confidence string) int {
	switch confidence {
	case constants.ClassificationConfidenceHigh:
		return 3
	case constants.ClassificationConfidenceMedium:
		return 2
	case constants.ClassificationConfidenceLow:
		return 1
	default:
		return 0
	}
}
