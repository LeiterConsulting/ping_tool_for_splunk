# Splunk Ping Monitor

A PowerShell 7.4+ based network monitoring tool that pings endpoints and sends results to Splunk for visualization and alerting.

## Features

- **Airgap Friendly**: No external modules required - uses only native PowerShell 7.4+ features
- **Flexible Endpoint Configuration**: CSV-based endpoint list with optional grouping and descriptions
- **Parallel Ping Execution**: Efficiently ping multiple endpoints simultaneously
- **Dual Output Modes**: 
  - File-based logging (for Splunk Universal Forwarder)
  - Direct Splunk HEC (HTTP Event Collector) integration
- **Structured JSON Logging**: Clean, parseable output for Splunk
- **Automatic Log Rotation**: Prevents disk space issues
- **Comprehensive Dashboard**: Pre-built Splunk dashboard for visualization

## Requirements

- **PowerShell 7.4 or higher** (required for parallel execution features)
- **Windows Server** (or Windows 10/11 for testing)
- **Splunk** (for data visualization)
  - Splunk Universal Forwarder (for file-based ingestion), OR
  - Splunk HEC Token (for direct HTTP ingestion)

> **Note**: This script has no external module dependencies and is fully airgap-compatible.

## Quick Start

### 1. Install PowerShell 7.4+

Download from: https://github.com/PowerShell/PowerShell/releases

Or install via winget:
```powershell
winget install Microsoft.PowerShell
```

### 2. Configure Endpoints

Edit `endpoints.csv` with your target endpoints:

**Minimal format (IP and hostname only):**
```csv
ip,hostname
192.168.1.1,router
10.0.0.50,app-server
8.8.8.8,google-dns
```

**Full format (with optional group and description):**
```csv
ip,hostname,group,description
192.168.1.1,router,network,Core Router
10.0.0.50,app-server,servers,Production Application Server
8.8.8.8,google-dns,external,Google Public DNS
```

> **Note**: The `group` and `description` columns are optional. If omitted, `group` defaults to "default" and `description` will be empty.

### 3. Configure Settings

Edit `config.psd1` to customize behavior. The file is fully commented with instructions:

```powershell
@{
    # Number of ICMP ping requests per endpoint per cycle
    pings_per_cycle = 4
    
    # Time (in seconds) between ping cycles
    cycle_interval_seconds = 60
    
    # Timeout (in milliseconds) for each ping request
    timeout_ms = 1000
    
    # Number of concurrent ping threads
    parallel_threads = 10
    
    # Output mode: "file", "hec", or "both"
    output_mode = "file"
    
    # Log file path
    log_path = "./logs/ping_results.log"
    
    # Splunk HEC settings (when output_mode is "hec" or "both")
    hec = @{
        enabled = $false
        url = "https://splunk.example.com:8088/services/collector/event"
        token = "your-hec-token-here"
        index = "main"
        sourcetype = "ping_monitor"
        verify_ssl = $true
    }
}
```

### 4. Run the Script

**Run continuously (default):**
```powershell
pwsh -File .\PingMonitor.ps1
```

**Run a single cycle:**
```powershell
pwsh -File .\PingMonitor.ps1 -RunOnce
```

**Use custom paths:**
```powershell
pwsh -File .\PingMonitor.ps1 -ConfigPath "C:\Config\myconfig.psd1" -EndpointsPath "C:\Config\myendpoints.csv"
```

## Output Format

The script outputs JSON-formatted events, one per line:

**Individual Ping Event:**
```json
{
  "timestamp": "2025-12-03T10:15:30.1234567-05:00",
  "target_ip": "192.168.1.1",
  "hostname": "router",
  "group": "network",
  "description": "Core Router",
  "status": "success",
  "latency_ms": 12,
  "ttl": 64,
  "ping_number": 1,
  "pings_in_cycle": 4,
  "record_type": "ping"
}
```

**Summary Event (per endpoint per cycle):**
```json
{
  "timestamp": "2025-12-03T10:15:30.5678901-05:00",
  "target_ip": "192.168.1.1",
  "hostname": "router",
  "group": "network",
  "description": "Core Router",
  "record_type": "summary",
  "pings_sent": 4,
  "pings_successful": 4,
  "pings_failed": 0,
  "packet_loss_pct": 0,
  "avg_latency_ms": 15.5,
  "min_latency_ms": 12,
  "max_latency_ms": 23
}
```

## Splunk Integration

### Option A: File-Based Ingestion (Splunk Universal Forwarder)

1. Set `output_mode: file` in `config.yaml`
2. Install Splunk Universal Forwarder on the monitoring server
3. Configure a file monitor input:

**inputs.conf:**
```ini
[monitor://C:\Path\To\Ping Tool for Splunk\logs\ping_results.log]
disabled = false
sourcetype = ping_monitor
index = your_index_name
```

**props.conf:**
```ini
[ping_monitor]
TIME_FORMAT = %Y-%m-%dT%H:%M:%S.%6N%:z
TIME_PREFIX = "timestamp":"
MAX_TIMESTAMP_LOOKAHEAD = 35
SHOULD_LINEMERGE = false
LINE_BREAKER = ([\r\n]+)
KV_MODE = json
```

### Option B: Direct HEC Integration

1. Create an HEC token in Splunk:
   - Settings → Data Inputs → HTTP Event Collector
   - New Token → Configure index and sourcetype

2. Configure `config.psd1`:
```powershell
@{
    output_mode = "hec"
    
    hec = @{
        enabled = $true
        url = "https://your-splunk-server:8088/services/collector/event"
        token = "your-hec-token-here"
        index = "network_monitoring"
        sourcetype = "ping_monitor"
        verify_ssl = $true
    }
}
```

### Installing the Dashboard

1. Open Splunk Web
2. Navigate to: Settings → User Interface → Dashboards
3. Click "Create New Dashboard" → "Classic Dashboards"
4. Choose "Dashboard" → "Source" (XML editor)
5. Copy contents of `splunk/ping_dashboard.xml`
6. **Find and Replace:**
   - Replace `YOUR_INDEX` with your actual index name
   - Replace `YOUR_SOURCETYPE` with your sourcetype (default: `ping_monitor`)
7. Save the dashboard

## Running as a Scheduled Task

### Using Task Scheduler (Recommended)

1. Open Task Scheduler (`taskschd.msc`)
2. Create a new task:
   - **General Tab:**
     - Name: `Splunk Ping Monitor`
     - Run whether user is logged on or not
     - Run with highest privileges
   - **Triggers Tab:**
     - New → At startup
     - (Optional) New → Daily, repeat every 1 minute for 1 day
   - **Actions Tab:**
     - New → Start a program
     - Program: `pwsh.exe`
     - Arguments: `-NoProfile -ExecutionPolicy Bypass -File "C:\Path\To\PingMonitor.ps1"`
     - Start in: `C:\Path\To\Ping Tool for Splunk`
   - **Settings Tab:**
     - Allow task to be run on demand
     - If the task fails, restart every 1 minute (up to 3 times)
     - Stop the task if it runs longer than: (disabled or 0)

### Using NSSM (Run as Windows Service)

For more robust service management, use NSSM (Non-Sucking Service Manager):

1. Run `Install-Service.ps1` as Administrator:
```powershell
.\Install-Service.ps1 -Install
```

2. To uninstall:
```powershell
.\Install-Service.ps1 -Uninstall
```

See `Install-Service.ps1` for manual NSSM configuration steps.

## Configuration Reference

### config.psd1 Options

| Setting | Default | Description |
|---------|---------|-------------|
| `pings_per_cycle` | 4 | Number of ICMP pings per endpoint per cycle |
| `cycle_interval_seconds` | 60 | Seconds between ping cycles |
| `timeout_ms` | 1000 | Timeout for each ping in milliseconds |
| `parallel_threads` | 10 | Number of concurrent ping operations |
| `output_mode` | file | Output destination: `file`, `hec`, or `both` |
| `log_path` | ./logs/ping_results.log | Path for log file output |
| `log_rotation_size_mb` | 50 | Max log size before rotation |
| `hec.enabled` | false | Enable HEC output |
| `hec.url` | (empty) | Splunk HEC endpoint URL |
| `hec.token` | (empty) | HEC authentication token |
| `hec.index` | main | Target Splunk index |
| `hec.sourcetype` | ping_monitor | Event sourcetype |
| `hec.verify_ssl` | true | Verify SSL certificates |

### endpoints.csv Columns

| Column | Required | Description |
|--------|----------|-------------|
| `ip` | Yes | IP address to ping |
| `hostname` | Yes | Friendly name for the endpoint |
| `group` | No | Grouping for dashboard filtering (default: "default") |
| `description` | No | Additional description text |

## Troubleshooting

### PowerShell Version Error
```
#Requires -Version 7.4
```
Ensure you're running PowerShell 7.4+:
```powershell
$PSVersionTable.PSVersion
```

### Permission Denied
Run PowerShell as Administrator or adjust execution policy:
```powershell
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
```

### HEC Connection Failed
- Verify the HEC URL is correct
- Check the HEC token is valid and enabled
- For self-signed certificates, set `verify_ssl: false`
- Ensure the HEC token has access to the target index

### High Memory Usage
Reduce `parallel_threads` in config.yaml if monitoring many endpoints.

## File Structure

```
Ping Tool for Splunk/
├── PingMonitor.ps1          # Main monitoring script
├── config.psd1              # Configuration file (PowerShell data file - fully commented)
├── endpoints.csv            # Target endpoints (full example)
├── endpoints_minimal.csv    # Minimal CSV example
├── logs/
│   └── ping_results.log     # Output logs (created automatically)
├── splunk/
│   └── ping_dashboard.xml   # Splunk dashboard
├── Install-Service.ps1      # Windows service installer
└── README.md                # This documentation
```

## License

MIT License - Feel free to modify and distribute.

## Contributing

Contributions welcome! Please submit issues and pull requests.
