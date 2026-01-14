# ============================================
# Splunk Ping Monitor Configuration
# ============================================
# This file controls all settings for the Ping Monitor script.
# Modify values below to customize behavior for your environment.
# ============================================

@{
    # ----------------------------------------
    # PING SETTINGS
    # ----------------------------------------
    
    # Number of ICMP ping requests to send per endpoint per cycle
    # Higher values give more accurate packet loss statistics
    # Recommended: 4-10
    pings_per_cycle = 4
    
    # Time (in seconds) to wait between ping cycles
    # This is the delay after completing all pings before starting the next round
    # Recommended: 60 for standard monitoring, 10-30 for critical systems
    cycle_interval_seconds = 60
    
    # Timeout (in milliseconds) for each individual ping request
    # If a ping doesn't respond within this time, it's marked as failed
    # Recommended: 1000-5000 depending on network latency
    timeout_ms = 1000
    
    # Number of endpoints to ping simultaneously (parallel threads)
    # Higher values = faster cycles but more network/CPU load
    # Recommended: 10-50 depending on endpoint count and system resources
    parallel_threads = 10
    
    # ----------------------------------------
    # EVENT SETTINGS
    # ----------------------------------------
    
    # Whether to emit individual ping events (record_type=ping) in addition to summary events
    # Set to $true to emit both ping + summary (original behavior, more detailed)
    # Set to $false to emit summary events only (reduces event volume by ~80%)
    # Summary events (record_type=summary) are always emitted regardless of this setting
    emit_individual_pings = $true
    
    # ----------------------------------------
    # OUTPUT SETTINGS
    # ----------------------------------------
    
    # Where to send ping results. Options:
    #   "file" - Write JSON logs to local file (for Splunk Universal Forwarder ingestion)
    #   "hec"  - Send directly to Splunk via HTTP Event Collector
    #   "both" - Use both methods simultaneously
    output_mode = "file"
    
    # Path to the log file (used when output_mode is "file" or "both")
    # Can be absolute path or relative to script directory
    # The directory will be created automatically if it doesn't exist
    # Example: "C:\Logs\ping_results.log" or "./logs/ping_results.log"
    log_path = "./logs/ping_results.log"
    
    # Maximum log file size in megabytes before automatic rotation
    # When exceeded, the current log is archived with a timestamp
    # Old rotated logs are automatically cleaned up (keeps last 5)
    # Recommended: 50-100 MB
    log_rotation_size_mb = 50
    
    # ----------------------------------------
    # SPLUNK HEC (HTTP Event Collector) SETTINGS
    # ----------------------------------------
    # Only used when output_mode is "hec" or "both"
    # 
    # To set up HEC in Splunk:
    #   1. Go to Settings > Data Inputs > HTTP Event Collector
    #   2. Click "New Token" and follow the wizard
    #   3. Note the token value and configure allowed indexes
    #   4. Ensure HEC is enabled (Settings > Data Inputs > HTTP Event Collector > Global Settings)
    #
    hec = @{
        # Set to $true to enable sending data to Splunk HEC
        # Must also set url and token below
        enabled = $false
        
        # Full URL to your Splunk HEC endpoint
        # Format: https://<splunk-server>:<port>/services/collector/event
        # Default HEC port is 8088 (free trials) or 443 (Splunk Cloud)
        #
        # On-Premises Examples:
        #   "https://splunk.mycompany.com:8088/services/collector/event"
        #   "https://10.0.0.50:8088/services/collector/event"
        #
        # Splunk Cloud Examples:
        #   AWS:      "https://http-inputs-myinstance.splunkcloud.com:443/services/collector/event"
        #   GCP/Azure: "https://http-inputs.myinstance.splunkcloud.com:443/services/collector/event"
        #   FedRAMP:  "https://http-inputs.myinstance.splunkcloudgc.com:443/services/collector/event"
        #
        url = ""
        
        # HEC authentication token (generated in Splunk)
        # This is a GUID-like string, e.g., "12345678-1234-1234-1234-123456789012"
        # Keep this secret! Consider using environment variables in production.
        token = ""
        
        # Target index in Splunk where events will be stored
        # This index must exist and be allowed by your HEC token configuration
        # Common choices: "main", "network", "infrastructure", "ping_monitor"
        index = "ping"
        
        # Sourcetype for the events (used for parsing and searching in Splunk)
        # This should match what you configure in the Splunk dashboard
        # Default: "ping_monitor"
        sourcetype = "ping_monitor"
        
        # Whether to verify SSL/TLS certificates when connecting to Splunk
        # Set to $false if using self-signed certificates (not recommended for production)
        # Set to $true for production environments with valid certificates
        verify_ssl = $true
        
        # SSL/TLS protocol version to use for HEC connections
        # Options: "Default", "Tls12", "Tls13", "Tls11", "Tls"
        # Use "Tls12" if your Splunk server requires TLS 1.2 specifically
        # Use "Default" to let the system negotiate the best available protocol
        # Most Splunk servers work best with "Tls12"
        ssl_protocol = "Default"
        
        # ----------------------------------------
        # v3.2.0 HEC BATCHING & RETRY SETTINGS (Optional)
        # ----------------------------------------
        
        # Number of events per HEC batch POST (default: 100)
        # batch_size = 100
        
        # If true, drop buffered events on HEC failure (default behavior)
        # If false and retry.enabled=true, retain events for retry
        # drop_on_failure = $true
        
        # Maximum events to buffer before dropping oldest (memory cap)
        # max_buffer_events = 5000
        
        # Maximum buffer size in bytes (accepts "5MB" or numeric)
        # max_buffer_bytes = "5MB"
        
        # Retry settings for failed HEC batches
        # retry = @{
        #     enabled = $false           # Enable retry on HEC failure
        #     max_attempts = 3           # Max retry attempts before giving up
        #     base_delay_ms = 250        # Initial delay between retries
        #     jitter_pct = 20            # Random jitter percentage (0-100)
        #     backoff = "exponential"    # "fixed" or "exponential"
        # }
    }
    
    # ----------------------------------------
    # METRICS (Optional - for Splunk Metrics Index)
    # ----------------------------------------
    # Send numeric data as Splunk metrics for better performance on large datasets.
    # Metrics use the HEC metrics endpoint and support faster aggregation queries.
    # Events remain the default; enable metrics for high-volume or long-term storage.
    #
    metrics = @{
        # Set to $true to enable metrics emission
        enabled = $false
        
        # Metrics output mode:
        #   "dual"         - Emit both events AND metrics (for transition period)
        #   "metrics_only" - Emit metrics only, no summary events (maximum savings)
        mode = "dual"
        
        # Splunk metrics index (must be a metrics-type index)
        # Create in Splunk: Settings > Indexes > New Index > Data Type: Metrics
        index = ""
        
        # HEC URL for metrics endpoint (note: uses /services/collector not /services/collector/event)
        # Format: https://<splunk-server>:<port>/services/collector
        hec_url = ""
        
        # HEC token (can be same as event token if allowed, or separate)
        token = ""
        
        # SSL verification for metrics HEC
        verify_ssl = $true
        
        # SSL protocol for metrics HEC
        ssl_protocol = "Default"
    }
}
