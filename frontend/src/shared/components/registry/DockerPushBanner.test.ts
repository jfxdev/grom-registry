// @vitest-environment jsdom

import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import DockerPushBanner from './DockerPushBanner.vue'

describe('DockerPushBanner', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('wraps the command in a bounded command region and copies it', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    const wrapper = mount(DockerPushBanner, {
      props: {
        registryHost: 'registry.example.test',
        project: 'docker',
        repository: 'base-images/forgejo',
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
})
