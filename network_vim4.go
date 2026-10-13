package kvm

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/guregu/null/v6"
	"github.com/jetkvm/kvm/internal/confparser"
	"github.com/jetkvm/kvm/internal/network/types"
	"github.com/jetkvm/kvm/internal/ota"
	"github.com/jetkvm/kvm/internal/platform"
	"github.com/vishvananda/netlink"
)

// NetworkManager owns the Ubuntu network. JetKVM reads interface state
// through netlink and changes the active Ethernet profile through the helper.
// HTTP proxy and similar application settings stay in the JetKVM config.

const vim4DHCPClientName = "NetworkManager"

func initVIM4Network() {
	validateNetworkConfig()
	go monitorVIM4Network()
}

// monitorVIM4Network reports interface changes to the UI, like nmlite's
// state callback on the appliance.
func monitorVIM4Network() {
	var last *types.InterfaceState
	for {
		state, err := vim4InterfaceState()
		if err == nil && (last == nil || vim4StateChanged(last, state)) {
			last = state
			if currentSession != nil {
				writeJSONRPCEvent("networkState", state.ToRpcInterfaceState(), currentSession)
			}
			setPublicIPReadyState(state.IPv4Ready, state.IPv6Ready)
			if mqttManager != nil {
				mqttManager.publishNetworkState()
			}
		}
		time.Sleep(2 * time.Second)
	}
}

func vim4StateChanged(a, b *types.InterfaceState) bool {
	return a.Up != b.Up || a.Online != b.Online || a.MACAddress != b.MACAddress ||
		a.Hostname != b.Hostname || a.IPv6Address != b.IPv6Address ||
		!slices.Equal(a.IPv4Addresses, b.IPv4Addresses) || !reflect.DeepEqual(a.DHCPLease4, b.DHCPLease4)
}

func vim4InterfaceState() (*types.InterfaceState, error) {
	link, err := netlink.LinkByName(NetIfName)
	if err != nil {
		return nil, err
	}
	attrs := link.Attrs()
	hostname, _ := os.Hostname()
	state := &types.InterfaceState{
		InterfaceName: NetIfName,
		Hostname:      hostname,
		MACAddress:    attrs.HardwareAddr.String(),
		Up:            attrs.OperState == netlink.OperUp,
		LastUpdated:   time.Now(),
	}
	addrs, err := netlink.AddrList(link, netlink.FAMILY_ALL)
	if err != nil {
		return nil, err
	}
	for _, addr := range addrs {
		switch {
		case addr.IP.To4() != nil:
			state.IPv4Addresses = append(state.IPv4Addresses, addr.IPNet.String())
			if state.IPv4Address == "" {
				state.IPv4Address = addr.IP.String()
				state.IPv4Ready = true
			}
		case addr.IP.IsLinkLocalUnicast():
			state.IPv6LinkLocal = addr.IP.String()
		case addr.IP.IsGlobalUnicast():
			state.IPv6Addresses = append(state.IPv6Addresses, types.IPv6Address{
				Address: addr.IP, Prefix: *addr.IPNet, Scope: addr.Scope, Flags: addr.Flags,
				ValidLifetime: lifetime(addr.ValidLft), PreferredLifetime: lifetime(addr.PreferedLft),
			})
			if state.IPv6Address == "" {
				state.IPv6Address = addr.IP.String()
				state.IPv6Ready = true
			}
		}
	}
	if routes, err := netlink.RouteList(link, netlink.FAMILY_V6); err == nil {
		for _, route := range routes {
			if route.Dst == nil && route.Gw != nil {
				state.IPv6Gateway = route.Gw.String()
				break
			}
		}
	}
	state.Online = state.Up && (state.IPv4Ready || state.IPv6Ready)
	if lease := vim4DHCPLease(); lease != nil {
		state.DHCPLease4 = lease
		state.NTPServers = lease.NTPServers
	}
	return state, nil
}

func lifetime(seconds int) *time.Time {
	if seconds <= 0 || uint32(seconds) == 0xFFFFFFFF {
		return nil
	}
	t := time.Now().Add(time.Duration(seconds) * time.Second)
	return &t
}

// vim4DHCPLease reads the lease NetworkManager obtained for Ethernet.
func vim4DHCPLease() *types.DHCPLease {
	output, err := exec.Command("nmcli", "--escape", "no", "-g", "DHCP4.OPTION", "device", "show", NetIfName).Output()
	if err != nil {
		return nil
	}
	return parseNMDHCPOptions(string(output))
}

// parseNMDHCPOptions parses "name = value | name = value" from nmcli -g DHCP4.OPTION.
func parseNMDHCPOptions(output string) *types.DHCPLease {
	options := map[string]string{}
	for _, item := range strings.FieldsFunc(output, func(r rune) bool { return r == '|' || r == '\n' }) {
		if name, value, ok := strings.Cut(item, "="); ok {
			options[strings.TrimSpace(name)] = strings.TrimSpace(value)
		}
	}
	ip := net.ParseIP(options["ip_address"])
	if ip == nil {
		return nil
	}
	ips := func(name string) []net.IP {
		var result []net.IP
		for _, field := range strings.Fields(options[name]) {
			if parsed := net.ParseIP(field); parsed != nil {
				result = append(result, parsed)
			}
		}
		return result
	}
	seconds := func(name string) time.Duration {
		value, _ := strconv.Atoi(options[name])
		return time.Duration(value) * time.Second
	}
	lease := &types.DHCPLease{
		IPAddress:  ip,
		Netmask:    net.ParseIP(options["subnet_mask"]),
		Broadcast:  net.ParseIP(options["broadcast_address"]),
		HostName:   options["host_name"],
		Domain:     options["domain_name"],
		SearchList: strings.Fields(options["domain_search"]),
		Routers:    ips("routers"),
		DNS:        ips("domain_name_servers"),
		NTPServers: ips("ntp_servers"),
		ServerID:   options["dhcp_server_identifier"],
		LeaseTime:  seconds("dhcp_lease_time"),
		// NetworkManager does not report renewal and rebinding times unless the server sends them.
		RenewalTime:   seconds("dhcp_renewal_time"),
		RebindingTime: seconds("dhcp_rebinding_time"),
		InterfaceName: NetIfName,
		DHCPClient:    vim4DHCPClientName,
	}
	if mtu, err := strconv.Atoi(options["interface_mtu"]); err == nil {
		lease.MTU = mtu
	}
	if expiry, err := strconv.ParseInt(options["expiry"], 10, 64); err == nil {
		t := time.Unix(expiry, 0)
		lease.LeaseExpiry = &t
	}
	return lease
}

// vim4Profile is the Ethernet configuration exchanged with the helper.
type vim4Profile struct {
	IPv4Mode   string                  `json:"ipv4_mode"`
	IPv4Static *types.IPv4StaticConfig `json:"ipv4_static,omitempty"`
	IPv6Mode   string                  `json:"ipv6_mode"`
	IPv6Static *types.IPv6StaticConfig `json:"ipv6_static,omitempty"`
}

// vim4NetworkSettings combines the NetworkManager profile with JetKVM's own
// settings. A failed read falls back to the stored values.
func vim4NetworkSettings() *types.NetworkConfig {
	settings := *config.NetworkConfig
	var profile vim4Profile
	if err := vim4Helper(15*time.Second, "network-get", nil, &profile); err != nil {
		networkLogger.Warn().Err(err).Msg("failed to read the NetworkManager profile")
	} else {
		settings.IPv4Mode = null.StringFrom(profile.IPv4Mode)
		settings.IPv4Static = profile.IPv4Static
		settings.IPv6Mode = null.StringFrom(profile.IPv6Mode)
		settings.IPv6Static = profile.IPv6Static
	}
	if hostname, err := os.Hostname(); err == nil {
		settings.Hostname = null.StringFrom(hostname)
	}
	// A device with a single DHCP client reports its name; the UI shows it as text.
	settings.DHCPClient = null.StringFrom(vim4DHCPClientName)
	return &settings
}

func vim4SetNetworkSettings(requested *types.NetworkConfig) (*RpcNetworkSettings, error) {
	next := *requested
	// Validate with a DHCP client name the appliance schema accepts; the
	// stored value is not used on VIM4.
	next.DHCPClient = config.NetworkConfig.DHCPClient
	if err := confparser.SetDefaultsAndValidate(&next); err != nil {
		return nil, err
	}
	current := vim4NetworkSettings()
	if next.MDNSMode.String != current.MDNSMode.String {
		if err := vim4Helper(30*time.Second, "mdns-set", map[string]string{"mode": next.MDNSMode.String}, nil); err != nil {
			return nil, fmt.Errorf("failed to apply mDNS mode: %w", err)
		}
	}
	if next.Hostname.String != "" && next.Hostname.String != current.Hostname.String {
		if err := vim4Helper(15*time.Second, "hostname-set", map[string]string{"hostname": next.Hostname.String}, nil); err != nil {
			return nil, fmt.Errorf("failed to set hostname: %w", err)
		}
	}
	profile := vim4Profile{IPv4Mode: next.IPv4Mode.String, IPv6Mode: next.IPv6Mode.String}
	if profile.IPv4Mode == "static" {
		profile.IPv4Static = next.IPv4Static
	}
	if profile.IPv6Mode == "static" {
		profile.IPv6Static = next.IPv6Static
	}
	// A new static address ends the browser session; send it to the new address.
	if state, err := vim4InterfaceState(); err == nil && profile.IPv4Static != nil &&
		profile.IPv4Static.Address.String != state.IPv4Address {
		address := fmt.Sprintf("%s:%d", profile.IPv4Static.Address.String, platform.HTTPPort)
		writeJSONRPCEvent("willReboot", &ota.PostRebootAction{
			HealthCheck: "//" + address + "/device/status",
			RedirectTo:  "//" + address,
		}, currentSession)
		time.Sleep(time.Second)
	}
	if err := vim4Helper(40*time.Second, "network-set", profile, nil); err != nil {
		return nil, fmt.Errorf("failed to apply network settings: %w", err)
	}
	config.NetworkConfig = &next
	if err := SaveConfig(); err != nil {
		return nil, err
	}
	return toRpcNetworkSettings(vim4NetworkSettings()), nil
}

// Wi-Fi

func vim4WifiAvailable() bool {
	matches, _ := filepath.Glob("/sys/class/net/*/wireless")
	return len(matches) > 0
}

// vim4WifiState tracks a connection attempt that runs after its RPC returns.
var vim4WifiState struct {
	sync.Mutex
	applying     bool
	connectError string
}

type vim4WifiSettings struct {
	Available    bool                `json:"available"`
	Connected    bool                `json:"connected"`
	Applying     bool                `json:"applying"`
	Switching    bool                `json:"switching"`
	StaticSSID   string              `json:"static_ssid"`
	ConnectError string              `json:"connect_error"`
	Channel      int                 `json:"channel"`
	RSSI         int                 `json:"rssi"`
	MaxNetworks  int                 `json:"max_networks"`
	Networks     []vim4SavedWifiInfo `json:"networks"`
}

type vim4SavedWifiInfo struct {
	SSID      string `json:"ssid"`
	Connected bool   `json:"connected"`
	Secure    bool   `json:"secure"`
}

type vim4WifiScan struct {
	Scanning bool `json:"scanning"`
	Networks []struct {
		SSID   string `json:"ssid"`
		RSSI   int    `json:"rssi"`
		Secure bool   `json:"secure"`
		Saved  bool   `json:"saved"`
	} `json:"networks"`
}

// Static IPv4 settings on the Network page belong to Ethernet, so no Wi-Fi
// network reports static_ssid. Ethernet stays connected while Wi-Fi changes,
// so the session is not switching networks.
func rpcGetWifiSettings() (*vim4WifiSettings, error) {
	var settings vim4WifiSettings
	if err := vim4Helper(15*time.Second, "wifi-get", nil, &settings); err != nil {
		return nil, err
	}
	vim4WifiState.Lock()
	settings.Applying, settings.ConnectError = vim4WifiState.applying, vim4WifiState.connectError
	vim4WifiState.Unlock()
	if settings.Networks == nil {
		settings.Networks = []vim4SavedWifiInfo{}
	}
	return &settings, nil
}

func rpcScanWifiNetworks(refresh bool) (*vim4WifiScan, error) {
	var scan vim4WifiScan
	if err := vim4Helper(45*time.Second, "wifi-scan", map[string]bool{"refresh": refresh}, &scan); err != nil {
		return nil, err
	}
	return &scan, nil
}

func rpcSaveWifiNetwork(ssid, password string, connect bool) (map[string]bool, error) {
	request := map[string]string{"ssid": ssid, "password": password}
	if err := vim4Helper(30*time.Second, "wifi-save", request, nil); err != nil {
		return nil, err
	}
	if connect {
		startVIM4WifiConnect(ssid)
	}
	return map[string]bool{"connecting": connect}, nil
}

func rpcConnectWifiNetwork(ssid string) error {
	startVIM4WifiConnect(ssid)
	return nil
}

func rpcRemoveWifiNetwork(ssid string) error {
	return vim4Helper(30*time.Second, "wifi-remove", map[string]string{"ssid": ssid}, nil)
}

func startVIM4WifiConnect(ssid string) {
	vim4WifiState.Lock()
	vim4WifiState.applying, vim4WifiState.connectError = true, ""
	vim4WifiState.Unlock()
	go func() {
		var result struct {
			Error string `json:"error"`
		}
		err := vim4Helper(60*time.Second, "wifi-connect", map[string]string{"ssid": ssid}, &result)
		if err != nil {
			result.Error = err.Error()
		}
		vim4WifiState.Lock()
		vim4WifiState.applying, vim4WifiState.connectError = false, result.Error
		vim4WifiState.Unlock()
	}()
}
