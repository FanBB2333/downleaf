import { useState, useEffect, useCallback } from 'react'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import {
  GetLoginStatus,
  GetMountStatus,
  GetLogs,
  GetEnvDefaults,
  GetVersion,
  Login,
  ListProjects,
  ListTags,
  Mount,
  Unmount,
  Sync,
  OpenMountpoint,
  IsBrowserLoginSupported,
  ListCredentials,
  LoginWithBrowser,
  LoginWithCredential,
  DeleteCredential,
} from '../../wailsjs/go/gui/App'
import type { gui, credential } from '../../wailsjs/go/models'
import type { model } from '../../wailsjs/go/models'

export type Theme = 'light' | 'dark' | 'system'
export type ColorScheme = 'classic' | 'sage' | 'rose' | 'blue' | 'lavender'

function getSystemTheme(): 'light' | 'dark' {
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function applyTheme(theme: Theme) {
  const resolved = theme === 'system' ? getSystemTheme() : theme
  document.documentElement.classList.toggle('dark', resolved === 'dark')
}

function applyColorScheme(scheme: ColorScheme) {
  document.documentElement.setAttribute('data-scheme', scheme)
}

export function useStore() {
  const [loginStatus, setLoginStatus] = useState<gui.LoginStatus | null>(null)
  const [mountStatus, setMountStatus] = useState<gui.MountStatus | null>(null)
  const [projects, setProjects] = useState<model.Project[]>([])
  const [tags, setTags] = useState<model.Tag[]>([])
  const [logs, setLogs] = useState<string[]>([])
  const [loading, setLoading] = useState('')
  const [error, setError] = useState('')
  const [version, setVersion] = useState('')
  const [envDefaults, setEnvDefaults] = useState<Record<string, string>>({})
  const [browserLoginSupported, setBrowserLoginSupported] = useState(false)
  const [savedCredentials, setSavedCredentials] = useState<credential.CredentialInfo[]>([])
  const [theme, setThemeState] = useState<Theme>(() => {
    return (localStorage.getItem('downleaf-theme') as Theme) || 'light'
  })
  const [colorScheme, setColorSchemeState] = useState<ColorScheme>(() => {
    return (localStorage.getItem('downleaf-color-scheme') as ColorScheme) || 'classic'
  })
  const [fontSize, setFontSizeState] = useState(() => {
    return parseInt(localStorage.getItem('downleaf-fontsize') || '14', 10)
  })

  const setTheme = useCallback((t: Theme) => {
    setThemeState(t)
    localStorage.setItem('downleaf-theme', t)
    applyTheme(t)
  }, [])

  const setColorScheme = useCallback((s: ColorScheme) => {
    setColorSchemeState(s)
    localStorage.setItem('downleaf-color-scheme', s)
    applyColorScheme(s)
  }, [])

  const setFontSize = useCallback((s: number) => {
    setFontSizeState(s)
    localStorage.setItem('downleaf-fontsize', String(s))
    document.documentElement.style.fontSize = `${s}px`
  }, [])

  // Init
  useEffect(() => {
    applyTheme(theme)
    applyColorScheme(colorScheme)
    document.documentElement.style.fontSize = `${fontSize}px`

    GetVersion().then(setVersion).catch(() => {})
    GetEnvDefaults().then(setEnvDefaults).catch(() => {})
    IsBrowserLoginSupported().then(setBrowserLoginSupported).catch(() => {})
    ListCredentials().then((creds) => setSavedCredentials(creds || [])).catch(() => {})
    GetLoginStatus().then((s) => {
      if (s.loggedIn) {
        setLoginStatus(s)
        ListProjects().then(setProjects).catch(() => {})
        ListTags().then(t => setTags(t || [])).catch(() => {})
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
      const [p, t] = await Promise.all([ListProjects(), ListTags()])
      setProjects(p || [])
      setTags(t || [])
      await GetMountStatus().then(setMountStatus)
      // Refresh credentials list after manual login
      ListCredentials().then((creds) => setSavedCredentials(creds || [])).catch(() => {})
    } catch (e: unknown) {
      setError(String(e))
      throw e
    } finally {
      setLoading('')
    }
  }, [])

  const loginWithBrowser = useCallback(async (site: string) => {
    setLoading('browser-login')
    setError('')
    try {
      const s = await LoginWithBrowser(site)
      setLoginStatus(s)
      const [p, t] = await Promise.all([ListProjects(), ListTags()])
      setProjects(p || [])
      setTags(t || [])
      await GetMountStatus().then(setMountStatus)
      // Refresh credentials list
      ListCredentials().then((creds) => setSavedCredentials(creds || [])).catch(() => {})
    } catch (e: unknown) {
      setError(String(e))
      throw e
    } finally {
      setLoading('')
    }
  }, [])

  const loginWithCredential = useCallback(async (id: string) => {
    setLoading('credential-login')
    setError('')
    try {
      const s = await LoginWithCredential(id)
      setLoginStatus(s)
      const [p, t] = await Promise.all([ListProjects(), ListTags()])
      setProjects(p || [])
      setTags(t || [])
      await GetMountStatus().then(setMountStatus)
      // Refresh credentials list to update lastUsedAt
      ListCredentials().then((creds) => setSavedCredentials(creds || [])).catch(() => {})
    } catch (e: unknown) {
      setError(String(e))
      throw e
    } finally {
      setLoading('')
    }
  }, [])

  const deleteCredential = useCallback(async (id: string) => {
    try {
      await DeleteCredential(id)
      setSavedCredentials((prev) => prev.filter((c) => c.id !== id))
    } catch (e: unknown) {
      setError(String(e))
    }
  }, [])

  const refreshProjects = useCallback(async () => {
    try {
      const [p, t] = await Promise.all([ListProjects(), ListTags()])
      setProjects(p || [])
      setTags(t || [])
    } catch (e: unknown) {
      setError(String(e))
    }
  }, [])

  const mount = useCallback(async (projectNames: string[], mountpoint: string, zenMode: boolean) => {
    setLoading('mount')
    setError('')
    try {
      await Mount(projectNames, mountpoint, zenMode)
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

  const logout = useCallback(() => {
    setLoginStatus(null)
    setProjects([])
    setMountStatus(null)
    setLogs([])
    setError('')
  }, [])

  const clearLogs = useCallback(() => setLogs([]), [])
  const clearError = useCallback(() => setError(''), [])

  return {
    version,
    loginStatus,
    mountStatus,
    projects,
    tags,
    logs,
    loading,
    error,
    envDefaults,
    browserLoginSupported,
    savedCredentials,
    theme,
    colorScheme,
    fontSize,
    setTheme,
    setColorScheme,
    setFontSize,
    login,
    loginWithBrowser,
    loginWithCredential,
    deleteCredential,
    refreshProjects,
    mount,
    unmount,
    sync,
    openMountpoint,
    logout,
    clearLogs,
    clearError,
  }
}
