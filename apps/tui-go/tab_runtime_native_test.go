//go:build !js

package main

import "testing"

func TestNativeTabPersistenceUsesEffectiveRuntimeProfileAndInvalidatesOldResults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DSH_HOME", t.TempDir())
	m := tabFixture(2)
	m.settingsLogin = "http://127.0.0.1:3080/?token=first"
	control := &nativeRuntime{config: runtimeConfig{Profile: "saved-profile", CWD: t.TempDir()}}
	m.runtime = control
	m.configureNativeTabPersistence("ignored-cli-default", "", true)
	path, err := tabPersistencePath(m.settingsLogin, "saved-profile", control.config.CWD)
	if err != nil || path != m.tabPersistenceScope {
		t.Fatal("effective runtime profile was not used", err)
	}
	previous := m.tabPersistence
	m.settingsLogin = "http://127.0.0.1:3080/?token=rotated"
	m.configureNativeTabPersistence("", "", true)
	if m.tabPersistenceScope != path || len(m.tabList()) != 2 {
		t.Fatal("token rotation changed scope or cleared open tabs")
	}
	previous.loading = true
	if cmd := m.handleTabRestore(tabRestoreResult{owner: previous, login: m.settingsLogin}); cmd != nil {
		t.Fatal("old restore result was accepted")
	}
	control.config.Profile = "other-profile"
	m.configureNativeTabPersistence("", "", true)
	if m.tabPersistenceScope == path || len(m.tabList()) != 0 || m.session != nil {
		t.Fatal("another profile inherited the previous profile's tabs")
	}
}

func TestNativeTabPersistenceCanBeEnabledBeforeRuntimeStarts(t *testing.T) {
	m := newModel()
	m.configureNativeTabPersistence("web", "", false)
	if m.tabPersistence != nil || m.tabPersistenceScope != "pending" {
		t.Fatal("offline startup must defer the scope until a login is known")
	}
}
