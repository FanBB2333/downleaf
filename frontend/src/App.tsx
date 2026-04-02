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
        browserLoginSupported={store.browserLoginSupported}
        savedCredentials={store.savedCredentials}
        onLogin={store.login}
        onLoginWithBrowser={store.loginWithBrowser}
        onLoginWithCredential={store.loginWithCredential}
        onDeleteCredential={store.deleteCredential}
      />
    )
  }

  return (
    <MainPage
      version={store.version}
      loginStatus={store.loginStatus}
      mountStatus={store.mountStatus}
      projects={store.projects}
      tags={store.tags}
      logs={store.logs}
      loading={store.loading}
      error={store.error}
      theme={store.theme}
      colorScheme={store.colorScheme}
      fontSize={store.fontSize}
      setTheme={store.setTheme}
      setColorScheme={store.setColorScheme}
      setFontSize={store.setFontSize}
      refreshProjects={store.refreshProjects}
      mount={(projects, mp, batch) =>
        store.mount(projects, mp, batch)
      }
      unmount={store.unmount}
      sync={store.sync}
      openMountpoint={store.openMountpoint}
      clearLogs={store.clearLogs}
      clearError={store.clearError}
      onLogout={store.logout}
    />
  )
}
