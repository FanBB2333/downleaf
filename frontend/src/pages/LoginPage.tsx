import { useState } from 'react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Button } from '@/components/ui/button'

interface LoginPageProps {
  envDefaults: Record<string, string>
  loading: string
  onLogin: (site: string, cookies: string) => Promise<void>
}

export function LoginPage({ envDefaults, loading, onLogin }: LoginPageProps) {
  const [site, setSite] = useState(envDefaults.site || '')
  const [cookies, setCookies] = useState(envDefaults.cookies || '')
  const [err, setErr] = useState('')

  const handleSubmit = async (e: React.FormEvent) => {
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

  return (
    <div className="flex flex-col h-full">
      {/* Drag region — fills the macOS titlebar inset area */}
      <div className="h-8 shrink-0" style={{ WebkitAppRegion: 'drag' } as React.CSSProperties} />

      <div className="flex-1 flex items-center justify-center px-4 -mt-8">
        <Card className="w-full max-w-lg">
          <CardHeader className="text-center pb-2">
            <CardTitle className="text-2xl font-semibold tracking-tight">
              Downleaf
            </CardTitle>
            <CardDescription className="text-muted-foreground">
              Connect to your Overleaf instance
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleSubmit} className="space-y-5">
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

              <div className="space-y-2">
                <Label htmlFor="cookies">Session Cookie</Label>
                <Textarea
                  id="cookies"
                  placeholder="overleaf_session2=s%3A..."
                  rows={5}
                  value={cookies}
                  onChange={(e) => setCookies(e.target.value)}
                  className="font-mono text-xs resize-none"
                />
                <p className="text-xs text-muted-foreground">
                  Open DevTools in your browser &rarr; Application &rarr; Cookies, copy the full cookie string.
                </p>
              </div>

              {err && (
                <p className="text-sm text-destructive">{err}</p>
              )}

              <Button
                type="submit"
                className="w-full h-11"
                disabled={loading === 'login'}
              >
                {loading === 'login' ? 'Connecting...' : 'Connect'}
              </Button>
            </form>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
