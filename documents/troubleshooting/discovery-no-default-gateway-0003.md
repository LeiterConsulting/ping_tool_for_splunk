# Discovery Cannot Find an Adapter with a Default Gateway

## Symptom

`DiscoverEndpoints.ps1` stops before scanning and reports:

```text
No active network adapter with a default gateway found
```

The error may occur on an isolated discovery host, a server that reaches monitored networks through static routes, a VPN-connected machine, or a host where `Get-NetIPConfiguration` does not expose the effective default route as `IPv4DefaultGateway`.

It can also occur when an explicit `-TargetNetwork` was supplied.

## Why it happens

The discovery script shipped with Ping Monitor v5.11.0 required an Up adapter with a populated `IPv4DefaultGateway` property. It performed that check before resolving `-TargetNetwork`.

That behavior was overly restrictive in two ways:

- a remote or statically routed target does not require the scanner to have a default gateway;
- `IPv4DefaultGateway` alone is not authoritative evidence of Windows route selection.

Reinstalling the Windows service does not repair this condition. The failing component is the discovery script used by the collector or invoked directly.

## Goal

Use Ping Monitor v5.11.1 or newer so explicit targets bypass local adapter detection and automatic discovery selects an interface from truthful Windows route evidence without guessing when multiple choices are equally plausible.

## Identify the installed versions

Check the collector:

```powershell
./pingmonitor.exe --version
```

Check the companion script header:

```powershell
Select-String -LiteralPath ./DiscoverEndpoints.ps1 -Pattern 'Version:'
```

The corrected discovery script reports version `2.5.3`. A v5.11.0 executable or script version 2.5.2 is affected.

## Upgrade safely

1. Download the v5.11.1 Windows ZIP from the tagged GitHub release.
2. Verify it against `SHA256SUMS_v5.11.1.txt`.
3. Stop the collector or service.
4. Replace `pingmonitor.exe` and any standalone `DiscoverEndpoints.ps1` in the deployment directory.
5. Preserve `config.psd1` or `config.json`, `endpoints.csv`, `ui_preferences.json`, and the discovery data directory.
6. Start the collector and confirm `/api/status` reports v5.11.1.

The collector resolves its discovery-script location at startup. If a standalone script exists beside the executable, configuration, or endpoints file, it takes precedence over the embedded fallback. Replace that file as well as the executable, then restart the collector. A service reinstall is unnecessary when deployment paths and service arguments are unchanged.

Direct command-line users can replace only `DiscoverEndpoints.ps1` for immediate recovery, but should align the executable and script at the next maintenance opportunity.

## Verify an explicit target

Choose a small approved subnet and an output path that does not replace production inventory:

```powershell
./DiscoverEndpoints.ps1 `
  -TargetNetwork '192.0.2.0/30' `
  -OutputPath ./discovery-verification.csv
```

The script should report:

```text
Skipping local adapter detection because an explicit discovery target was supplied.
```

The scan may find no responsive hosts if the example network is not routed in the environment. That is separate from adapter selection; use an approved reachable test range for a complete result test.

## Understand automatic local selection

When `-TargetNetwork` is omitted, v5.11.1:

1. considers only Up adapters with usable non-loopback, non-APIPA IPv4 addresses;
2. correlates them with active IPv4 default routes;
3. selects the lowest combined route and interface metric;
4. uses one isolated active IPv4 interface only when it is unambiguous, and reports a warning;
5. refuses to choose arbitrarily when multiple interfaces or equally preferred default routes remain.

An ambiguity error includes the interface aliases, indexes, metrics where available, and addresses. Specify the intended subnet with `-TargetNetwork` rather than disabling the guard.

## Collect sanitized route evidence

If automatic local discovery still cannot choose safely, collect:

```powershell
Get-NetIPConfiguration |
  Select-Object InterfaceAlias, InterfaceIndex, IPv4Address, IPv4DefaultGateway

Get-NetRoute -AddressFamily IPv4 -DestinationPrefix '0.0.0.0/0' |
  Select-Object InterfaceAlias, InterfaceIndex, NextHop, RouteMetric, State

Get-NetIPInterface -AddressFamily IPv4 |
  Select-Object InterfaceAlias, InterfaceIndex, ConnectionState, InterfaceMetric
```

Internal IP addresses and interface names may be sensitive. Redact them according to organizational policy. Do not include Splunk tokens, passwords, `.env` contents, or unrelated configuration secrets.

## Success criteria

- `pingmonitor.exe --version` reports v5.11.1 or newer.
- The standalone script, if present, reports version 2.5.3 or newer.
- Explicit targets start without local adapter detection.
- Automatic local discovery either selects a route with a stated reason or stops with actionable ambiguity evidence.
- Discovery produces review-only output without modifying `endpoints.csv` until an operator explicitly stages and saves approved results.
