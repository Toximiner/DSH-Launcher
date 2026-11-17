//go:build !js

package main

// Bind immutable file paths to callbacks: a late result from the previous
// backend must never read or write the next backend's tab state.
func (m *model) configureNativeTabPersistence(profile, cwd string, fallbackNew bool) {
	if control, ok := m.runtime.(*nativeRuntime); ok {
		control.mu.Lock()
		profile, cwd = control.config.Profile, control.config.CWD
		control.mu.Unlock()
	}
	if m.settingsLogin == "" {
		// Remember that this is a native CLI model, even before /runtime starts dsh.
		m.tabPersistenceScope = "pending"
		return
	}
	path, err := tabPersistencePath(m.settingsLogin, profile, cwd)
	if err != nil {
		m.tabPersistence = nil
		m.events = append(m.events, tr("Не удалось определить файл открытых вкладок."))
		return
	}
	if m.tabPersistenceScope != "" && m.tabPersistenceScope != path {
		m.detachCurrentTab()
		m.tabState = sessionTabsState{}
	}
	m.tabPersistenceScope = path
	m.configureTabPersistence(
		func() (savedSessionTabs, error) { return readSavedSessionTabs(path) },
		func(saved savedSessionTabs) error { return writeSavedSessionTabs(path, saved) },
		fallbackNew,
	)
}
