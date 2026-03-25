import { useState, useEffect, useCallback } from 'react'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import {
  GetLoginStatus,
  GetMountStatus,
  GetLogs,
  GetEnvDefaults,
  Login,
  ListProjects,
  Mount,
  Unmount,
  Sync,
  OpenMountpoint,
} from '../../wailsjs/go/gui/App'
import type { gui } from '../../wailsjs/go/models'
import type { model } from '../../wailsjs/go/models'

export type Theme = 'light' | 'dark' | 'system'

function getSystemTheme(): 'light' | 'dark' {
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function applyTheme(theme: Theme) {
  const resolved = theme === 'system' ? getSystemTheme() : theme
  document.documentElement.classList.toggle('dark', resolved === 'dark')
}

export function useStore() {
  const [loginStatus, setLoginStatus] = useState<gui.LoginStatus | null>(null)
  const [mountStatus, setMountStatus] = useState<gui.MountStatus | null>(null)
  const [projects, setProjects] = useState<model.Project[]>([])
  const [logs, setLogs] = useState<string[]>([])
  const [loading, setLoading] = useState('')
  const [error, setError] = useState('')
  const [envDefaults, setEnvDefaults] = useState<Record<string, string>>({})
  const [theme, setThemeState] = useState<Theme>(() => {
    return (localStorage.getItem('downleaf-theme') as Theme) || 'light'
  })
  const [fontSize, setFontSizeState] = useState(() => {
    return parseInt(localStorage.getItem('downleaf-fontsize') || '14', 10)
  })

  const setTheme = useCallback((t: Theme) => {
    setThemeState(t)
    localStorage.setItem('downleaf-theme', t)
    applyTheme(t)
  }, [])

  const setFontSize = useCallback((s: number) => {
    setFontSizeState(s)
    localStorage.setItem('downleaf-fontsize', String(s))
    document.documentElement.style.fontSize = `${s}px`
  }, [])

  // Init
  useEffect(() => {
    applyTheme(theme)
    document.documentElement.style.fontSize = `${fontSize}px`

    GetEnvDefaults().then(setEnvDefaults).catch(() => {})
    GetLoginStatus().then((s) => {
      if (s.loggedIn) {
        setLoginStatus(s)
        ListProjects().then(setProjects).catch(() => {})
      }
    }).catch(() => {})
    GetMountStatus().then(setMountStatus).catch(() => {})
    GetLogs().then(setLogs).catch(() => {})

    const offLog = EventsOn('log', (line: string) => {
      setLogs((prev) => [...prev.slice(-499), line])
    })
    const offMount = EventsOn('mountStatusChanged', () => {
      GetMountStatus().then(setMountStatus).catch(() => {})
    })

    // Listen for system theme changes
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const handler = () => {
      const stored = localStorage.getItem('downleaf-theme') as Theme
      if (stored === 'system') applyTheme('system')
    }
    mq.addEventListener('change', handler)

    return () => {
      offLog()
      offMount()
      mq.removeEventListener('change', handler)
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const login = useCallback(async (site: string, cookies: string) => {
    setLoading('login')
    setError('')
    try {
      const s = await Login(site, cookies)
      setLoginStatus(s)
      const p = await ListProjects()
      setProjects(p || [])
      await GetMountStatus().then(setMountStatus)
    } catch (e: unknown) {
      setError(String(e))
      throw e
    } finally {
      setLoading('')
    }
  }, [])

  const refreshProjects = useCallback(async () => {
    try {
      const p = await ListProjects()
      setProjects(p || [])
    } catch (e: unknown) {
      setError(String(e))
    }
  }, [])

  const mount = useCallback(async (projectName: string, mountpoint: string, batchMode: boolean) => {
    setLoading('mount')
    setError('')
    try {
      await Mount(projectName, mountpoint, batchMode)
      await GetMountStatus().then(setMountStatus)
    } catch (e: unknown) {
      setError(String(e))
    } finally {
      setLoading('')
    }
  }, [])

  const unmount = useCallback(async () => {
    setLoading('unmount')
    setError('')
    try {
      await Unmount()
      await GetMountStatus().then(setMountStatus)
    } catch (e: unknown) {
      setError(String(e))
    } finally {
      setLoading('')
    }
  }, [])

  const sync = useCallback(async () => {
    setLoading('sync')
    try {
      const msg = await Sync()
      setError(msg)
    } catch (e: unknown) {
      setError(String(e))
    } finally {
      setLoading('')
    }
  }, [])

  const openMountpoint = useCallback(async () => {
    try {
      await OpenMountpoint()
    } catch {
      // ignore
    }
  }, [])

  const clearLogs = useCallback(() => setLogs([]), [])
  const clearError = useCallback(() => setError(''), [])

  return {
    loginStatus,
    mountStatus,
    projects,
    logs,
    loading,
    error,
    envDefaults,
    theme,
    fontSize,
    setTheme,
    setFontSize,
    login,
    refreshProjects,
    mount,
    unmount,
    sync,
    openMountpoint,
    clearLogs,
    clearError,
  }
}
