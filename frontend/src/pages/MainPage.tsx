import { useState, useRef, useEffect, useCallback } from 'react'
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
import { useWindowDrag } from '@/hooks/use-window-drag'
import type { gui } from '../../wailsjs/go/models'
import type { model } from '../../wailsjs/go/models'
import type { Theme, ColorScheme } from '@/hooks/use-store'

interface MainPageProps {
  version: string
  loginStatus: gui.LoginStatus
  mountStatus: gui.MountStatus | null
  projects: model.Project[]
  tags: model.Tag[]
  logs: string[]
  loading: string
  error: string
  theme: Theme
  colorScheme: ColorScheme
  fontSize: number
  backend: string
  backends: gui.BackendInfo[]
  setTheme: (t: Theme) => void
  setColorScheme: (s: ColorScheme) => void
  setFontSize: (s: number) => void
  setBackend: (name: string) => Promise<void>
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
  version,
  loginStatus,
  mountStatus,
  projects,
  tags,
  logs,
  loading,
  error,
  theme,
  colorScheme,
  fontSize,
  backend,
  backends,
  setTheme,
  setColorScheme,
  setFontSize,
  setBackend,
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
  const [selectedTags, setSelectedTags] = useState<string[]>([])
  const [viewMode, setViewMode] = useState<'flat' | 'grouped'>(() => {
    return (localStorage.getItem('downleaf-view-mode') as 'flat' | 'grouped') || 'flat'
  })
  const [searchQuery, setSearchQuery] = useState('')
  const [mountpoint, setMountpoint] = useState('~/downleaf')
  const [zenMode, setZenMode] = useState(true)
  const [logPanelHeight, setLogPanelHeight] = useState(200)
  const logEndRef = useRef<HTMLDivElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const isDraggingRef = useRef(false)
  const isMounted = mountStatus?.mounted ?? false
  const onDragMouseDown = useWindowDrag()

  const handleResizeMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    isDraggingRef.current = true
    const startY = e.clientY
    const startHeight = logPanelHeight

    const onMouseMove = (ev: MouseEvent) => {
      if (!isDraggingRef.current) return
      const delta = startY - ev.clientY
      const newHeight = Math.max(80, Math.min(startHeight + delta, (containerRef.current?.clientHeight ?? 600) - 200))
      setLogPanelHeight(newHeight)
    }

    const onMouseUp = () => {
      isDraggingRef.current = false
      document.removeEventListener('mousemove', onMouseMove)
      document.removeEventListener('mouseup', onMouseUp)
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
    }

    document.body.style.cursor = 'row-resize'
    document.body.style.userSelect = 'none'
    document.addEventListener('mousemove', onMouseMove)
    document.addEventListener('mouseup', onMouseUp)
  }, [logPanelHeight])

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

  const toggleViewMode = () => {
    setViewMode(prev => {
      const next = prev === 'flat' ? 'grouped' : 'flat'
      localStorage.setItem('downleaf-view-mode', next)
      return next
    })
  }

  const filteredProjects = projects.filter(p => {
    const matchesSearch = p.name.toLowerCase().includes(searchQuery.toLowerCase())
    if (!matchesSearch) return false
    if (selectedTags.length === 0) return true
    return selectedTags.some(tagId =>
      tags.find(t => t._id === tagId)?.project_ids?.includes(p._id)
    )
  })

  const groupedProjects = viewMode === 'grouped'
    ? (() => {
        const groups: { name: string; id: string; projects: model.Project[] }[] = []
        const taggedIds = new Set<string>()

        for (const tag of tags) {
          const tagProjects = filteredProjects.filter(p => tag.project_ids?.includes(p._id))
          if (tagProjects.length > 0) {
            groups.push({ name: tag.name, id: tag._id, projects: tagProjects })
            tagProjects.forEach(p => taggedIds.add(p._id))
          }
        }

        const untagged = filteredProjects.filter(p => !taggedIds.has(p._id))
        if (untagged.length > 0) {
          groups.push({ name: 'Untagged', id: '__untagged__', projects: untagged })
        }

        return groups
      })()
    : []

  return (
    <div className="flex h-full bg-background overflow-hidden border-b border-border/50">
      {/* ===== Left Sidebar ===== */}
      <div className="w-[280px] shrink-0 border-r border-border/50 bg-muted/10 flex flex-col h-full z-10 relative">
        <div onMouseDown={onDragMouseDown} className="h-12 border-b border-border/50 flex items-center px-4 pl-[78px] shrink-0 bg-background/60 backdrop-blur-md cursor-default">
          <div className="flex items-center gap-2.5">
            <span className="text-sm font-semibold tracking-tight">Downleaf</span>
            <Badge variant="secondary" className="text-[10px] font-normal px-1.5 py-0 bg-muted/60">
              {version}
            </Badge>
          </div>
        </div>
        
        <div className="p-4 flex items-center justify-between shrink-0">
          <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">Projects</span>
          <Button variant="ghost" size="icon" className="h-6 w-6 text-muted-foreground hover:text-foreground" onClick={refreshProjects} title="Refresh">
            <RotateCw className="w-3.5 h-3.5" />
          </Button>
        </div>
        
        <ScrollArea className="flex-1 min-h-0 px-3 pb-2">
          <div className="space-y-2 pb-3">
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

            {/* Tag filters */}
            {tags.length > 0 && (
              <div className="flex flex-wrap gap-1.5 mb-2">
                <button
                  onClick={() => setSelectedTags([])}
                  disabled={isMounted}
                  className={`text-[11px] px-2 py-0.5 rounded-full transition-all ${
                    selectedTags.length === 0
                      ? 'bg-primary text-primary-foreground font-medium'
                      : 'bg-muted/60 text-muted-foreground hover:bg-muted hover:text-foreground'
                  }`}
                >
                  All
                </button>
                {tags.map(tag => (
                  <button
                    key={tag._id}
                    onClick={() => {
                      if (isMounted) return
                      setSelectedTags(prev =>
                        prev.includes(tag._id)
                          ? prev.filter(id => id !== tag._id)
                          : [...prev, tag._id]
                      )
                    }}
                    disabled={isMounted}
                    className={`text-[11px] px-2 py-0.5 rounded-full transition-all ${
                      selectedTags.includes(tag._id)
                        ? 'bg-primary text-primary-foreground font-medium'
                        : 'bg-muted/60 text-muted-foreground hover:bg-muted hover:text-foreground'
                    }`}
                  >
                    {tag.name}
                  </button>
                ))}
              </div>
            )}

            {/* View mode toggle */}
            {tags.length > 0 && (
              <div className="flex justify-end mb-1">
                <button
                  onClick={toggleViewMode}
                  className="text-[10px] text-muted-foreground hover:text-foreground transition-colors px-1.5 py-0.5 rounded hover:bg-muted/60"
                  title={viewMode === 'flat' ? 'Switch to grouped view' : 'Switch to flat view'}
                >
                  {viewMode === 'flat' ? 'Group by tag' : 'Flat list'}
                </button>
              </div>
            )}

            <div className="space-y-0.5">
              {viewMode === 'flat' ? (
                <>
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
                    <ProjectButton key={p._id} project={p} selected={selectedProjects.includes(p.name)} disabled={isMounted && !selectedProjects.includes(p.name)} onClick={() => handleProjectClick(p.name)} isMounted={isMounted} />
                  ))}
                </>
              ) : (
                <>
                  <button
                    onClick={() => handleProjectClick('__all__')}
                    disabled={isMounted}
                    className={`w-full text-left px-3 py-2 rounded-md transition-all duration-200 text-sm flex items-center justify-between outline-none focus-visible:ring-2 focus-visible:ring-ring ${
                      selectedProjects.length === 0 ? 'bg-primary text-primary-foreground font-medium shadow-sm' : 'hover:bg-muted/60 text-muted-foreground hover:text-foreground'
                    } ${(isMounted && selectedProjects.length > 0) ? 'opacity-40 cursor-not-allowed' : ''}`}
                  >
                    <span>All Projects</span>
                  </button>

                  {groupedProjects.map(group => (
                    <GroupSection
                      key={group.id}
                      name={group.name}
                      projects={group.projects}
                      selectedProjects={selectedProjects}
                      isMounted={isMounted}
                      onProjectClick={handleProjectClick}
                    />
                  ))}
                </>
              )}

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
        <div className="h-12 border-b border-border/50 flex items-center justify-end px-4 shrink-0 bg-background/60 backdrop-blur-md z-10 cursor-default" onMouseDown={onDragMouseDown}>
          <div className="flex items-center gap-1.5">
             {isMounted && mountStatus && (
               <Badge variant="outline" className="gap-1.5 text-[11px] font-normal border-sage/40 text-sage mr-2 h-6 px-2 shadow-sm">
                 <span className="w-1.5 h-1.5 rounded-full bg-sage inline-block animate-pulse-slow" />
                 Mounted
               </Badge>
             )}
             <SettingsDialog theme={theme} colorScheme={colorScheme} fontSize={fontSize} backend={backend} backends={backends} isMounted={isMounted} setTheme={setTheme} setColorScheme={setColorScheme} setFontSize={setFontSize} setBackend={setBackend} />

             <DropdownMenu>
               <DropdownMenuTrigger className="inline-flex items-center gap-1.5 text-[11px] font-medium text-muted-foreground hover:text-foreground px-2 py-1.5 rounded-md hover:bg-muted/60 transition-colors cursor-pointer outline-none focus-visible:ring-2 focus-visible:ring-ring">
                 <span className="w-1.5 h-1.5 rounded-full bg-green-500 inline-block" />
                 {loginStatus.email}
               </DropdownMenuTrigger>
               <DropdownMenuContent align="end" className="w-48">
                 <DropdownMenuItem onClick={onLogout} className="cursor-pointer">
                   Switch Account
                 </DropdownMenuItem>
                 <DropdownMenuItem onClick={onLogout} className="text-destructive focus:text-destructive focus:bg-destructive/10 cursor-pointer">
                   Log out
                 </DropdownMenuItem>
               </DropdownMenuContent>
             </DropdownMenu>
          </div>
        </div>

        {/* Configurations & Logs Area */}
        <div ref={containerRef} className="flex-1 overflow-hidden p-4 lg:p-8 flex flex-col bg-muted/5">
          <div className="max-w-3xl w-full mx-auto gap-3 lg:gap-4 flex-1 flex flex-col min-h-0 h-full">
            
            {/* Configuration Card */}
            <Card className="flex flex-col shrink-0 shadow-sm border-border/60 text-left bg-card group/card">
              <CardHeader className="shrink-0 py-3 px-5 bg-card">
                <div className="flex items-center justify-between">
                  <CardTitle className="text-sm font-semibold">Mount Setup</CardTitle>
                  {selectedProjects.length === 0 ? (
                    <span className="text-xs text-muted-foreground flex items-center gap-1.5">
                      <Library className="w-3.5 h-3.5" /> All Projects
                    </span>
                  ) : selectedProjects.length === 1 ? (
                    <span className="text-xs text-muted-foreground flex items-center gap-1.5">
                      <Folder className="w-3.5 h-3.5 text-sage/80" /> {selectedProjects[0]}
                    </span>
                  ) : (
                    <DropdownMenu>
                      <DropdownMenuTrigger className="text-xs text-muted-foreground flex items-center gap-1.5 hover:text-foreground transition-colors cursor-pointer outline-none rounded px-1.5 py-0.5 hover:bg-muted/60">
                        <Folder className="w-3.5 h-3.5 text-sage/80" /> {selectedProjects.length} projects
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="min-w-[180px]">
                        {selectedProjects.map((p) => (
                          <DropdownMenuItem key={p} className="text-xs gap-2" onSelect={(e) => e.preventDefault()}>
                            <Folder className="w-3.5 h-3.5 text-sage/80 shrink-0" />
                            <span className="truncate">{p}</span>
                          </DropdownMenuItem>
                        ))}
                      </DropdownMenuContent>
                    </DropdownMenu>
                  )}
                </div>
              </CardHeader>
              <Separator className="shrink-0" />
              <CardContent className="shrink-0 space-y-4 py-4 px-5 bg-card">
                <div className="space-y-1.5">
                  <Label htmlFor="mountpoint" className="text-sm font-medium">Local Mountpoint</Label>
                  <Input
                    id="mountpoint"
                    value={mountpoint}
                    onChange={(e) => setMountpoint(e.target.value)}
                    disabled={isMounted}
                    className="font-mono text-sm h-9 bg-background/50 shadow-sm"
                  />
                </div>

                <div className="flex items-center space-x-3 bg-muted/40 px-3 py-2.5 rounded-lg border border-border/50 transition-colors hover:bg-muted/60">
                  <Switch
                    id="zen"
                    checked={zenMode}
                    onCheckedChange={setZenMode}
                    disabled={isMounted}
                  />
                  <Label htmlFor="zen" className="text-sm font-medium cursor-pointer flex-1">
                    Zen Mode
                    <p className="text-xs text-muted-foreground font-normal leading-snug mt-0.5">
                      Local-first editing, sync manually.
                    </p>
                  </Label>
                </div>

                {isMounted && mountStatus && (
                  <div className="text-sm text-sage px-3 py-2 rounded-md bg-sage-soft/20 border border-sage/20 flex items-center gap-2">
                     <span className="font-medium">Active:</span>
                     <span className="font-mono text-xs opacity-90 truncate">{mountStatus.mountpoint}</span>
                  </div>
                )}

                {error && (
                  <div className="text-sm text-destructive px-3 py-2 rounded-md bg-destructive/10 border border-destructive/20 font-medium">
                    {error}
                  </div>
                )}
              </CardContent>
              <div className="shrink-0 px-5 py-3 bg-muted/30 border-t border-border/50 flex items-center justify-end gap-2.5 rounded-b-xl">
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

            {/* Resize Handle */}
            <div
              onMouseDown={handleResizeMouseDown}
              className="shrink-0 h-2 -my-1 cursor-row-resize z-10 rounded-full mx-auto w-full transition-colors hover:bg-border/60"
            />

            {/* Logs Area */}
            <Card className="p-0 gap-0 flex flex-col shadow-sm border-border/60 overflow-hidden text-left bg-card flex-1" style={{ minHeight: logPanelHeight }}>
              <div className="flex items-center justify-between py-2 px-3 border-b border-border/50 shrink-0 bg-muted/30">
                <span className="text-xs font-medium flex items-center gap-2 text-muted-foreground">
                  <Terminal className="w-3.5 h-3.5" />
                  Terminal Logs
                </span>
                <Button variant="ghost" size="sm" className="h-6 text-xs px-2 text-muted-foreground hover:text-foreground" onClick={clearLogs}>
                  Clear
                </Button>
              </div>
              <div className="flex-1 min-h-0 overflow-y-auto custom-scrollbar bg-card/30">
                <div className="p-3 font-mono text-[11px] leading-relaxed text-muted-foreground whitespace-pre-wrap break-all min-h-full">
                  {logs.length > 0 ? logs.join('\n') : "No logs available."}
                  <div ref={logEndRef} className="h-4" />
                </div>
              </div>
            </Card>
          </div>
        </div>
      </div>
    </div>
  )
}

/* ===== Project Button ===== */

function ProjectButton({ project: p, selected, disabled, onClick, isMounted }: {
  project: model.Project
  selected: boolean
  disabled: boolean
  onClick: () => void
  isMounted: boolean
}) {
  return (
    <button
      onClick={onClick}
      disabled={isMounted}
      className={`w-full text-left px-3 py-2.5 rounded-md transition-all duration-200 text-sm flex auto items-center justify-between outline-none focus-visible:ring-2 focus-visible:ring-ring ${
        selected ? 'bg-primary text-primary-foreground font-medium shadow-sm' : 'hover:bg-muted/60 text-muted-foreground hover:text-foreground'
      } ${disabled ? 'opacity-40 cursor-not-allowed' : ''}`}
    >
      <span className="truncate mr-3">{p.name}</span>
      <Badge variant="secondary" className={`text-[10px] shrink-0 transition-colors ${selected ? 'bg-primary-foreground/20 text-primary-foreground hover:bg-primary-foreground/30 border-transparent shadow-none' : 'bg-background hover:bg-muted'}`}>
        {p.accessLevel}
      </Badge>
    </button>
  )
}

/* ===== Group Section (for grouped view) ===== */

function GroupSection({
  name,
  projects,
  selectedProjects,
  isMounted,
  onProjectClick,
}: {
  name: string
  projects: model.Project[]
  selectedProjects: string[]
  isMounted: boolean
  onProjectClick: (name: string) => void
}) {
  const [collapsed, setCollapsed] = useState(false)

  return (
    <div className="mt-2">
      <button
        onClick={() => setCollapsed(prev => !prev)}
        className="w-full flex items-center gap-1.5 px-2 py-1 text-[11px] font-semibold text-muted-foreground uppercase tracking-wider hover:text-foreground transition-colors rounded hover:bg-muted/40"
      >
        <span className={`transition-transform duration-200 text-[9px] ${collapsed ? '' : 'rotate-90'}`}>&#9654;</span>
        {name}
        <span className="text-[10px] font-normal ml-auto opacity-60">{projects.length}</span>
      </button>
      {!collapsed && (
        <div className="space-y-0.5 mt-0.5">
          {projects.map((p) => (
            <ProjectButton
              key={p._id}
              project={p}
              selected={selectedProjects.includes(p.name)}
              disabled={isMounted && !selectedProjects.includes(p.name)}
              onClick={() => onProjectClick(p.name)}
              isMounted={isMounted}
            />
          ))}
        </div>
      )}
    </div>
  )
}

/* ===== Settings Dialog ===== */

const COLOR_SCHEMES: { id: ColorScheme; label: string; swatch: [string, string] }[] = [
  { id: 'classic', label: 'Classic',  swatch: ['#c5bdb4', '#5e544b'] },
  { id: 'sage',    label: 'Sage',     swatch: ['#a8b5a0', '#6b7c65'] },
  { id: 'rose',    label: 'Rose',     swatch: ['#c4a4a0', '#9e7a76'] },
  { id: 'blue',    label: 'Blue',     swatch: ['#8e9aab', '#6a7a8e'] },
  { id: 'lavender',label: 'Lavender', swatch: ['#b0a4b8', '#8a7a96'] },
]

function SettingsDialog({
  theme,
  colorScheme,
  fontSize,
  backend,
  backends,
  isMounted,
  setTheme,
  setColorScheme,
  setFontSize,
  setBackend,
}: {
  theme: Theme
  colorScheme: ColorScheme
  fontSize: number
  backend: string
  backends: gui.BackendInfo[]
  isMounted: boolean
  setTheme: (t: Theme) => void
  setColorScheme: (s: ColorScheme) => void
  setFontSize: (s: number) => void
  setBackend: (name: string) => Promise<void>
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
            <Label className="text-sm">Color Scheme</Label>
            <div className="grid grid-cols-5 gap-2">
              {COLOR_SCHEMES.map((s) => (
                <button
                  key={s.id}
                  onClick={() => setColorScheme(s.id)}
                  className={`flex flex-col items-center gap-1.5 p-2 rounded-lg border-2 transition-all duration-200 cursor-pointer outline-none focus-visible:ring-2 focus-visible:ring-ring ${
                    colorScheme === s.id
                      ? 'border-primary bg-muted/60 shadow-sm'
                      : 'border-transparent hover:bg-muted/40'
                  }`}
                >
                  <div className="w-8 h-8 rounded-full overflow-hidden border border-border/60 shadow-sm" style={{
                    background: `linear-gradient(135deg, ${s.swatch[0]} 50%, ${s.swatch[1]} 50%)`
                  }} />
                  <span className="text-[10px] font-medium text-muted-foreground">{s.label}</span>
                </button>
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

          {backends.length > 0 && (
            <>
              <Separator />
              <div className="space-y-2">
                <Label className="text-sm">Mount Backend</Label>
                <div className="flex gap-2">
                  {backends.map((b) => (
                    <Button
                      key={b.name}
                      size="sm"
                      variant={backend === b.name ? 'default' : 'outline'}
                      className="flex-1 text-xs"
                      disabled={!b.available || isMounted}
                      onClick={() => b.available && !isMounted && setBackend(b.name)}
                      title={
                        isMounted ? 'Unmount first to change backend'
                        : !b.available ? 'Coming soon'
                        : b.name
                      }
                    >
                      {b.name}{!b.available && ' (coming soon)'}
                    </Button>
                  ))}
                </div>
                {isMounted && (
                  <p className="text-[10px] text-muted-foreground">Unmount to change backend</p>
                )}
              </div>
            </>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
