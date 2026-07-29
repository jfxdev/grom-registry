import { createSession, deleteSession, getCurrentUser } from '@/modules/auth/api/session'
import type { User } from '@/shared/api/models'
import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useSessionStore = defineStore('session', () => {
  const user = ref<User | null>(null)
  const checked = ref(false)

  async function restore() {
    try {
      user.value = await getCurrentUser()
    } catch {
      user.value = null
    } finally {
      checked.value = true
    }
  }

  async function signIn(email: string, password: string) {
    user.value = await createSession({ email, password })
    checked.value = true
  }

  async function signOut() {
    try {
      await deleteSession()
    } finally {
      user.value = null
      checked.value = true
    }
  }

  return { user, checked, restore, signIn, signOut }
})
