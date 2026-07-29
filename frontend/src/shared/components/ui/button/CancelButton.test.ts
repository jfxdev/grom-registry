// @vitest-environment jsdom

import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import CancelButton from './CancelButton.vue'

describe('CancelButton', () => {
  it('uses the raised gray variant with a cancel icon', () => {
    const wrapper = mount(CancelButton)

    expect(wrapper.get('button').classes()).toContain('grom-button-variant-secondary')
    expect(wrapper.get('.lucide-x')).toBeTruthy()
    expect(wrapper.text()).toBe('Cancel')
  })

  it('forwards click events', async () => {
    const onClick = vi.fn()
    const wrapper = mount(CancelButton, { attrs: { onClick } })

    await wrapper.get('button').trigger('click')

    expect(onClick).toHaveBeenCalledOnce()
  })
})
