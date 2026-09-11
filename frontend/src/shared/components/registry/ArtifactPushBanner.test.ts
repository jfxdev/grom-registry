// @vitest-environment jsdom

import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ArtifactPushBanner from './ArtifactPushBanner.vue'

describe('ArtifactPushBanner', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('wraps the command in a bounded command region and copies it', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    const wrapper = mount(ArtifactPushBanner, {
      props: {
        registryHost: 'registry.example.test',
        project: 'docker',
        repository: 'base-images/forgejo',
        profile: 'container_image' as const,
      },
    })

    expect(wrapper.get('.terminal-command').text()).toContain(
      'docker push registry.example.test/docker/base-images/forgejo:tag',
    )

    await wrapper.get('button[aria-label="Copy push command"]').trigger('click')
    await flushPromises()

    expect(writeText).toHaveBeenCalledWith('docker push registry.example.test/docker/base-images/forgejo:tag')
    expect(wrapper.get('button[aria-label="Copied"]').text()).toContain('Copied')
  })

  it('offers an ORAS module push and a Terraform follow-up for a Terraform repository', () => {
    const wrapper = mount(ArtifactPushBanner, {
      props: {
        registryHost: 'registry.example.test',
        project: 'platform',
        repository: 'modules/vpc',
        profile: 'terraform_module' as const,
      },
    })

    const command = wrapper.get('.terminal-command').text()
    expect(command).toContain('oras push registry.example.test/platform/modules/vpc:tag')
    expect(command).toContain('--artifact-type application/vnd.opentofu.modulepkg')
    expect(wrapper.text()).toContain('oci:// source in Terraform 1.14')
    expect(wrapper.text()).not.toContain('docker push')
  })

  it('falls back to ORAS for an unclassified generic artifact repository', () => {
    const wrapper = mount(ArtifactPushBanner, {
      props: {
        registryHost: 'registry.example.test',
        project: 'platform',
        repository: 'bundles/policy',
        profile: 'generic_oci' as const,
      },
    })

    expect(wrapper.get('.terminal-command').text()).toContain(
      'oras push registry.example.test/platform/bundles/policy:tag',
    )
  })

  it('defaults an empty project-level banner to Docker', () => {
    const wrapper = mount(ArtifactPushBanner, {
      props: { registryHost: 'registry.example.test', project: 'platform' },
    })

    expect(wrapper.get('.terminal-command').text()).toContain('docker push')
    expect(wrapper.text()).toContain('a repository in this project')
  })
})
