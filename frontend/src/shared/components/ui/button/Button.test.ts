// @vitest-environment jsdom

import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import Button from './Button.vue'

describe('Button', () => {
  it('uses the raised default variant and size classes', () => {
    const wrapper = mount(Button, { slots: { default: 'Create project' } })

    expect(wrapper.classes()).toContain('grom-button-variant-default')
    expect(wrapper.classes()).toContain('grom-button-size-default')
    expect(wrapper.text()).toBe('Create project')
  })

  it('exposes and enforces its loading state', () => {
    const wrapper = mount(Button, {
      props: { loading: true },
      slots: { default: 'Signing in…' },
    })

    expect(wrapper.attributes('aria-busy')).toBe('true')
    expect(wrapper.attributes()).toHaveProperty('disabled')
  })

  it('keeps variant and size modifiers independent', () => {
    const wrapper = mount(Button, {
      props: { variant: 'outline', size: 'sm' },
      slots: { default: 'Keys' },
    })

    expect(wrapper.classes()).toContain('grom-button-variant-outline')
    expect(wrapper.classes()).toContain('grom-button-size-sm')
    expect(wrapper.classes()).not.toContain('grom-button-variant-default')
  })
})
