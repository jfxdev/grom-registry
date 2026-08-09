// @vitest-environment jsdom

import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { afterEach, describe, expect, it } from 'vitest'
import Dialog from './Dialog.vue'

describe('Dialog', () => {
  afterEach(() => {
    document.body.replaceChildren()
  })

  it('returns focus to the triggering control when it closes', async () => {
    const trigger = document.createElement('button')
    trigger.textContent = 'Delete v1'
    document.body.append(trigger)
    trigger.focus()

    const wrapper = mount(Dialog, {
      attachTo: document.body,
      props: { labelledBy: 'dialog-title' },
      slots: { default: '<button aria-label="Close deletion">Close</button>' },
    })
    await nextTick()

    wrapper.unmount()

    expect(document.activeElement).toBe(trigger)
  })
})
