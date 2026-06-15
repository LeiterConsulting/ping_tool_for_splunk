# Ping Monitor v5 (Go)

v5.3.1 is the current Go release of the PowerShell Ping Monitor rewrite, focused on long-running stability, bounded resource usage, drop-in compatibility with existing deployments, and an embedded admin UI for live management.

## Compatibility

- Uses `config.psd1` and `endpoints.csv` with the same keys/fields as v4.
- Emits the same event schema (`record_type=ping` and `record_type=summary`) and the same HEC envelope.
- Metrics payload structure matches v4 compat mode.
- Reloads `endpoints.csv` automatically between ping cycles; replacing the binary and restarting the service is enough to enable it.

## Dev endpoint flag

- `endpoints.csv` supports an optional `dev` column (`true|false`, `yes|no`, `1|0`).
- Endpoints with `dev=true` are emitted as `record_type=summary_dev` and `record_type=ping_dev`.
- Standard production searches that use `record_type=summary` are unaffected by dev/test devices.

## Runtime behavior

- Loads `config.psd1` at startup using `pwsh` when present, or a native `.psd1` parser when `pwsh` is unavailable.
- Reloads `endpoints.csv` at the start of each new cycle; changes apply on the next cycle, not mid-cycle.
- If `endpoints.csv` is temporarily invalid, v5 keeps using the last known good endpoint set and logs a warning.
- If Splunk HEC or the metrics endpoint is unavailable, v5 continues running and retries delivery with backoff instead of exiting.
- If raw ICMP is unavailable on the host, `ping.mode=auto` falls back once to the OS `ping` command and stays there for the rest of the run.
- An optional local web UI can be enabled with `--ui-listen`; it edits the live endpoint/config files, runs discovery, and can validate HEC and metrics delivery from the Settings page.
- If `config.psd1` and `endpoints.csv` already exist next to the binary, they are loaded at startup and immediately surfaced in the UI so existing deployments can be managed in place.

## Run

From repo root:

- Build: `go -C .\\go build -o pingmonitor.exe .\\go\\cmd\\pingmonitor`
- Run one cycle: `./pingmonitor.exe --run-once`
- Launch local UI only: `./pingmonitor.exe --ui-listen 127.0.0.1:8080 --ui-only`
- Run monitor plus UI together: `./pingmonitor.exe --ui-listen 127.0.0.1:8080`

With the default file names, the binary prefers `config.psd1` and `endpoints.csv` next to the executable, which keeps drop-in upgrades aligned with existing deployment folders.

### Embedded web UI

The embedded UI provides:

- a local admin shell served by the Go binary
- `GET /healthz`
- `GET /api/status`
- `GET` and `PUT /api/endpoints`
- `GET` and `PUT /api/config`
- `POST /api/discovery/run`
- `POST /api/output/test`

The discovery workflow ships with the binary through an embedded PowerShell script fallback, so drop-in deployments do not need a separate `DiscoverEndpoints.ps1` file just to use the admin UI.

Recommended local launch:

- `./pingmonitor.exe --ui-listen 127.0.0.1:8080 --ui-only`

Or run the monitor and UI together:

- `./pingmonitor.exe --ui-listen 127.0.0.1:8080`

The UI works directly against the same deployment files the runtime uses, so an existing install can be upgraded by dropping in the current binary and restarting it.

You can choose the local admin UI address and port explicitly with `--ui-listen`, for example `127.0.0.1:8090`.

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

- PowerShell: `pwsh -File .\go\build.ps1 -Version v5.3.1`
- Bash: `./go/build.sh dist` (set `VERSION` env var if desired)

## Windows service

- Build `pingmonitor.exe` and run `./Install-Service.ps1 -Install -Runtime go` from repo root.
- The Go runtime service enables the embedded admin UI on `127.0.0.1:8080` by default.
- Use `-UIListen` to choose a different bind address or port, or `-DisableUI` to keep the service headless.
- For an in-place upgrade, replace the existing binary in the deployment folder and restart the service; the co-located `config.psd1` and `endpoints.csv` are picked up automatically and shown in the UI.
