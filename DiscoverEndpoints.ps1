#Requires -Version 7.4
<#
.SYNOPSIS
    Network Discovery Tool - Discovers devices on local network and generates endpoints.csv

.DESCRIPTION
    This companion tool scans your local IP subnet to discover active hosts,
    resolves hostnames via DNS, and attempts to classify devices into groups.
    Output is an endpoints.csv file compatible with the current Go v5 runtime and legacy PowerShell runtimes.

.PARAMETER OutputPath
    Path for the generated CSV file. Defaults to discovered_endpoints.csv

.PARAMETER SubnetMask
    CIDR notation for subnet size. Defaults to /24 (255.255.255.0 = 254 hosts)

.PARAMETER Timeout
    Ping timeout in milliseconds. Defaults to 500ms for faster scanning.

.PARAMETER ThrottleLimit
    Maximum parallel scans. Defaults to 50 for fast scanning.

.PARAMETER IncludeOffline
    If specified, includes hosts that don't respond (commented out in CSV)

.EXAMPLE
    .\DiscoverEndpoints.ps1
    Scans local /24 subnet and outputs to discovered_endpoints.csv

.EXAMPLE
    .\DiscoverEndpoints.ps1 -SubnetMask 23 -OutputPath "my_network.csv"
    Scans /23 subnet (512 hosts) and outputs to custom file

.NOTES
    Author: Network Discovery Tool
    Version: 2.5.2
    Compatible with the current endpoint schema, including the optional trailing dev column.
#>

[CmdletBinding()]
param(
    [Parameter()]
    [string]$OutputPath = "discovered_endpoints.csv",

    [Parameter()]
    [ValidateRange(16, 30)]
    [int]$SubnetMask = 24,

    [Parameter()]
    [int]$Timeout = 500,

    [Parameter()]
    [int]$ThrottleLimit = 50,

    [Parameter()]
    [switch]$IncludeOffline
)

$ErrorActionPreference = "Stop"

#region Helper Functions

function Get-LocalIPInfo {
    <#
    .SYNOPSIS
        Gets the primary local IP address and network information
    #>
    
    # Get the primary network adapter with a default gateway (most likely the main network)
    $adapters = Get-NetIPConfiguration | Where-Object { 
        $null -ne $_.IPv4DefaultGateway -and
        $_.NetAdapter.Status -eq 'Up' 
    }
    
    if (-not $adapters) {
        throw "No active network adapter with a default gateway found"
    }
    
    # Prefer ethernet over wifi if both available
    $adapter = $adapters | Sort-Object { 
        if ($_.InterfaceAlias -match 'Ethernet|LAN') { 0 } 
        elseif ($_.InterfaceAlias -match 'Wi-Fi|Wireless') { 1 } 
        else { 2 } 
    } | Select-Object -First 1
    
    $ipAddress = ($adapter.IPv4Address | Select-Object -First 1).IPAddress
    $gateway = $adapter.IPv4DefaultGateway.NextHop
    $interfaceName = $adapter.InterfaceAlias
    
    return [PSCustomObject]@{
        IPAddress     = $ipAddress
        Gateway       = $gateway
        InterfaceName = $interfaceName
    }
}

function Get-SubnetRange {
    <#
    .SYNOPSIS
        Calculates the IP range for a given subnet
    #>
    param(
        [string]$BaseIP,
        [int]$CIDR
    )
    
    $ipBytes = [System.Net.IPAddress]::Parse($BaseIP).GetAddressBytes()
    [Array]::Reverse($ipBytes)
    $ipInt = [BitConverter]::ToUInt32($ipBytes, 0)
    
    # Calculate network address
    $maskBits = (-bnot [uint32]0) -shl (32 - $CIDR)
    $networkInt = $ipInt -band $maskBits
    
    # Calculate broadcast address
    $hostBits = (-bnot $maskBits) -band [uint32]::MaxValue
    $broadcastInt = $networkInt -bor $hostBits
    
    # Generate IP range (excluding network and broadcast)
    $ips = @()
    for ($i = $networkInt + 1; $i -lt $broadcastInt; $i++) {
        $bytes = [BitConverter]::GetBytes([uint32]$i)
        [Array]::Reverse($bytes)
        $ips += ([System.Net.IPAddress]::new($bytes)).ToString()
    }
    
    return $ips
}

function Resolve-HostnameFromIP {
    <#
    .SYNOPSIS
        Attempts to resolve hostname from IP via DNS reverse lookup
    #>
    param([string]$IPAddress)
    
    try {
        $dns = [System.Net.Dns]::GetHostEntry($IPAddress)
        $hostname = $dns.HostName
        
        # Clean up FQDN to short name
        if ($hostname -match '\.') {
            $shortName = $hostname.Split('.')[0]
            return $shortName
        }
        return $hostname
    }
    catch {
        return $null
    }
}

function Get-DeviceGroup {
    <#
    .SYNOPSIS
        Attempts to classify device into a group based on hostname patterns
    #>
    param(
        [string]$Hostname,
        [string]$IPAddress,
        [string]$GatewayIP
    )
    
    # If this is the gateway, it's network infrastructure
    if ($IPAddress -eq $GatewayIP) {
        return "network"
    }
    
    if (-not $Hostname) {
        return "unknown"
    }
    
    $hostLower = $Hostname.ToLower()
    
    # Network infrastructure
    if ($hostLower -match 'router|gateway|gw|firewall|fw|switch|sw|ap\d*|wap|wifi|unifi|ubnt|cisco|juniper|aruba|meraki') {
        return "network"
    }
    
    # Servers
    if ($hostLower -match 'server|srv|dc|domain|dns|dhcp|ad\d*|sql|db|web|app|mail|exchange|file|nas|san|esxi|vcenter|hyperv|proxmox') {
        return "servers"
    }
    
    # Workstations/Desktops
    if ($hostLower -match 'desktop|pc|workstation|ws|dt|computer') {
        return "workstations"
    }
    
    # Laptops
    if ($hostLower -match 'laptop|lt|nb|notebook|portable') {
        return "laptops"
    }
    
    # Printers
    if ($hostLower -match 'printer|prn|print|hp|canon|xerox|brother|epson|lexmark|ricoh') {
        return "printers"
    }
    
    # IoT/Smart devices
    if ($hostLower -match 'camera|cam|ipcam|nest|ring|doorbell|thermostat|alexa|echo|google-home|sonos|roku|appletv|firetv|chromecast|smart|iot|sensor') {
        return "iot"
    }
    
    # Phones/Mobile
    if ($hostLower -match 'iphone|android|phone|mobile|ipad|tablet') {
        return "mobile"
    }
    
    # Virtual machines
    if ($hostLower -match 'vm-|vm\d|virtual|-vm$') {
        return "virtual"
    }
    
    # Development
    if ($hostLower -match 'dev|test|staging|qa|lab|sandbox') {
        return "development"
    }
    
    return "endpoints"
}

function Get-DeviceClassification {
    <#
    .SYNOPSIS
        Returns a comprehensive device classification including entitytype, device type, and vendor
    #>
    param(
        [string]$Hostname,
        [string]$Group,
        [string]$IPAddress,
        [string]$GatewayIP
    )
    
    $hostLower = if ($Hostname) { $Hostname.ToLower() } else { "" }
    
    # Initialize result
    $result = @{
        EntityType = "endpoint"
        DeviceType = "unknown"
        Vendor     = ""
    }
    
    # Gateway detection
    if ($IPAddress -eq $GatewayIP) {
        $result.EntityType = "network"
        $result.DeviceType = "gateway"
        return $result
    }
    
    # Vendor detection from hostname patterns
    $vendorPatterns = @{
        'ubnt|unifi|ubiquiti'           = 'Ubiquiti'
        'cisco|meraki'                   = 'Cisco'
        'juniper|junos'                  = 'Juniper'
        'aruba'                          = 'Aruba'
        'fortinet|fortigate'             = 'Fortinet'
        'paloalto|pan-'                  = 'Palo Alto'
        'netgear'                        = 'Netgear'
        'linksys'                        = 'Linksys'
        'asus'                           = 'ASUS'
        'tplink|tp-link'                 = 'TP-Link'
        'dlink|d-link'                   = 'D-Link'
        'hp|hewlett'                     = 'HP'
        'dell|emc'                       = 'Dell'
        'lenovo'                         = 'Lenovo'
        'apple|mac|iphone|ipad'          = 'Apple'
        'microsoft|surface'              = 'Microsoft'
        'samsung'                        = 'Samsung'
        'synology'                       = 'Synology'
        'qnap'                           = 'QNAP'
        'vmware|esxi|vcenter'            = 'VMware'
        'proxmox'                        = 'Proxmox'
        'canon'                          = 'Canon'
        'xerox'                          = 'Xerox'
        'brother'                        = 'Brother'
        'epson'                          = 'Epson'
        'lexmark'                        = 'Lexmark'
        'ricoh'                          = 'Ricoh'
        'nest|google-home|chromecast'    = 'Google'
        'ring|echo|alexa|firetv|kindle'  = 'Amazon'
        'sonos'                          = 'Sonos'
        'roku'                           = 'Roku'
        'appletv'                        = 'Apple'
        'philips|hue'                    = 'Philips'
    }
    
    foreach ($pattern in $vendorPatterns.Keys) {
        if ($hostLower -match $pattern) {
            $result.Vendor = $vendorPatterns[$pattern]
            break
        }
    }
    
    # EntityType and DeviceType based on group
    switch ($Group) {
        "network" {
            $result.EntityType = "network"
            if ($hostLower -match 'router|gw|gateway') { 
                $result.DeviceType = "router" 
            }
            elseif ($hostLower -match 'switch|sw') { 
                $result.DeviceType = "switch" 
            }
            elseif ($hostLower -match 'ap\d*|wap|wifi|wireless') { 
                $result.DeviceType = "access-point" 
            }
            elseif ($hostLower -match 'firewall|fw|fortinet|paloalto') { 
                $result.DeviceType = "firewall" 
            }
            elseif ($hostLower -match 'unifi|ubnt') {
                $result.DeviceType = "controller"
            }
            else { 
                $result.DeviceType = "network-device" 
            }
        }
        "servers" {
            $result.EntityType = "server"
            if ($hostLower -match 'dc|domain|ad\d*') { 
                $result.DeviceType = "domain-controller" 
            }
            elseif ($hostLower -match 'dns') { 
                $result.DeviceType = "dns-server" 
            }
            elseif ($hostLower -match 'dhcp') { 
                $result.DeviceType = "dhcp-server" 
            }
            elseif ($hostLower -match 'sql|db|database|mysql|postgres|mongo') { 
                $result.DeviceType = "database-server" 
            }
            elseif ($hostLower -match 'web|iis|apache|nginx') { 
                $result.DeviceType = "web-server" 
            }
            elseif ($hostLower -match 'file|nas|san|synology|qnap') { 
                $result.DeviceType = "file-server" 
            }
            elseif ($hostLower -match 'mail|exchange|smtp') { 
                $result.DeviceType = "mail-server" 
            }
            elseif ($hostLower -match 'esxi|vcenter|hyperv|proxmox|hyper-v') { 
                $result.DeviceType = "hypervisor" 
            }
            elseif ($hostLower -match 'backup|veeam|acronis') {
                $result.DeviceType = "backup-server"
            }
            elseif ($hostLower -match 'app') {
                $result.DeviceType = "application-server"
            }
            else { 
                $result.DeviceType = "server" 
            }
        }
        "workstations" {
            $result.EntityType = "endpoint"
            $result.DeviceType = "desktop"
        }
        "laptops" {
            $result.EntityType = "endpoint"
            $result.DeviceType = "laptop"
        }
        "printers" {
            $result.EntityType = "peripheral"
            $result.DeviceType = "printer"
        }
        "iot" {
            $result.EntityType = "iot"
            if ($hostLower -match 'camera|cam|ipcam') { 
                $result.DeviceType = "camera" 
            }
            elseif ($hostLower -match 'doorbell|ring') { 
                $result.DeviceType = "doorbell" 
            }
            elseif ($hostLower -match 'thermostat|nest|ecobee') { 
                $result.DeviceType = "thermostat" 
            }
            elseif ($hostLower -match 'alexa|echo|google-home|homepod') { 
                $result.DeviceType = "smart-speaker" 
            }
            elseif ($hostLower -match 'sonos|speaker') { 
                $result.DeviceType = "speaker" 
            }
            elseif ($hostLower -match 'roku|appletv|firetv|chromecast') { 
                $result.DeviceType = "streaming-device" 
            }
            elseif ($hostLower -match 'hue|bulb|light') { 
                $result.DeviceType = "smart-lighting" 
            }
            elseif ($hostLower -match 'sensor') { 
                $result.DeviceType = "sensor" 
            }
            elseif ($hostLower -match 'tv|television') { 
                $result.DeviceType = "smart-tv" 
            }
            else { 
                $result.DeviceType = "iot-device" 
            }
        }
        "mobile" {
            $result.EntityType = "mobile"
            if ($hostLower -match 'iphone') { 
                $result.DeviceType = "smartphone"
                $result.Vendor = "Apple"
            }
            elseif ($hostLower -match 'ipad') { 
                $result.DeviceType = "tablet"
                $result.Vendor = "Apple"
            }
            elseif ($hostLower -match 'android') { 
                $result.DeviceType = "smartphone" 
            }
            elseif ($hostLower -match 'tablet') { 
                $result.DeviceType = "tablet" 
            }
            else { 
                $result.DeviceType = "mobile-device" 
            }
        }
        "virtual" {
            $result.EntityType = "virtual"
            $result.DeviceType = "virtual-machine"
        }
        "development" {
            $result.EntityType = "development"
            if ($hostLower -match 'dev') { 
                $result.DeviceType = "dev-workstation" 
            }
            elseif ($hostLower -match 'test|qa') { 
                $result.DeviceType = "test-system" 
            }
            elseif ($hostLower -match 'staging') { 
                $result.DeviceType = "staging-server" 
            }
            elseif ($hostLower -match 'lab|sandbox') { 
                $result.DeviceType = "lab-system" 
            }
            else { 
                $result.DeviceType = "dev-system" 
            }
        }
        "unknown" {
            $result.EntityType = "unknown"
            $result.DeviceType = "unknown"
        }
        default {
            $result.EntityType = "endpoint"
            $result.DeviceType = "endpoint"
        }
    }
    
    return $result
}

function Get-DeviceDescription {
    <#
    .SYNOPSIS
        Generates a description based on hostname and group
    #>
    param(
        [string]$Hostname,
        [string]$Group,
        [string]$IPAddress,
        [string]$GatewayIP
    )
    
    if ($IPAddress -eq $GatewayIP) {
        return "Default gateway / router"
    }
    
    if (-not $Hostname) {
        return "Unknown device - no DNS record"
    }
    
    $hostLower = $Hostname.ToLower()
    
    # Try to generate meaningful description
    switch ($Group) {
        "network" {
            if ($hostLower -match 'router|gw|gateway') { return "Network router" }
            if ($hostLower -match 'switch|sw') { return "Network switch" }
            if ($hostLower -match 'ap|wap|wifi') { return "Wireless access point" }
            if ($hostLower -match 'firewall|fw') { return "Firewall" }
            return "Network infrastructure device"
        }
        "servers" {
            if ($hostLower -match 'dc|domain|ad') { return "Domain controller" }
            if ($hostLower -match 'dns') { return "DNS server" }
            if ($hostLower -match 'dhcp') { return "DHCP server" }
            if ($hostLower -match 'sql|db') { return "Database server" }
            if ($hostLower -match 'web') { return "Web server" }
            if ($hostLower -match 'file|nas') { return "File server / NAS" }
            if ($hostLower -match 'mail|exchange') { return "Mail server" }
            if ($hostLower -match 'esxi|vcenter|hyperv|proxmox') { return "Hypervisor / VM host" }
            return "Server"
        }
        "printers" { return "Network printer" }
        "iot" { return "IoT / Smart device" }
        "mobile" { return "Mobile device" }
        "virtual" { return "Virtual machine" }
        "workstations" { return "Desktop workstation" }
        "laptops" { return "Laptop computer" }
        "development" { return "Development / Test system" }
        default { return "Discovered endpoint" }
    }
}

#endregion

#region Main Discovery Logic

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  Network Endpoint Discovery Tool" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# Get local network info
Write-Host "Detecting local network configuration..." -ForegroundColor Yellow
$localInfo = Get-LocalIPInfo
Write-Host "  Local IP:    $($localInfo.IPAddress)" -ForegroundColor White
Write-Host "  Gateway:     $($localInfo.Gateway)" -ForegroundColor White
Write-Host "  Interface:   $($localInfo.InterfaceName)" -ForegroundColor White
Write-Host ""

# Calculate IP range
Write-Host "Calculating subnet range (/$SubnetMask)..." -ForegroundColor Yellow
$ipRange = Get-SubnetRange -BaseIP $localInfo.IPAddress -CIDR $SubnetMask
$totalHosts = $ipRange.Count
Write-Host "  Hosts to scan: $totalHosts" -ForegroundColor White
Write-Host ""

# Perform parallel ping sweep
Write-Host "Scanning network (this may take a moment)..." -ForegroundColor Yellow
Write-Host ""

$startTime = Get-Date
$discoveredHosts = [System.Collections.Concurrent.ConcurrentBag[PSCustomObject]]::new()
$ipRange | ForEach-Object -Parallel {
    $ip = $_
    $timeout = $using:Timeout
    $bag = $using:discoveredHosts
    
    try {
        $ping = Test-Connection -TargetName $ip -Count 1 -TimeoutSeconds ([math]::Ceiling($timeout / 1000)) -ErrorAction SilentlyContinue
        
        if ($ping -and $ping.Status -eq 'Success') {
            $bag.Add([PSCustomObject]@{
                IP      = $ip
                Latency = $ping.Latency
                Online  = $true
            })
        }
    }
    catch {
        # Ignore ping failures
    }
} -ThrottleLimit $ThrottleLimit

$scanDuration = (Get-Date) - $startTime
$onlineHosts = @($discoveredHosts | Where-Object { $_.Online })

Write-Host "Scan complete in $([math]::Round($scanDuration.TotalSeconds, 1)) seconds" -ForegroundColor Green
Write-Host "  Found $($onlineHosts.Count) active hosts out of $totalHosts scanned" -ForegroundColor White
Write-Host ""

if ($onlineHosts.Count -eq 0) {
    Write-Warning "No hosts found! Check your network connection and firewall settings."
    exit 1
}

# Resolve hostnames and classify devices
Write-Host "Resolving hostnames and classifying devices..." -ForegroundColor Yellow

$endpoints = @()
$counter = 0

foreach ($host_ in ($onlineHosts | Sort-Object { [version]($_.IP -replace '(\d+)\.(\d+)\.(\d+)\.(\d+)', '$1.$2.$3.$4') })) {
    $counter++
    $pct = [math]::Round(($counter / $onlineHosts.Count) * 100)
    Write-Progress -Activity "Resolving hostnames" -Status "$($host_.IP)" -PercentComplete $pct
    
    $hostname = Resolve-HostnameFromIP -IPAddress $host_.IP
    
    if (-not $hostname) {
        # Generate a placeholder hostname from IP
        $hostname = "host-$($host_.IP -replace '\.', '-')"
    }
    
    $group = Get-DeviceGroup -Hostname $hostname -IPAddress $host_.IP -GatewayIP $localInfo.Gateway
    $description = Get-DeviceDescription -Hostname $hostname -Group $group -IPAddress $host_.IP -GatewayIP $localInfo.Gateway
    $classification = Get-DeviceClassification -Hostname $hostname -Group $group -IPAddress $host_.IP -GatewayIP $localInfo.Gateway
    
    $endpoints += [PSCustomObject]@{
        ip               = $host_.IP
        hostname         = $hostname.ToLower()
        group            = $group
        description      = $description
        entitytype       = $classification.EntityType
        device           = $classification.DeviceType
        vendor           = $classification.Vendor
        additional_notes = ""
        dev              = if ($group -eq 'development') { $true } else { $false }
        latency_ms       = $host_.Latency
    }
}

Write-Progress -Activity "Resolving hostnames" -Completed

# Display summary by group
Write-Host ""
Write-Host "Discovery Summary:" -ForegroundColor Cyan
Write-Host "----------------------------------------" -ForegroundColor Gray

$groupSummary = $endpoints | Group-Object -Property group | Sort-Object Count -Descending
foreach ($grp in $groupSummary) {
    Write-Host "  $($grp.Name): $($grp.Count)" -ForegroundColor White
}

Write-Host "----------------------------------------" -ForegroundColor Gray
Write-Host "  Total: $($endpoints.Count) endpoints" -ForegroundColor Green
Write-Host ""

# Display entity type breakdown
Write-Host "Entity Types:" -ForegroundColor Cyan
Write-Host "----------------------------------------" -ForegroundColor Gray
$entitySummary = $endpoints | Group-Object -Property entitytype | Sort-Object Count -Descending
foreach ($ent in $entitySummary) {
    Write-Host "  $($ent.Name): $($ent.Count)" -ForegroundColor White
}
Write-Host "----------------------------------------" -ForegroundColor Gray
Write-Host ""

# Display vendor breakdown (only those detected)
$vendorEndpoints = $endpoints | Where-Object { $_.vendor -ne "" }
if ($vendorEndpoints.Count -gt 0) {
    Write-Host "Vendors Detected:" -ForegroundColor Cyan
    Write-Host "----------------------------------------" -ForegroundColor Gray
    $vendorSummary = $vendorEndpoints | Group-Object -Property vendor | Sort-Object Count -Descending
    foreach ($v in $vendorSummary) {
        Write-Host "  $($v.Name): $($v.Count)" -ForegroundColor White
    }
    Write-Host "----------------------------------------" -ForegroundColor Gray
    Write-Host ""
}

# Export to CSV
Write-Host "Exporting to $OutputPath..." -ForegroundColor Yellow

# Create CSV with all columns for the current endpoint schema
$csvData = $endpoints | Select-Object ip, hostname, group, description, entitytype, device, vendor, additional_notes, dev
$csvData | Export-Csv -Path $OutputPath -NoTypeInformation -Encoding UTF8

Write-Host ""
Write-Host "========================================" -ForegroundColor Green
Write-Host "  Discovery Complete!" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host ""
Write-Host "Output file: $OutputPath" -ForegroundColor White
Write-Host "CSV Columns: ip, hostname, group, description, entitytype, device, vendor, additional_notes, dev" -ForegroundColor Gray
Write-Host ""
Write-Host "Next steps:" -ForegroundColor Yellow
Write-Host "  1. Review the generated CSV file" -ForegroundColor Gray
Write-Host "  2. Edit hostnames/groups/descriptions as needed" -ForegroundColor Gray
Write-Host "  3. Add vendor info where it wasn't auto-detected" -ForegroundColor Gray
Write-Host "  4. Rename to endpoints.csv for use with pingmonitor.exe or legacy PingMonitor scripts" -ForegroundColor Gray
Write-Host ""

# Show preview
Write-Host "Preview (first 10 entries):" -ForegroundColor Cyan
$endpoints | Select-Object ip, hostname, group, entitytype, device, vendor, dev | Select-Object -First 10 | Format-Table -AutoSize

#endregion
