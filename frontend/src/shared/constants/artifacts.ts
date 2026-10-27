import type { ArtifactKind, RepositoryProfile } from '@/shared/api/models'

export const REPOSITORY_PROFILE_LABELS: Record<RepositoryProfile, string> = {
  unknown: 'Unknown',
  container_image: 'Container image',
  opentofu_module: 'OpenTofu module',
  helm_chart: 'Helm chart',
  wasm: 'WebAssembly',
  sbom: 'SBOM',
  generic_oci: 'Generic OCI artifact',
  mixed: 'Mixed',
}

export const ARTIFACT_KIND_LABELS: Record<ArtifactKind, string> = {
  container_image: 'Container image',
  image_index: 'Image index',
  opentofu_module: 'OpenTofu module',
  helm_chart: 'Helm chart',
  wasm: 'WebAssembly',
  sbom_spdx: 'SBOM (SPDX)',
  sbom_cyclonedx: 'SBOM (CycloneDX)',
  signature: 'Signature',
  generic_oci: 'Generic OCI artifact',
  unknown_oci: 'Unrecognised OCI artifact',
}

export function repositoryProfileLabel(profile: RepositoryProfile): string {
  return REPOSITORY_PROFILE_LABELS[profile] ?? profile.replaceAll('_', ' ')
}

export function artifactKindLabel(kind: ArtifactKind): string {
  return ARTIFACT_KIND_LABELS[kind] ?? kind.replaceAll('_', ' ')
}

/** The pieces of an OCI reference, kept apart because Helm addresses the
 *  repository and the version separately while Docker and ORAS do not. */
export type ArtifactReference = {
  /** Registry host, for example registry.example.com. */
  host: string
  /** Repository path below the host, for example my-project/my-module. */
  path: string
  /** Tag or version. */
  tag: string
}

type ArtifactRecipe = {
  /** Heading for the push panel. */
  title: string
  /** One line of context under the heading. */
  description: string
  pushCommand: (reference: ArtifactReference) => string
  pullCommand: (reference: ArtifactReference) => string
  /** Shown under the push command when the tool needs a follow-up step. */
  note?: string
}

function tagged(reference: ArtifactReference): string {
  return `${reference.host}/${reference.path}:${reference.tag}`
}

const DOCKER: ArtifactRecipe = {
  title: 'Push an image',
  description: 'Push a tagged local image to this repository.',
  pushCommand: reference => `docker push ${tagged(reference)}`,
  pullCommand: reference => `docker pull ${tagged(reference)}`,
}

const ORAS: ArtifactRecipe = {
  title: 'Push an artifact',
  description: 'Push any OCI artifact to this repository with ORAS.',
  pushCommand: reference => `oras push ${tagged(reference)} artifact.tgz:archive/tar+gzip`,
  pullCommand: reference => `oras pull ${tagged(reference)}`,
}

const OPENTOFU: ArtifactRecipe = {
  title: 'Push a module',
  description: 'Publish an OpenTofu module package to this repository.',
  pushCommand: reference =>
    `oras push ${tagged(reference)} \\\n  --artifact-type application/vnd.opentofu.modulepkg \\\n  module.tgz:archive/tar+gzip`,
  pullCommand: reference => `oras pull ${tagged(reference)}`,
  note: 'Consume the module from an oci:// module source in OpenTofu.',
}

const HELM: ArtifactRecipe = {
  title: 'Push a chart',
  description: 'Publish a packaged Helm chart to this repository.',
  pushCommand: reference => `helm push chart.tgz oci://${reference.host}/${parentPath(reference.path)}`,
  pullCommand: reference => `helm pull oci://${reference.host}/${reference.path} --version ${reference.tag}`,
}

// No artifact type is asserted here: the WebAssembly toolchains have not settled
// on one, while application/wasm is the registered media type for the binary.
const WASM: ArtifactRecipe = {
  title: 'Push a module',
  description: 'Publish a WebAssembly module or component to this repository.',
  pushCommand: reference => `oras push ${tagged(reference)} module.wasm:application/wasm`,
  pullCommand: reference => `oras pull ${tagged(reference)}`,
}

function parentPath(path: string): string {
  const segments = path.split('/')
  return segments.length > 1 ? segments.slice(0, -1).join('/') : path
}

const PROFILE_RECIPES: Record<RepositoryProfile, ArtifactRecipe> = {
  unknown: DOCKER,
  container_image: DOCKER,
  opentofu_module: OPENTOFU,
  helm_chart: HELM,
  wasm: WASM,
  sbom: ORAS,
  generic_oci: ORAS,
  mixed: ORAS,
}

/**
 * A repository's profile is inferred from what has actually been pushed, so it
 * is also the best guide to which client the next person will reach for. An
 * empty repository has nothing to infer from and defaults to Docker.
 */
export function artifactRecipe(profile: RepositoryProfile): ArtifactRecipe {
  return PROFILE_RECIPES[profile] ?? ORAS
}

export function artifactPushCommand(reference: ArtifactReference, profile: RepositoryProfile): string {
  return artifactRecipe(profile).pushCommand(reference)
}

export function artifactPullCommand(reference: ArtifactReference, profile: RepositoryProfile): string {
  return artifactRecipe(profile).pullCommand(reference)
}
