//go:build vim4

package platform

const VIM4 = true
const StateDir = "/var/lib/jetkvm"
const ConfigPath = StateDir + "/config.json"

// sshd reads authorized_keys2 by default. Using it keeps the key managed by
// the web UI separate from the account's own authorized_keys.
const SSHKeyFile = "/home/khadas/.ssh/authorized_keys2"

// vim4-manager owns the public listeners and authenticates backend requests.
const HTTPPort = 8081
