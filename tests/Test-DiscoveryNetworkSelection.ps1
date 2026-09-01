#Requires -Version 7.4

[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repoRoot = Split-Path -Parent $PSScriptRoot
$scriptPath = Join-Path $repoRoot 'DiscoverEndpoints.ps1'
$embeddedPath = Join-Path $repoRoot 'go\internal\webui\assets\DiscoverEndpoints.ps1'

function Assert-True {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) {
        throw "Assertion failed: $Message"
    }
}

function Assert-Equal {
    param($Expected, $Actual, [string]$Message)
    if ($Expected -ne $Actual) {
        throw "Assertion failed: $Message. Expected '$Expected', got '$Actual'."
    }
}

function Assert-ThrowsLike {
    param([scriptblock]$Action, [string]$Pattern, [string]$Message)
    try {
        & $Action
    }
    catch {
        if ($_.Exception.Message -notmatch $Pattern) {
            throw "Assertion failed: $Message. Error '$($_.Exception.Message)' did not match '$Pattern'."
        }
        return
    }
    throw "Assertion failed: $Message. Expected an exception matching '$Pattern'."
}

function Import-DiscoveryFunction {
    param(
        [System.Management.Automation.Language.ScriptBlockAst]$Ast,
        [string]$Name
    )
    $definition = $Ast.Find({
        param($node)
        $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq $Name
    }, $true)
    if ($null -eq $definition) {
        throw "Function '$Name' was not found in $scriptPath"
    }
    return [scriptblock]::Create($definition.Extent.Text)
}

function New-MockAddress {
    param(
        [string]$IPAddress,
        [int]$PrefixLength = 24,
        [bool]$SkipAsSource = $false,
        [string]$AddressState = 'Preferred'
    )
    [PSCustomObject]@{
        IPAddress    = $IPAddress
        PrefixLength = $PrefixLength
        SkipAsSource = $SkipAsSource
        AddressState = $AddressState
    }
}

function New-MockConfiguration {
    param(
        [string]$Alias,
        [int]$Index,
        [object[]]$Addresses,
        [int]$InterfaceMetric = 25,
        [string]$Gateway = '',
        [string]$Status = 'Up'
    )
    [PSCustomObject]@{
        InterfaceAlias      = $Alias
        InterfaceIndex      = $Index
        NetAdapter          = [PSCustomObject]@{ Status = $Status; ifIndex = $Index }
        NetIPv4Interface    = [PSCustomObject]@{ InterfaceMetric = $InterfaceMetric }
        IPv4Address         = $Addresses
        IPv4DefaultGateway  = if ([string]::IsNullOrWhiteSpace($Gateway)) { $null } else { [PSCustomObject]@{ NextHop = $Gateway } }
    }
}

function New-MockRoute {
    param(
        [int]$InterfaceIndex,
        [int]$RouteMetric,
        [string]$NextHop,
        [string]$State = 'Alive'
    )
    [PSCustomObject]@{
        InterfaceIndex = $InterfaceIndex
        RouteMetric     = $RouteMetric
        NextHop         = $NextHop
        State           = $State
    }
}

$tokens = $null
$parseErrors = $null
$ast = [System.Management.Automation.Language.Parser]::ParseFile(
    $scriptPath,
    [ref]$tokens,
    [ref]$parseErrors
)
Assert-Equal 0 $parseErrors.Count 'The discovery script must parse without errors'

. (Import-DiscoveryFunction -Ast $ast -Name 'Get-DiscoveryLocalInfo')
. (Import-DiscoveryFunction -Ast $ast -Name 'Resolve-DiscoveryTarget')

function Get-LocalIPInfo { throw 'LOCAL_DETECTION_CALLED' }
$explicitInfo = Get-DiscoveryLocalInfo -TargetNetwork '10.20.30.0/24'
Assert-True ($null -eq $explicitInfo) 'An explicit target must bypass local adapter detection'
$explicitTarget = Resolve-DiscoveryTarget -TargetNetwork '10.20.30.0/24' -SubnetMask 24 -LocalInfo $null
Assert-Equal '10.20.30.0' $explicitTarget.BaseIP 'An explicit target must resolve without local adapter information'
Assert-Equal 24 $explicitTarget.CIDR 'An explicit target must retain its CIDR'
Assert-ThrowsLike { Get-DiscoveryLocalInfo -TargetNetwork '' } 'LOCAL_DETECTION_CALLED' 'Automatic mode must still request local adapter information'

. (Import-DiscoveryFunction -Ast $ast -Name 'Get-LocalIPInfo')

$script:MockConfigurations = @()
$script:MockRoutes = @()
function Get-NetIPConfiguration {
    param([object]$ErrorAction)
    return $script:MockConfigurations
}
function Get-NetRoute {
    param([string]$AddressFamily, [string]$DestinationPrefix, [object]$ErrorAction)
    return $script:MockRoutes
}

$script:MockConfigurations = @(
    (New-MockConfiguration -Alias 'Ethernet' -Index 10 -Addresses @(
        (New-MockAddress -IPAddress '10.0.0.20')
    ) -InterfaceMetric 20),
    (New-MockConfiguration -Alias 'Wi-Fi' -Index 11 -Addresses @(
        (New-MockAddress -IPAddress '192.168.50.20')
    ) -InterfaceMetric 50)
)
$script:MockRoutes = @(
    (New-MockRoute -InterfaceIndex 10 -RouteMetric 5 -NextHop '10.0.0.1'),
    (New-MockRoute -InterfaceIndex 11 -RouteMetric 1 -NextHop '192.168.50.1')
)
$routed = Get-LocalIPInfo
Assert-Equal '10.0.0.20' $routed.IPAddress 'The lowest effective default-route metric must win'
Assert-Equal '10.0.0.1' $routed.Gateway 'A route-table gateway must be used when IPv4DefaultGateway is empty'
Assert-Equal 'default_route' $routed.SelectionReason 'The route-based selection reason must be reported'

$script:MockConfigurations = @(
    (New-MockConfiguration -Alias 'Isolated LAN' -Index 12 -Addresses @(
        (New-MockAddress -IPAddress '172.20.0.9')
    ))
)
$script:MockRoutes = @()
$isolatedOutput = @(Get-LocalIPInfo 3>&1)
$isolated = $isolatedOutput | Where-Object { $_ -isnot [System.Management.Automation.WarningRecord] } | Select-Object -Last 1
$warning = $isolatedOutput | Where-Object { $_ -is [System.Management.Automation.WarningRecord] } | Select-Object -First 1
Assert-Equal '172.20.0.9' $isolated.IPAddress 'A single isolated IPv4 interface must be usable'
Assert-Equal 'only_active_ipv4_interface' $isolated.SelectionReason 'The isolated-interface selection reason must be reported'
Assert-True ($null -ne $warning -and [string]$warning -match 'No IPv4 default route') 'Isolated-interface fallback must emit an honest warning'

$script:MockConfigurations = @(
    (New-MockConfiguration -Alias 'LAN A' -Index 20 -Addresses @((New-MockAddress -IPAddress '10.1.0.9'))),
    (New-MockConfiguration -Alias 'LAN B' -Index 21 -Addresses @((New-MockAddress -IPAddress '10.2.0.9')))
)
$script:MockRoutes = @()
Assert-ThrowsLike { Get-LocalIPInfo } 'selection is ambiguous.*LAN A.*LAN B.*TargetNetwork' 'Multiple isolated interfaces must fail with candidate evidence'

$script:MockConfigurations = @(
    (New-MockConfiguration -Alias 'Route A' -Index 30 -Addresses @((New-MockAddress -IPAddress '10.3.0.9')) -InterfaceMetric 20),
    (New-MockConfiguration -Alias 'Route B' -Index 31 -Addresses @((New-MockAddress -IPAddress '10.4.0.9')) -InterfaceMetric 20)
)
$script:MockRoutes = @(
    (New-MockRoute -InterfaceIndex 30 -RouteMetric 5 -NextHop '10.3.0.1'),
    (New-MockRoute -InterfaceIndex 31 -RouteMetric 5 -NextHop '10.4.0.1')
)
Assert-ThrowsLike { Get-LocalIPInfo } 'share the lowest effective metric.*Route A.*Route B.*TargetNetwork' 'Equal best default routes must fail instead of selecting arbitrarily'

$script:MockConfigurations = @(
    (New-MockConfiguration -Alias 'Invalid LAN' -Index 40 -Addresses @(
        (New-MockAddress -IPAddress '127.0.0.1'),
        (New-MockAddress -IPAddress '169.254.10.20')
    ))
)
$script:MockRoutes = @()
Assert-ThrowsLike { Get-LocalIPInfo } 'No usable active IPv4 interface.*TargetNetwork' 'Loopback and APIPA addresses must not be selected'

$rootHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $scriptPath).Hash
$embeddedHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $embeddedPath).Hash
Assert-Equal $rootHash $embeddedHash 'The downloadable and embedded discovery scripts must remain identical'

Write-Host 'Discovery network-selection tests passed.' -ForegroundColor Green
