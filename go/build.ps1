param(
  [string]$OutDir = "dist",
  [string]$Version = "v5.2.1"
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$goDir = $PSScriptRoot
$dist = Join-Path $goDir $OutDir
New-Item -ItemType Directory -Path $dist -Force | Out-Null

Push-Location $goDir
try {
  $prevGOOS = $env:GOOS
  $prevGOARCH = $env:GOARCH
  $prevCGO = $env:CGO_ENABLED

  $env:CGO_ENABLED = "0"

  $targets = @(
    @{ GOOS='windows'; GOARCH='amd64'; OUT="pingmonitor_${Version}_windows_amd64.exe" },
    @{ GOOS='linux';   GOARCH='amd64'; OUT="pingmonitor_${Version}_linux_amd64" },
    @{ GOOS='linux';   GOARCH='arm64'; OUT="pingmonitor_${Version}_linux_arm64" },
    @{ GOOS='darwin';  GOARCH='amd64'; OUT="pingmonitor_${Version}_darwin_amd64" },
    @{ GOOS='darwin';  GOARCH='arm64'; OUT="pingmonitor_${Version}_darwin_arm64" }
  )

  foreach ($t in $targets) {
    $env:GOOS = $t.GOOS
    $env:GOARCH = $t.GOARCH
    $outPath = Join-Path $dist $t.OUT
    Write-Host "Building $($t.GOOS)/$($t.GOARCH) -> $outPath"
    go build -trimpath -ldflags "-s -w" -o $outPath .\cmd\pingmonitor
  }
}
finally {
  if ($null -ne $prevGOOS) { $env:GOOS = $prevGOOS } else { Remove-Item Env:GOOS -ErrorAction SilentlyContinue }
  if ($null -ne $prevGOARCH) { $env:GOARCH = $prevGOARCH } else { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue }
  if ($null -ne $prevCGO) { $env:CGO_ENABLED = $prevCGO } else { Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue }
  Pop-Location
}
