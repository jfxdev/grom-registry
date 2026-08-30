// @vitest-environment jsdom

import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import { afterEach, beforeAll, describe, expect, it } from 'vitest'
import Select from './Select.vue'

describe('Select', () => {
  let mounted: VueWrapper | undefined

  beforeAll(() => {
    HTMLElement.prototype.scrollIntoView = () => {}
  })

  afterEach(async () => {
    mounted?.unmount()
    mounted = undefined
    await nextTick()
    document.body.replaceChildren()
  })

  it('displays and emits the logical value for an empty option', async () => {
    mounted = mount(Select, {
      attachTo: document.body,
      props: {
        modelValue: '',
        ariaLabel: 'Filter events',
        options: [
          { value: '', label: 'All events' },
          { value: 'created', label: 'Created' },
        ],
      },
    })
    const wrapper = mounted!

    const input = wrapper.get('input[aria-label="Filter events"]')
    await flushPromises()
    expect((input.element as HTMLInputElement).value).toBe('All events')

    await input.trigger('focus')
    await new Promise((resolve) => window.setTimeout(resolve, 0))
    expect((input.element as HTMLInputElement).value).toBe('')
    await input.setValue('cre')
    await flushPromises()
    const created = Array.from(document.querySelectorAll<HTMLElement>('[role="option"]'))
      .find((option) => option.textContent?.includes('Created'))
    expect(created).toBeDefined()
    expect(document.querySelector('[role="option"]')?.textContent).toContain('Created')
    expect(created?.querySelector('.grom-select-item-label')?.getAttribute('title')).toBe('Created')
    created!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    await flushPromises()

    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual(['created'])
  })
})
