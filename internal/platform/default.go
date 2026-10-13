//go:build !vim4

package platform

const VIM4 = false
const StateDir = "/userdata/jetkvm"
const ConfigPath = "/userdata/kvm_config.json"
const SSHKeyFile = "/userdata/dropbear/.ssh/authorized_keys"
const HTTPPort = 80
