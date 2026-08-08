// @vitest-environment jsdom

import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import DeleteButton from './DeleteButton.vue'

describe('DeleteButton', () => {
  it('uses the raised red danger variant with a trash icon', () => {
    const wrapper = mount(DeleteButton)

    expect(wrapper.get('button').classes()).toContain('grom-button-variant-delete')
    expect(wrapper.get('.lucide-trash-2')).toBeTruthy()
    expect(wrapper.text()).toBe('Delete')
  })

  it('supports custom text and forwards state and click events', async () => {
    const onClick = vi.fn()
    const wrapper = mount(DeleteButton, {
      props: { loading: true },
      attrs: { onClick },
      slots: { default: 'Delete project' },
    })

    expect(wrapper.get('button').attributes('aria-busy')).toBe('true')
    expect(wrapper.get('button').attributes()).toHaveProperty('disabled')
    expect(wrapper.text()).toBe('Delete project')

    await wrapper.setProps({ loading: false })
    await wrapper.get('button').trigger('click')

    expect(onClick).toHaveBeenCalledOnce()
  })

  it('renders only the trash icon at icon size', () => {
    const wrapper = mount(DeleteButton, {
      props: { size: 'icon' },
      attrs: { 'aria-label': 'Delete account' },
    })

    expect(wrapper.get('button').attributes('aria-label')).toBe('Delete account')
    expect(wrapper.get('.lucide-trash-2')).toBeTruthy()
    expect(wrapper.text()).toBe('')
  })

  it('forwards the compact size to the shared button', () => {
    const wrapper = mount(DeleteButton, { props: { size: 'sm' } })

    expect(wrapper.get('button').classes()).toContain('grom-button-size-sm')
  })
})
