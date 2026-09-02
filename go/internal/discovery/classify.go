package discovery

import (
	"regexp"
	"strings"
	"sync"
)

type namedPattern struct {
	Expression *regexp.Regexp
	Value      string
}

var vendorPatterns = []namedPattern{
	{regexp.MustCompile(`ubnt|unifi|ubiquiti`), "Ubiquiti"},
	{regexp.MustCompile(`cisco|meraki`), "Cisco"},
	{regexp.MustCompile(`juniper|junos`), "Juniper"},
	{regexp.MustCompile(`aruba`), "Aruba"},
	{regexp.MustCompile(`fortinet|fortigate`), "Fortinet"},
	{regexp.MustCompile(`paloalto|pan-`), "Palo Alto"},
	{regexp.MustCompile(`netgear`), "Netgear"},
	{regexp.MustCompile(`linksys`), "Linksys"},
	{regexp.MustCompile(`asus`), "ASUS"},
	{regexp.MustCompile(`tplink|tp-link`), "TP-Link"},
	{regexp.MustCompile(`dlink|d-link`), "D-Link"},
	{regexp.MustCompile(`hp|hewlett`), "HP"},
	{regexp.MustCompile(`dell|emc`), "Dell"},
	{regexp.MustCompile(`lenovo`), "Lenovo"},
	{regexp.MustCompile(`apple|mac|iphone|ipad`), "Apple"},
	{regexp.MustCompile(`microsoft|surface`), "Microsoft"},
	{regexp.MustCompile(`samsung`), "Samsung"},
	{regexp.MustCompile(`synology`), "Synology"},
	{regexp.MustCompile(`qnap`), "QNAP"},
	{regexp.MustCompile(`vmware|esxi|vcenter`), "VMware"},
	{regexp.MustCompile(`proxmox`), "Proxmox"},
	{regexp.MustCompile(`canon`), "Canon"},
	{regexp.MustCompile(`xerox`), "Xerox"},
	{regexp.MustCompile(`brother`), "Brother"},
	{regexp.MustCompile(`epson`), "Epson"},
	{regexp.MustCompile(`lexmark`), "Lexmark"},
	{regexp.MustCompile(`ricoh`), "Ricoh"},
	{regexp.MustCompile(`nest|google-home|chromecast`), "Google"},
	{regexp.MustCompile(`ring|echo|alexa|firetv|kindle`), "Amazon"},
	{regexp.MustCompile(`sonos`), "Sonos"},
	{regexp.MustCompile(`roku`), "Roku"},
	{regexp.MustCompile(`appletv`), "Apple"},
	{regexp.MustCompile(`philips|hue`), "Philips"},
}

var groupPatterns = []namedPattern{
	{regexp.MustCompile(`router|gateway|gw|firewall|fw|switch|sw|ap\d*|wap|wifi|unifi|ubnt|cisco|juniper|aruba|meraki`), "network"},
	{regexp.MustCompile(`server|srv|dc|domain|dns|dhcp|ad\d*|sql|db|web|app|mail|exchange|file|nas|san|esxi|vcenter|hyperv|proxmox`), "servers"},
	{regexp.MustCompile(`desktop|pc|workstation|ws|dt|computer`), "workstations"},
	{regexp.MustCompile(`laptop|lt|nb|notebook|portable`), "laptops"},
	{regexp.MustCompile(`printer|prn|print|hp|canon|xerox|brother|epson|lexmark|ricoh`), "printers"},
	{regexp.MustCompile(`camera|cam|ipcam|nest|ring|doorbell|thermostat|alexa|echo|google-home|sonos|roku|appletv|firetv|chromecast|smart|iot|sensor`), "iot"},
	{regexp.MustCompile(`iphone|android|phone|mobile|ipad|tablet`), "mobile"},
	{regexp.MustCompile(`vm-|vm\d|virtual|-vm$`), "virtual"},
	{regexp.MustCompile(`dev|test|staging|qa|lab|sandbox`), "development"},
}

var classificationPatternCache sync.Map

func classifyHostname(hostname string) (group, entityType, device, vendor, description string) {
	host := strings.ToLower(strings.TrimSpace(hostname))
	group = classifyGroup(host)
	entityType, device = classifyDevice(host, group)
	for _, candidate := range vendorPatterns {
		if candidate.Expression.MatchString(host) {
			vendor = candidate.Value
			break
		}
	}
	description = describeDevice(host, group)
	return
}

func classifyGroup(host string) string {
	for _, candidate := range groupPatterns {
		if candidate.Expression.MatchString(host) {
			return candidate.Value
		}
	}
	return "endpoints"
}

func classifyDevice(host, group string) (string, string) {
	switch group {
	case "network":
		switch {
		case matches(host, `router|gw|gateway`):
			return "network", "router"
		case matches(host, `switch|sw`):
			return "network", "switch"
		case matches(host, `ap\d*|wap|wifi|wireless`):
			return "network", "access-point"
		case matches(host, `firewall|fw|fortinet|paloalto`):
			return "network", "firewall"
		case matches(host, `unifi|ubnt`):
			return "network", "controller"
		default:
			return "network", "network-device"
		}
	case "servers":
		switch {
		case matches(host, `dc|domain|ad\d*`):
			return "server", "domain-controller"
		case matches(host, `dns`):
			return "server", "dns-server"
		case matches(host, `dhcp`):
			return "server", "dhcp-server"
		case matches(host, `sql|db|database|mysql|postgres|mongo`):
			return "server", "database-server"
		case matches(host, `web|iis|apache|nginx`):
			return "server", "web-server"
		case matches(host, `file|nas|san|synology|qnap`):
			return "server", "file-server"
		case matches(host, `mail|exchange|smtp`):
			return "server", "mail-server"
		case matches(host, `esxi|vcenter|hyperv|proxmox|hyper-v`):
			return "server", "hypervisor"
		case matches(host, `backup|veeam|acronis`):
			return "server", "backup-server"
		case matches(host, `app`):
			return "server", "application-server"
		default:
			return "server", "server"
		}
	case "workstations":
		return "endpoint", "desktop"
	case "laptops":
		return "endpoint", "laptop"
	case "printers":
		return "peripheral", "printer"
	case "iot":
		switch {
		case matches(host, `camera|cam|ipcam`):
			return "iot", "camera"
		case matches(host, `doorbell|ring`):
			return "iot", "doorbell"
		case matches(host, `thermostat|nest|ecobee`):
			return "iot", "thermostat"
		case matches(host, `alexa|echo|google-home|homepod`):
			return "iot", "smart-speaker"
		case matches(host, `sonos|speaker`):
			return "iot", "speaker"
		case matches(host, `roku|appletv|firetv|chromecast`):
			return "iot", "streaming-device"
		case matches(host, `hue|bulb|light`):
			return "iot", "smart-lighting"
		case matches(host, `sensor`):
			return "iot", "sensor"
		case matches(host, `tv|television`):
			return "iot", "smart-tv"
		default:
			return "iot", "iot-device"
		}
	case "mobile":
		if matches(host, `iphone|android|phone`) {
			return "mobile", "smartphone"
		}
		if matches(host, `ipad|tablet`) {
			return "mobile", "tablet"
		}
		return "mobile", "mobile-device"
	case "virtual":
		return "virtual", "virtual-machine"
	case "development":
		switch {
		case matches(host, `dev`):
			return "development", "dev-workstation"
		case matches(host, `test|qa`):
			return "development", "test-system"
		case matches(host, `staging`):
			return "development", "staging-server"
		default:
			return "development", "lab-system"
		}
	default:
		return "endpoint", "endpoint"
	}
}

func describeDevice(host, group string) string {
	switch group {
	case "network":
		if matches(host, `router|gw|gateway`) {
			return "Network router"
		}
		if matches(host, `switch|sw`) {
			return "Network switch"
		}
		if matches(host, `ap|wap|wifi`) {
			return "Wireless access point"
		}
		if matches(host, `firewall|fw`) {
			return "Firewall"
		}
		return "Network infrastructure device"
	case "servers":
		return "Server"
	case "printers":
		return "Network printer"
	case "iot":
		return "IoT / Smart device"
	case "mobile":
		return "Mobile device"
	case "virtual":
		return "Virtual machine"
	case "workstations":
		return "Desktop workstation"
	case "laptops":
		return "Laptop computer"
	case "development":
		return "Development / Test system"
	default:
		return "Discovered endpoint"
	}
}

func matches(value, pattern string) bool {
	if cached, ok := classificationPatternCache.Load(pattern); ok {
		return cached.(*regexp.Regexp).MatchString(value)
	}
	compiled := regexp.MustCompile(pattern)
	actual, _ := classificationPatternCache.LoadOrStore(pattern, compiled)
	return actual.(*regexp.Regexp).MatchString(value)
}
