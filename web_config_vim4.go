//go:build vim4

package kvm

// The manager is the only public web entry point, including after loading an
// older config or resetting the application. Credentials remain on disk.
func normalizePlatformWebConfig(cfg *Config) {
	cfg.LocalLoopbackOnly = true
	cfg.LocalAuthMode = "noPassword"
	cfg.TLSMode = ""
}
