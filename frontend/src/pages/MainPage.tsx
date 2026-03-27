import { useState, useRef, useEffect } from 'react'
import { RotateCw, Terminal, Search, Folder, Library } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'
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

/* Shorthand for the drag / no-drag inline style */
const DRAG: React.CSSProperties = { WebkitAppRegion: 'drag', '--wails-draggable': 'drag' } as React.CSSProperties
const NO_DRAG: React.CSSProperties = { WebkitAppRegion: 'no-drag', '--wails-draggable': 'no-drag' } as React.CSSProperties

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
  mount: (projects: string[], mountpoint: string, zenMode: boolean) => Promise<void>
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
  const [selectedProjects, setSelectedProjects] = useState<string[]>([])
  const [searchQuery, setSearchQuery] = useState('')
  const [mountpoint, setMountpoint] = useState('~/downleaf')
  const [zenMode, setZenMode] = useState(false)
  const logEndRef = useRef<HTMLDivElement>(null)
  const isMounted = mountStatus?.mounted ?? false

  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [logs])

  const handleProjectClick = (name: string) => {
    if (isMounted) return
    if (name === '__all__') {
      setSelectedProjects([])
      return
    }
    setSelectedProjects(prev => {
      if (prev.includes(name)) {
        return prev.filter(p => p !== name)
      } else {
        return [...prev, name]
      }
    })
  }

  const handleMount = async () => {
    clearError()
    await mount(selectedProjects, mountpoint, zenMode)
  }

  const filteredProjects = projects.filter(p => p.name.toLowerCase().includes(searchQuery.toLowerCase()))

  return (
    <div className="flex h-full bg-background overflow-hidden">
      {/* ===== Left Sidebar ===== */}
      <div className="w-[280px] shrink-0 border-r bg-muted/10 flex flex-col h-full z-10 relative">
        <div style={DRAG} className="h-12 border-b border-border/50 flex items-center px-4 pl-[78px] shrink-0 bg-background/60 backdrop-blur-md">
          <div className="flex items-center gap-2.5">
            <span className="text-sm font-semibold tracking-tight">Downleaf</span>
            <Badge variant="secondary" className="text-[10px] font-normal px-1.5 py-0 bg-muted/60">
              v0.1.0
            </Badge>
          </div>
        </div>
        
        <div className="p-4 flex items-center justify-between shrink-0">
          <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">Projects</span>
          <Button variant="ghost" size="icon" className="h-6 w-6 text-muted-foreground hover:text-foreground" onClick={refreshProjects} title="Refresh">
            <RotateCw className="w-3.5 h-3.5" />
          </Button>
        </div>
        
        <ScrollArea className="flex-1 min-h-0 px-3">
          <div className="space-y-2 pb-4">
            <div className="relative mb-2">
              <Search className="absolute left-2 top-2 h-3.5 w-3.5 text-muted-foreground" />
              <Input
                placeholder="Filter projects..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="h-7.5 pl-7 text-xs bg-background/50 border-border/40 focus-visible:ring-1"
                disabled={isMounted}
              />
            </div>
            <div className="space-y-0.5">
              <button
                 onClick={() => handleProjectClick('__all__')}
                 disabled={isMounted}
                 className={`w-full text-left px-3 py-2 rounded-md transition-all duration-200 text-sm flex items-center justify-between outline-none focus-visible:ring-2 focus-visible:ring-ring ${
                   selectedProjects.length === 0 ? 'bg-primary text-primary-foreground font-medium shadow-sm' : 'hover:bg-muted/60 text-muted-foreground hover:text-foreground'
                 } ${(isMounted && selectedProjects.length > 0) ? 'opacity-40 cursor-not-allowed' : ''}`}
              >
                 <span>All Projects</span>
              </button>
              
              {filteredProjects.map((p) => (
                <button
                   key={p._id}
                   onClick={() => handleProjectClick(p.name)}
                   disabled={isMounted}
                   className={`w-full text-left px-3 py-2.5 rounded-md transition-all duration-200 text-sm flex auto items-center justify-between outline-none focus-visible:ring-2 focus-visible:ring-ring ${
                     selectedProjects.includes(p.name) ? 'bg-primary text-primary-foreground font-medium shadow-sm' : 'hover:bg-muted/60 text-muted-foreground hover:text-foreground'
                   } ${(isMounted && !selectedProjects.includes(p.name)) ? 'opacity-40 cursor-not-allowed' : ''}`}
                >
                   <span className="truncate mr-3">{p.name}</span>
                   <Badge variant="secondary" className={`text-[10px] shrink-0 transition-colors ${selectedProjects.includes(p.name) ? 'bg-primary-foreground/20 text-primary-foreground hover:bg-primary-foreground/30 border-transparent shadow-none' : 'bg-background hover:bg-muted'}`}>
                      {p.accessLevel}
                   </Badge>
                </button>
              ))}
              {filteredProjects.length === 0 && (
                <p className="text-xs text-muted-foreground px-3 py-2">No projects found.</p>
              )}
            </div>
          </div>
        </ScrollArea>
      </div>

      {/* ===== Right Main Content ===== */}
      <div className="flex-1 flex flex-col min-w-0 h-full relative">
        {/* Main Content Header (Drag region) */}
        <div className="h-12 border-b border-border/50 flex items-center justify-end px-4 shrink-0 bg-background/60 backdrop-blur-md z-10" style={DRAG}>
          <div className="flex items-center gap-1.5" style={NO_DRAG}>
             {isMounted && mountStatus && (
               <Badge variant="outline" className="gap-1.5 text-[11px] font-normal border-sage/40 text-sage mr-2 h-6 px-2 shadow-sm">
                 <span className="w-1.5 h-1.5 rounded-full bg-sage inline-block animate-pulse-slow" />
                 Mounted
               </Badge>
             )}
             <SettingsDialog theme={theme} fontSize={fontSize} setTheme={setTheme} setFontSize={setFontSize} />

             <DropdownMenu>
               <DropdownMenuTrigger className="inline-flex items-center gap-1.5 text-[11px] font-medium text-muted-foreground hover:text-foreground px-2 py-1.5 rounded-md hover:bg-muted/60 transition-colors cursor-pointer outline-none focus-visible:ring-2 focus-visible:ring-ring">
                 <span className="w-1.5 h-1.5 rounded-full bg-sage inline-block" />
                 {loginStatus.email}
               </DropdownMenuTrigger>
               <DropdownMenuContent align="end" className="w-48">
                 <DropdownMenuItem onClick={onLogout} className="text-destructive focus:text-destructive focus:bg-destructive/10 cursor-pointer">
                   Log out
                 </DropdownMenuItem>
               </DropdownMenuContent>
             </DropdownMenu>
          </div>
        </div>

        {/* Configurations & Logs Area */}
        <div className="flex-1 overflow-hidden p-6 lg:p-10 flex flex-col gap-4 lg:gap-8 bg-muted/5">
          <div className="max-w-3xl w-full mx-auto space-y-4 lg:space-y-8 flex-1 flex flex-col min-h-0 h-full">
            
            {/* Configuration Card */}
            <Card className="flex flex-col shrink min-h-[150px] shadow-sm border-border/60 overflow-hidden text-left bg-card group/card">
              <CardHeader className="shrink-0 pb-5 pt-6 px-6 bg-card">
                <CardTitle className="text-lg">Mount Setup</CardTitle>
                <CardDescription className="text-sm mt-1.5 flex flex-col gap-2">
                  <span>Configure local sync for:</span>
                  <div className="flex flex-col gap-1.5 mt-0.5 max-h-[140px] overflow-y-auto pr-1 custom-scrollbar">
                    {selectedProjects.length === 0 ? (
                      <div className="flex items-center gap-2.5 px-3 py-2 rounded-md bg-muted/30 border border-border/40 text-foreground font-medium text-sm shadow-sm transition-colors hover:bg-muted/40">
                         <Library className="w-4 h-4 text-muted-foreground" />
                         All Projects
                      </div>
                    ) : (
                      selectedProjects.map((p) => (
                        <div key={p} className="flex items-center justify-between px-3 py-2 rounded-md bg-background border border-border/50 text-foreground font-medium text-sm shadow-sm transition-colors hover:bg-muted/30">
                          <div className="flex items-center gap-2.5 overflow-hidden">
                             <Folder className="w-4 h-4 shrink-0 text-sage/80 fill-sage/10" />
                             <span className="truncate">{p}</span>
                          </div>
                        </div>
                      ))
                    )}
                  </div>
                </CardDescription>
              </CardHeader>
              <Separator className="shrink-0" />
              <CardContent className="flex-1 overflow-y-auto space-y-6 pt-6 px-6 pb-6 bg-card custom-scrollbar min-h-0">
                <div className="space-y-2.5">
                  <Label htmlFor="mountpoint" className="text-sm font-medium">Local Mountpoint</Label>
                  <Input
                    id="mountpoint"
                    value={mountpoint}
                    onChange={(e) => setMountpoint(e.target.value)}
                    disabled={isMounted}
                    className="font-mono text-sm h-10 bg-background/50 shadow-sm"
                  />
                  <p className="text-xs text-muted-foreground pt-0.5">The absolute path where project files will be synchronized.</p>
                </div>
                
                <div className="flex items-center space-x-3 bg-muted/40 p-4 rounded-lg border border-border/50 transition-colors hover:bg-muted/60">
                  <Switch
                    id="zen"
                    checked={zenMode}
                    onCheckedChange={setZenMode}
                    disabled={isMounted}
                  />
                  <Label htmlFor="zen" className="text-sm font-medium cursor-pointer flex-1">
                    Zen Mode
                    <p className="text-xs text-muted-foreground font-normal leading-snug mt-1">
                      Disable auto-sync on code change. Sync on manually.
                    </p>
                  </Label>
                </div>

                {isMounted && mountStatus && (
                  <div className="text-sm text-sage px-4 py-3 rounded-md bg-sage-soft/20 border border-sage/20 flex flex-col gap-1">
                     <span className="font-medium">Active Mount</span>
                     <span className="font-mono text-xs opacity-90">{mountStatus.mountpoint}</span>
                  </div>
                )}

                {error && (
                  <div className="text-sm text-destructive px-4 py-3 rounded-md bg-destructive/10 border border-destructive/20 font-medium">
                    {error}
                  </div>
                )}
              </CardContent>
              <div className="shrink-0 px-6 py-4 bg-muted/30 border-t border-border/50 flex items-center justify-end gap-3 rounded-b-xl">
                 {isMounted ? (
                   <>
                     <Button variant="outline" className="shadow-sm" onClick={openMountpoint}>
                       Open Folder
                     </Button>
                     {mountStatus?.zenMode && (
                       <Button variant="secondary" className="shadow-sm" disabled={loading === 'sync'} onClick={sync}>
                         {loading === 'sync' ? 'Syncing...' : 'Sync Now'}
                       </Button>
                     )}
                     <Button variant="destructive" className="shadow-sm" disabled={loading === 'unmount'} onClick={unmount}>
                       {loading === 'unmount' ? 'Unmounting...' : 'Unmount'}
                     </Button>
                   </>
                 ) : (
                   <Button disabled={loading === 'mount'} onClick={handleMount} className="min-w-[120px] shadow-sm">
                     {loading === 'mount' ? 'Mounting...' : 'Mount Project'}
                   </Button>
                 )}
              </div>
            </Card>

            {/* Logs Area */}
            <Card className="flex-1 flex flex-col min-h-[100px] shadow-sm border-border/60 overflow-hidden text-left">
              <CardHeader className="py-3.5 px-6 flex-row items-center justify-between border-b border-border/50 shrink-0 bg-muted/30">
                <CardTitle className="text-sm font-medium flex items-center gap-2 text-muted-foreground">
                  <Terminal className="w-4 h-4" />
                  Terminal Logs
                </CardTitle>
                <Button variant="ghost" size="sm" className="h-7 text-xs px-2.5 text-muted-foreground hover:text-foreground" onClick={clearLogs}>
                  Clear
                </Button>
              </CardHeader>
              <CardContent className="p-0 flex-1 relative bg-card/50">
                <ScrollArea className="absolute inset-0 w-full h-full text-left bg-background/30 custom-scrollbar">
                  <div className="p-5 font-mono text-[11px] leading-relaxed text-muted-foreground whitespace-pre-wrap break-all min-h-full">
                    {logs.length > 0 ? logs.join('\n') : "No logs available."}
                    <div ref={logEndRef} className="h-4" />
                  </div>
                </ScrollArea>
              </CardContent>
            </Card>
          </div>
        </div>
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
      <DialogTrigger className="inline-flex items-center text-[11px] font-medium text-muted-foreground hover:text-foreground px-2 py-1.5 rounded-md hover:bg-muted/60 transition-colors cursor-pointer outline-none focus-visible:ring-2 focus-visible:ring-ring">
        Settings
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Settings</DialogTitle>
        </DialogHeader>
        <div className="space-y-6 pt-2">
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
