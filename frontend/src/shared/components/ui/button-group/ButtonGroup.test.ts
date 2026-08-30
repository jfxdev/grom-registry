// @vitest-environment jsdom

import { mount } from '@vue/test-utils'
import { h } from 'vue'
import { describe, expect, it } from 'vitest'
import ButtonGroup from './ButtonGroup.vue'

describe('ButtonGroup', () => {
  it('groups related controls with an accessible group role', () => {
    const wrapper = mount(ButtonGroup, {
      attrs: { 'aria-label': 'Date filter' },
      slots: { default: () => [h('button', 'Choose date'), h('button', 'Clear date')] },
    })

    expect(wrapper.attributes('role')).toBe('group')
    expect(wrapper.attributes('aria-label')).toBe('Date filter')
    expect(wrapper.findAll('button')).toHaveLength(2)
  })
})
