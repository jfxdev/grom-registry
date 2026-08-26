// @vitest-environment jsdom

import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import PrincipalTypeBadge from './PrincipalTypeBadge.vue'

describe('PrincipalTypeBadge', () => {
  it('labels a user principal', () => {
    const wrapper = mount(PrincipalTypeBadge, { props: { kind: 'user' } })

    expect(wrapper.text()).toContain('User')
    expect(wrapper.get('.lucide-user-round')).toBeTruthy()
  })

  it('labels a service account principal', () => {
    const wrapper = mount(PrincipalTypeBadge, { props: { kind: 'service_account' } })

    expect(wrapper.text()).toContain('Service account')
    expect(wrapper.get('.lucide-bot')).toBeTruthy()
  })

  it('can render an icon-only accessible badge', () => {
    const wrapper = mount(PrincipalTypeBadge, { props: { kind: 'user', iconOnly: true } })

    expect(wrapper.text()).toBe('')
    expect(wrapper.attributes('aria-label')).toBe('User')
    expect(wrapper.get('.lucide-user-round')).toBeTruthy()
  })
})
