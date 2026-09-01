// @vitest-environment jsdom

import { mount, type VueWrapper } from '@vue/test-utils'
import { h, nextTick } from 'vue'
import { afterEach, describe, expect, it } from 'vitest'
import Popover from './Popover.vue'

describe('Popover', () => {
  let mounted: VueWrapper | undefined

  afterEach(async () => {
    mounted?.unmount()
    mounted = undefined
    await nextTick()
    document.body.replaceChildren()
  })

  function mountPopover() {
    mounted = mount(Popover, {
      attachTo: document.body,
      props: { ariaLabel: 'Choose a value' },
      slots: {
        trigger: () => h('button', 'Open'),
        default: ({ close }: { close: () => void }) => h('button', { class: 'option', onClick: close }, 'Pick me'),
      },
    })
    return mounted
  }

  it('starts closed and opens when the trigger toggles', async () => {
    const wrapper = mountPopover()
    expect(document.querySelector('.popover-content')).toBeNull()

    await wrapper.get('button').trigger('click')
    const content = document.querySelector('.popover-content')
    expect(content).not.toBeNull()
    expect(content?.closest('[data-v-app]')).toBeNull()
  })

  it('closes when a click lands outside the popover', async () => {
    const wrapper = mountPopover()
    await wrapper.get('button').trigger('click')
    expect(document.querySelector('.popover-content')).not.toBeNull()

    await new Promise((resolve) => window.setTimeout(resolve, 0))
    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    await wrapper.vm.$nextTick()
    await wrapper.vm.$nextTick()
    await new Promise((resolve) => window.setTimeout(resolve, 0))
    expect(document.querySelector('.popover-content')).toBeNull()
  })

  it('closes on Escape and restores focus to the trigger', async () => {
    const wrapper = mountPopover()
    const trigger = wrapper.get('button').element as InstanceType<typeof globalThis.HTMLElement>
    trigger.focus()
    await wrapper.get('button').trigger('click')

    const option = document.querySelector('.option') as InstanceType<typeof globalThis.HTMLElement>
    option.focus()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await wrapper.vm.$nextTick()
    await wrapper.vm.$nextTick()
    await new Promise((resolve) => window.setTimeout(resolve, 0))

    expect(document.querySelector('.popover-content')).toBeNull()
    expect(document.activeElement).toBe(trigger)
  })
})
