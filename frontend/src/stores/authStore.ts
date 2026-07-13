import { create } from 'zustand'
import { setAccessToken } from '../api/client'
import type { User } from '../api/types'

interface AuthState {
  user: User | null
  ready: boolean
  setAuth: (user: User, accessToken: string, refreshToken: string) => void
  clearAuth: () => void
  setReady: () => void
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  ready: false,

  setAuth: (user, accessToken, refreshToken) => {
    setAccessToken(accessToken)
    localStorage.setItem('refresh_token', refreshToken)
    set({ user })
  },

  clearAuth: () => {
    setAccessToken(null)
    localStorage.removeItem('refresh_token')
    set({ user: null })
  },

  setReady: () => set({ ready: true }),
}))
