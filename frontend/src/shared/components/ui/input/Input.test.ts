// @vitest-environment jsdom

import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import Input from './Input.vue'

describe('Input', () => {
  it('emits model updates from the native input', async () => {
    const wrapper = mount(Input, { props: { modelValue: '' } })

    await wrapper.get('input').setValue('admin@grom.local')

    expect(wrapper.emitted('update:modelValue')).toEqual([['admin@grom.local']])
  })

  it('forwards accessibility attributes to the native input', () => {
    const wrapper = mount(Input, {
      attrs: { 'aria-invalid': 'true', id: 'email' },
    })

    expect(wrapper.get('input').attributes('aria-invalid')).toBe('true')
    expect(wrapper.get('input').attributes('id')).toBe('email')
  })

  it('renders an optional trailing action', () => {
    const wrapper = mount(Input, {
      slots: {
        trailing: '<button type="button" aria-label="Show password">Show</button>',
      },
    })

    expect(wrapper.get('button').attributes('aria-label')).toBe('Show password')
    expect(wrapper.classes()).toContain('input-shell-trailing')
  })
})
