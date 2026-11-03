import { useSyncExternalStore } from 'react'

export type ThemePreference = 'system' | 'light' | 'dark'

export const THEME_STORAGE_KEY = 'vp-theme'
export const themePreferences: readonly ThemePreference[] = ['system', 'light', 'dark']

const listeners = new Set<() => void>()

function isThemePreference(value: unknown): value is ThemePreference {
  return value === 'system' || value === 'light' || value === 'dark'
}

function readStored(): ThemePreference {
  try {
    const value = localStorage.getItem(THEME_STORAGE_KEY)
    return isThemePreference(value) ? value : 'system'
  } catch {
    return 'system'
  }
}

let current: ThemePreference = readStored()

/** Writes the preference onto <html>; "system" leaves the media query in charge. */
export function applyTheme(preference: ThemePreference): void {
  const root = document.documentElement
  if (preference === 'system') {
    delete root.dataset.theme
  } else {
    root.dataset.theme = preference
  }
}

export function getTheme(): ThemePreference {
  return current
}

export function setTheme(preference: ThemePreference): void {
  current = preference
  try {
    localStorage.setItem(THEME_STORAGE_KEY, preference)
  } catch {
    // Storage may be unavailable (private mode); the in-memory choice still applies.
  }
  applyTheme(preference)
  listeners.forEach((listener) => listener())
}

/** Re-reads storage; used at startup and by tests that reset localStorage. */
export function initTheme(): ThemePreference {
  current = readStored()
  applyTheme(current)
  return current
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

export function useTheme(): ThemePreference {
  return useSyncExternalStore(subscribe, getTheme, getTheme)
}
