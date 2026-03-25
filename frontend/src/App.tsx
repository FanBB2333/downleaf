import { useStore } from '@/hooks/use-store'
import { LoginPage } from '@/pages/LoginPage'
import { MainPage } from '@/pages/MainPage'

export default function App() {
  const store = useStore()

  if (!store.loginStatus?.loggedIn) {
    return (
      <LoginPage
        envDefaults={store.envDefaults}
        loading={store.loading}
        onLogin={store.login}
      />
    )
  }

  return (
    <MainPage
      loginStatus={store.loginStatus}
      mountStatus={store.mountStatus}
      projects={store.projects}
      logs={store.logs}
      loading={store.loading}
      error={store.error}
      theme={store.theme}
      fontSize={store.fontSize}
      setTheme={store.setTheme}
      setFontSize={store.setFontSize}
      refreshProjects={store.refreshProjects}
      mount={(project, mp, batch) =>
        store.mount(project === '__all__' ? '' : project, mp, batch)
      }
      unmount={store.unmount}
      sync={store.sync}
      openMountpoint={store.openMountpoint}
      clearLogs={store.clearLogs}
      clearError={store.clearError}
      onLogout={() => window.location.reload()}
    />
  )
}
