import { useState } from 'react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Button } from '@/components/ui/button'
import { useWindowDrag } from '@/hooks/use-window-drag'
import { ChevronDown, ChevronRight, Globe, Trash2, User } from 'lucide-react'
import type { credential } from '../../wailsjs/go/models'

interface LoginPageProps {
  envDefaults: Record<string, string>
  loading: string
  browserLoginSupported: boolean
  savedCredentials: credential.CredentialInfo[]
  onLogin: (site: string, cookies: string) => Promise<void>
  onLoginWithBrowser: (site: string) => Promise<void>
  onLoginWithCredential: (id: string) => Promise<void>
  onDeleteCredential: (id: string) => Promise<void>
}

function formatLastUsed(dateStr: string): string {
  const date = new Date(dateStr)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffMins = Math.floor(diffMs / 60000)
  const diffHours = Math.floor(diffMs / 3600000)
  const diffDays = Math.floor(diffMs / 86400000)

  if (diffMins < 1) return 'Just now'
  if (diffMins < 60) return `${diffMins}m ago`
  if (diffHours < 24) return `${diffHours}h ago`
  if (diffDays < 7) return `${diffDays}d ago`
  return date.toLocaleDateString()
}

export function LoginPage({
  envDefaults,
  loading,
  browserLoginSupported,
  savedCredentials,
  onLogin,
  onLoginWithBrowser,
  onLoginWithCredential,
  onDeleteCredential,
}: LoginPageProps) {
  const [site, setSite] = useState(envDefaults.site || 'https://www.overleaf.com')
  const [cookies, setCookies] = useState(envDefaults.cookies || '')
  const [err, setErr] = useState('')
  const [manualExpanded, setManualExpanded] = useState(false)
  const [deletingId, setDeletingId] = useState<string | null>(null)
  const onDragMouseDown = useWindowDrag()

  const handleManualSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setErr('')
    if (!site.trim() || !cookies.trim()) {
      setErr('Please fill in both fields.')
      return
    }
    try {
      await onLogin(site.trim(), cookies.trim())
    } catch (ex: unknown) {
      setErr(String(ex))
    }
  }

  const handleBrowserLogin = async () => {
    setErr('')
    if (!site.trim()) {
      setErr('Please enter a site URL.')
      return
    }
    try {
      await onLoginWithBrowser(site.trim())
    } catch (ex: unknown) {
      setErr(String(ex))
    }
  }

  const handleCredentialLogin = async (id: string) => {
    setErr('')
    try {
      await onLoginWithCredential(id)
    } catch (ex: unknown) {
      setErr(String(ex))
    }
  }

  const handleDeleteCredential = async (id: string) => {
    if (deletingId === id) {
      // Confirm delete
      await onDeleteCredential(id)
      setDeletingId(null)
    } else {
      // First click - ask for confirmation
      setDeletingId(id)
      // Reset after 3 seconds if not confirmed
      setTimeout(() => setDeletingId((curr) => (curr === id ? null : curr)), 3000)
    }
  }

  const isLoading = loading === 'login' || loading === 'browser-login' || loading === 'credential-login'

  return (
    <div className="flex flex-col h-full">
      {/* Drag region — fills the macOS titlebar inset area */}
      <div className="h-8 shrink-0 cursor-default relative z-10" onMouseDown={onDragMouseDown} />

      <div className="flex-1 flex items-center justify-center px-4 -mt-8 overflow-y-auto py-4">
        <Card className="w-full max-w-lg">
          <CardHeader className="text-center pb-2">
            <CardTitle className="text-2xl font-semibold tracking-tight">
              Downleaf
            </CardTitle>
            <CardDescription className="text-muted-foreground">
              Connect to your Overleaf instance
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-5">
            {/* Site URL input - shared between all login methods */}
            <div className="space-y-2">
              <Label htmlFor="site">Site URL</Label>
              <Input
                id="site"
                type="url"
                placeholder="https://overleaf.example.com"
                value={site}
                onChange={(e) => setSite(e.target.value)}
                className="h-11"
              />
            </div>

            {/* Saved Accounts section */}
            {savedCredentials.length > 0 && (
              <div className="space-y-3">
                <Label className="text-sm font-medium">Saved Accounts</Label>
                <div className="space-y-2">
                  {savedCredentials.map((cred) => (
                    <div
                      key={cred.id}
                      className="flex items-center gap-3 p-3 rounded-lg border bg-card hover:bg-accent/50 transition-colors group"
                    >
                      <button
                        className="flex-1 flex items-center gap-3 text-left"
                        onClick={() => handleCredentialLogin(cred.id)}
                        disabled={isLoading}
                      >
                        <div className="w-10 h-10 rounded-full bg-primary/10 flex items-center justify-center shrink-0">
                          <User className="w-5 h-5 text-primary" />
                        </div>
                        <div className="flex-1 min-w-0">
                          <div className="font-medium truncate">{cred.email}</div>
                          <div className="text-xs text-muted-foreground flex items-center gap-1">
                            <Globe className="w-3 h-3" />
                            <span className="truncate">{cred.siteURL}</span>
                            <span className="text-muted-foreground/60">·</span>
                            <span>{formatLastUsed(cred.lastUsedAt)}</span>
                          </div>
                        </div>
                      </button>
                      <Button
                        variant={deletingId === cred.id ? 'destructive' : 'ghost'}
                        size="icon"
                        className="shrink-0 opacity-0 group-hover:opacity-100 transition-opacity"
                        onClick={() => handleDeleteCredential(cred.id)}
                        disabled={isLoading}
                      >
                        <Trash2 className="w-4 h-4" />
                      </Button>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {/* Browser Login button - primary action on macOS */}
            {browserLoginSupported && (
              <Button
                type="button"
                className="w-full h-11"
                onClick={handleBrowserLogin}
                disabled={isLoading}
              >
                {loading === 'browser-login' ? 'Opening Browser...' : 'Login with Browser'}
              </Button>
            )}

            {/* Collapsible Manual Cookie Login */}
            <div className="border rounded-lg">
              <button
                type="button"
                className="w-full flex items-center justify-between p-3 text-sm font-medium text-muted-foreground hover:text-foreground transition-colors"
                onClick={() => setManualExpanded(!manualExpanded)}
              >
                <span>Manual Cookie Login</span>
                {manualExpanded ? (
                  <ChevronDown className="w-4 h-4" />
                ) : (
                  <ChevronRight className="w-4 h-4" />
                )}
              </button>
              {manualExpanded && (
                <form onSubmit={handleManualSubmit} className="px-3 pb-3 space-y-4">
                  <div className="space-y-2">
                    <Label htmlFor="cookies">Session Cookie</Label>
                    <Textarea
                      id="cookies"
                      placeholder="overleaf_session2=s%3A..."
                      rows={4}
                      value={cookies}
                      onChange={(e) => setCookies(e.target.value)}
                      className="font-mono text-xs resize-none"
                    />
                    <p className="text-xs text-muted-foreground">
                      Open DevTools in your browser &rarr; Application &rarr; Cookies, copy the full cookie string.
                    </p>
                  </div>
                  <Button
                    type="submit"
                    variant="secondary"
                    className="w-full"
                    disabled={isLoading}
                  >
                    {loading === 'login' ? 'Connecting...' : 'Connect with Cookie'}
                  </Button>
                </form>
              )}
            </div>

            {err && (
              <p className="text-sm text-destructive">{err}</p>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
