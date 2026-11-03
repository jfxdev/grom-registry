import type { Project } from '@/shared/api/models'

export const PROJECT_MODE_LABELS: Record<Project['mode'], string> = {
  standard: 'Standard · push and pull',
  pull_through: 'Pull-through · read-only mirror',
}

export const PROJECT_UPSTREAM_LABELS: Record<NonNullable<Project['upstream']>, string> = {
  dockerhub: 'Docker Hub',
  ghcr: 'GitHub Container Registry',
}
