import { useState, useRef, useEffect } from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Slider } from '@/components/ui/slider'
import type { gui } from '../../wailsjs/go/models'
import type { model } from '../../wailsjs/go/models'
import type { Theme } from '@/hooks/use-store'

interface MainPageProps {
  loginStatus: gui.LoginStatus
  mountStatus: gui.MountStatus | null
  projects: model.Project[]
  logs: string[]
  loading: string
  error: string
  theme: Theme
  fontSize: number
  setTheme: (t: Theme) => void
  setFontSize: (s: number) => void
  refreshProjects: () => Promise<void>
  mount: (project: string, mountpoint: string, batch: boolean) => Promise<void>
  unmount: () => Promise<void>
  sync: () => Promise<void>
  openMountpoint: () => Promise<void>
  clearLogs: () => void
  clearError: () => void
  onLogout: () => void
}

export function MainPage({
  loginStatus,
  mountStatus,
  projects,
  logs,
  loading,
  error,
  theme,
  fontSize,
  setTheme,
  setFontSize,
  refreshProjects,
  mount,
  unmount,
  sync,
  openMountpoint,
  clearLogs,
  clearError,
  onLogout,
}: MainPageProps) {
  const [selectedProject, setSelectedProject] = useState<string | null>('__all__')
  const [mountpoint, setMountpoint] = useState('~/downleaf')
  const [batchMode, setBatchMode] = useState(false)
  const logEndRef = useRef<HTMLDivElement>(null)
  const isMounted = mountStatus?.mounted ?? false

  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [logs])

  const handleMount = async () => {
    clearError()
    const proj = selectedProject === '__all__' ? '' : (selectedProject ?? '')
    await mount(proj, mountpoint, batchMode)
  }

  return (
    <div className="flex flex-col h-full">
      {/* ===== Top Bar ===== */}
      <header
        className="flex items-center justify-between px-5 py-3 border-b bg-card"
        data-wails-drag
      >
        <div className="flex items-center gap-3">
          <span className="text-lg font-semibold tracking-tight select-none">
            Downleaf
          </span>
          <Badge variant="secondary" className="text-xs font-normal">
            v0.1.0
          </Badge>
        </div>

        {/* Mount controls at top */}
        <div className="flex items-center gap-2">
          {isMounted ? (
            <>
              <Badge variant="outline" className="gap-1.5 text-xs font-normal border-sage/40 text-sage">
                <span className="w-1.5 h-1.5 rounded-full bg-sage inline-block" />
                Mounted
              </Badge>
              {mountStatus?.batchMode && (
                <Button
                  size="sm"
                  variant="secondary"
                  disabled={loading === 'sync'}
                  onClick={sync}
                >
                  {loading === 'sync' ? 'Syncing...' : 'Sync'}
                </Button>
              )}
              <Button size="sm" variant="secondary" onClick={openMountpoint}>
                Open Folder
              </Button>
              <Button
                size="sm"
                variant="destructive"
                disabled={loading === 'unmount'}
                onClick={unmount}
              >
                {loading === 'unmount' ? 'Unmounting...' : 'Unmount'}
              </Button>
            </>
          ) : (
            <Button size="sm" disabled={loading === 'mount'} onClick={handleMount}>
              {loading === 'mount' ? 'Mounting...' : 'Mount'}
            </Button>
          )}
        </div>

        <div className="flex items-center gap-2">
          <SettingsDialog
            theme={theme}
            fontSize={fontSize}
            setTheme={setTheme}
            setFontSize={setFontSize}
          />

          <DropdownMenu>
            <DropdownMenuTrigger className="inline-flex items-center gap-2 text-xs px-3 py-1.5 rounded-md hover:bg-muted transition-colors cursor-pointer">
              <span className="w-1.5 h-1.5 rounded-full bg-sage inline-block" />
              {loginStatus.email}
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={onLogout} className="text-destructive">
                Log out
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </header>

      {/* ===== Main Content ===== */}
      <div className="flex-1 overflow-y-auto">
        <div className="max-w-3xl mx-auto px-5 py-5 space-y-4">
          {/* Mount Config */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm font-medium">Mount Configuration</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-1.5">
                  <Label className="text-xs">Project</Label>
                  <Select
                    value={selectedProject ?? '__all__'}
                    onValueChange={(val) => setSelectedProject(val)}
                    disabled={isMounted}
                  >
                    <SelectTrigger className="h-9 w-full">
                      <SelectValue placeholder="All Projects" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="__all__">All Projects</SelectItem>
                      {projects.map((p) => (
                        <SelectItem key={p._id} value={p.name}>
                          {p.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-1.5">
                  <Label className="text-xs">Mountpoint</Label>
                  <Input
                    value={mountpoint}
                    onChange={(e) => setMountpoint(e.target.value)}
                    disabled={isMounted}
                    className="h-9"
                  />
                </div>
              </div>
              <div className="flex items-center gap-2">
                <Switch
                  id="batch"
                  checked={batchMode}
                  onCheckedChange={setBatchMode}
                  disabled={isMounted}
                />
                <Label htmlFor="batch" className="text-xs text-muted-foreground cursor-pointer">
                  Batch mode — write locally, push with Sync
                </Label>
              </div>

              {isMounted && mountStatus && (
                <div className="text-xs text-sage px-3 py-2 rounded-md bg-sage-soft/30 border border-sage/20">
                  Mounted at <span className="font-medium">{mountStatus.mountpoint}</span>
                  {mountStatus.project
                    ? <> &middot; project: <span className="font-medium">{mountStatus.project}</span></>
                    : <> &middot; all projects</>
                  }
                </div>
              )}

              {error && (
                <div className="text-xs text-destructive px-3 py-2 rounded-md bg-destructive/10 border border-destructive/20">
                  {error}
                </div>
              )}
            </CardContent>
          </Card>

          {/* Project List */}
          <Card>
            <CardHeader className="pb-3 flex-row items-center justify-between">
              <CardTitle className="text-sm font-medium">
                Projects
                <span className="ml-2 text-xs text-muted-foreground font-normal">
                  ({projects.length})
                </span>
              </CardTitle>
              <Button variant="ghost" size="sm" className="text-xs h-7" onClick={refreshProjects}>
                Refresh
              </Button>
            </CardHeader>
            <CardContent className="pt-0">
              {projects.length === 0 ? (
                <p className="text-sm text-muted-foreground py-4 text-center">
                  No projects found.
                </p>
              ) : (
                <ScrollArea className="max-h-[240px]">
                  <div className="space-y-0.5">
                    {projects.map((p) => (
                      <div
                        key={p._id}
                        className="flex items-center justify-between px-3 py-2 rounded-md hover:bg-muted/50 transition-colors"
                      >
                        <div className="min-w-0">
                          <p className="text-sm font-medium truncate">{p.name}</p>
                          <p className="text-[11px] text-muted-foreground font-mono truncate">
                            {p._id}
                          </p>
                        </div>
                        <Badge variant="secondary" className="text-[10px] shrink-0 ml-2">
                          {p.accessLevel}
                        </Badge>
                      </div>
                    ))}
                  </div>
                </ScrollArea>
              )}
            </CardContent>
          </Card>
        </div>
      </div>

      {/* ===== Log Panel ===== */}
      <div className="border-t bg-card">
        <div className="flex items-center justify-between px-5 py-2">
          <span className="text-xs font-medium text-muted-foreground">Logs</span>
          <Button variant="ghost" size="sm" className="text-xs h-6 px-2" onClick={clearLogs}>
            Clear
          </Button>
        </div>
        <Separator />
        <ScrollArea className="h-[140px] px-5 py-2">
          <pre className="text-[11px] leading-relaxed text-muted-foreground font-mono whitespace-pre-wrap break-all">
            {logs.join('\n')}
          </pre>
          <div ref={logEndRef} />
        </ScrollArea>
      </div>
    </div>
  )
}

/* ===== Settings Dialog ===== */

function SettingsDialog({
  theme,
  fontSize,
  setTheme,
  setFontSize,
}: {
  theme: Theme
  fontSize: number
  setTheme: (t: Theme) => void
  setFontSize: (s: number) => void
}) {
  return (
    <Dialog>
      <DialogTrigger className="inline-flex items-center text-xs px-3 py-1.5 rounded-md hover:bg-muted transition-colors cursor-pointer">
        Settings
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Settings</DialogTitle>
        </DialogHeader>
        <div className="space-y-6 pt-2">
          {/* Theme */}
          <div className="space-y-2">
            <Label className="text-sm">Theme</Label>
            <div className="flex gap-2">
              {(['light', 'dark', 'system'] as const).map((t) => (
                <Button
                  key={t}
                  size="sm"
                  variant={theme === t ? 'default' : 'outline'}
                  className="flex-1 capitalize text-xs"
                  onClick={() => setTheme(t)}
                >
                  {t}
                </Button>
              ))}
            </div>
          </div>

          <Separator />

          {/* Font Size */}
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <Label className="text-sm">Font Size</Label>
              <span className="text-xs text-muted-foreground tabular-nums">{fontSize}px</span>
            </div>
            <Slider
              min={11}
              max={20}
              step={1}
              value={[fontSize]}
              onValueChange={(val) => {
                const v = Array.isArray(val) ? val[0] : val
                setFontSize(v)
              }}
            />
            <div className="flex justify-between text-[10px] text-muted-foreground">
              <span>Small</span>
              <span>Large</span>
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
