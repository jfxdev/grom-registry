// @vitest-environment jsdom

import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ActionButton from './ActionButton.vue'

describe('ActionButton', () => {
  it('uses the raised green action variant', () => {
    const wrapper = mount(ActionButton, {
      slots: { default: 'Create project' },
    })

    expect(wrapper.get('button').classes()).toContain('grom-button-variant-default')
    expect(wrapper.text()).toBe('Create project')
  })

  it('forwards state and click events', async () => {
    const onClick = vi.fn()
    const wrapper = mount(ActionButton, {
      props: { loading: true },
      attrs: { onClick },
    })

    expect(wrapper.get('button').attributes('aria-busy')).toBe('true')
    expect(wrapper.get('button').attributes()).toHaveProperty('disabled')

    await wrapper.setProps({ loading: false })
    await wrapper.get('button').trigger('click')

    expect(onClick).toHaveBeenCalledOnce()
  })
})
