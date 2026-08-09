// @vitest-environment jsdom

import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { nextTick } from 'vue'
import DropdownMenu from './DropdownMenu.vue'

describe('DropdownMenu', () => {
  it('opens a connected menu and lets an item close it', async () => {
    const wrapper = mount(DropdownMenu, {
      props: { label: 'Set role', ariaLabel: 'Change role for alex' },
      slots: {
        icon: '<span class="trigger-icon">icon</span>',
        default: `<template #default="{ close }"><button class="dropdown-menu-item" @click="close">Viewer</button></template>`,
      },
    })

    const trigger = wrapper.get('button[aria-label="Change role for alex"]')
    expect(trigger.attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('[role="menu"]').exists()).toBe(false)

    await trigger.trigger('click')
    expect(trigger.classes()).toContain('dropdown-menu-trigger-open')
    expect(wrapper.get('[role="menu"]').text()).toContain('Viewer')

    await wrapper.get('.dropdown-menu-item').trigger('click')
    expect(wrapper.find('[role="menu"]').exists()).toBe(false)
  })

  it('returns focus to the trigger when pressing Escape from a menu item', async () => {
    const wrapper = mount(DropdownMenu, {
      attachTo: document.body,
      props: { label: 'Set role', ariaLabel: 'Change role' },
      slots: { default: '<button class="dropdown-menu-item">Viewer</button>' },
    })
    const trigger = wrapper.get('button')
    await trigger.trigger('click')
    const item = wrapper.get('.dropdown-menu-item')
    ;(item.element as HTMLButtonElement).focus()
    await document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.find('[role="menu"]').exists()).toBe(false)
    await nextTick()
    expect(document.activeElement).toBe(trigger.element)

    await trigger.trigger('click')
    await document.body.dispatchEvent(new MouseEvent('pointerdown', { bubbles: true }))
    expect(wrapper.find('[role="menu"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
