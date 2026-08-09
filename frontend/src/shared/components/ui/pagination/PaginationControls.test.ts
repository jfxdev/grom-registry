// @vitest-environment jsdom

import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import PaginationControls from './PaginationControls.vue'

describe('PaginationControls', () => {
  it('does not invent a total when the server does not provide one', () => {
    const wrapper = mount(PaginationControls, {
      props: { page: 2, hasPrevious: true, hasNext: true },
    })

    expect(wrapper.text()).toContain('Page 2')
    expect(wrapper.text()).not.toContain('of 1')
  })

  it('displays a valid known total', () => {
    const wrapper = mount(PaginationControls, {
      props: { page: 2, pageCount: 3, hasPrevious: true, hasNext: true },
    })

    expect(wrapper.text()).toContain('Page 2 of 3')
  })
})
