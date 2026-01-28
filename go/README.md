# Ping Monitor v5 (Go)

v5 is a Go rewrite of the PowerShell Ping Monitor (v4) focused on long-running stability and bounded resource usage.

## Compatibility

- Uses `config.psd1` and `endpoints.csv` with the same keys/fields as v4.
- Emits the same event schema (`record_type=ping` and `record_type=summary`) and the same HEC envelope.
- Metrics payload structure matches v4 compat mode.

## Run

From repo root:

- Build: `go -C .\\go build -o pingmonitor.exe .\\go\\cmd\\pingmonitor`
- Run one cycle: `./pingmonitor.exe --run-once`

By default it looks for `config.psd1` and `endpoints.csv` in the working directory.

### Ping mode

v5 supports multiple ping strategies (for compatibility with locked-down hosts):

- `auto` (default): try raw ICMP via Go, then fall back to OS `ping` if ICMP sockets aren't available
- `raw`: raw ICMP via Go only (no fallback)
- `exec`: OS `ping` only

Override via CLI:

- `./pingmonitor.exe --run-once --ping-mode auto`

Or via config:

```powershell
ping = @{
	mode = "auto"   # auto|raw|exec
}
```

### HEC dead-letter rotation

If `hec.drop_on_failure = $true` and `hec.dead_letter_path` is set, v5 can rotate that dead-letter file:

```powershell
hec = @{
	dead_letter_path = "./logs/hec_deadletter.ndjson"
	dead_letter_rotation_size_mb = 50
}
```

## Build all platforms

- PowerShell: `pwsh -File .\\go\\build.ps1 -Version v5.0.0-dev`
- Bash: `./go/build.sh dist` (set `VERSION` env var if desired)
