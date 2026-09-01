// @vitest-environment jsdom

import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import TerminalCommand from './TerminalCommand.vue'

describe('TerminalCommand', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders a bounded terminal command and copies its full value', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    const command = 'docker push registry.example.test/docker/base-images/forgejo:tag'
    const wrapper = mount(TerminalCommand, {
      props: { command, ariaLabel: 'Copy example command' },
    })

    expect(wrapper.get('.terminal-command-title').text()).toContain('Terminal')
    expect(wrapper.get('pre').text()).toContain(command)

    await wrapper.get('button[aria-label="Copy example command"]').trigger('click')
    await flushPromises()

    expect(writeText).toHaveBeenCalledWith(command)
    expect(wrapper.get('button[aria-label="Copy example command"]').text()).toContain('Copied')
  })
})
