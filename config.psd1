# ============================================
# Splunk Ping Monitor Configuration (Go v6.0.0)
# ============================================
# Legacy-compatible configuration example. New deployments use config.json.
# Relative paths are resolved from the directory containing this file.
# Supported fallback formats: config.yaml, config.json.
# ============================================

@{
    config_schema_version = 2

    # ----------------------------------------
    # CORE CYCLE SETTINGS
    # ----------------------------------------
    pings_per_cycle = 4
    cycle_interval_seconds = 60
    timeout_ms = 1000
    parallel_threads = 10

    # ----------------------------------------
    # EVENT SETTINGS
    # ----------------------------------------
    # Summary events are always emitted.
    # Set to $false to skip per-ping events and reduce event volume.
    emit_individual_pings = $true

    # ----------------------------------------
    # OUTPUT SETTINGS
    # ----------------------------------------
    # file | hec | both
    output_mode = "file"
    log_path = "./logs/ping_results.log"
    log_rotation_size_mb = 50
    log_retention_files = 10
    log_retention_days = 14
    log_compress_rotated = $true

    # ----------------------------------------
    # PING ENGINE
    # ----------------------------------------
    ping = @{
        # auto = native ICMP with exec fallback
        # raw  = native/raw ICMP only
        # exec = OS ping only
        mode = "auto"
    }

	# Hysteresis prevents a single missed reply from flipping a device down.
	health = @{
		down_after_failures = 3
		recovery_after_successes = 2
		stale_after_intervals = 2
	}

    # ----------------------------------------
    # DIAGNOSTICS
    # ----------------------------------------
    diagnostics = @{
        enabled = $false
        handle_probe_mode = "none"   # none | hec_only | metrics_only
    }

    debug = @{
        emit_memory_stats = $false
    }

    # ----------------------------------------
    # SPLUNK HEC (EVENTS)
    # ----------------------------------------
    hec = @{
        enabled = $false
        url = ""
        token = ""
        index = "main"
        sourcetype = "ping_monitor"
        verify_ssl = $true
        ssl_protocol = "Default"     # Default | Tls12 | Tls13 | Tls11 | Tls

        batch_size = 100
        # Deprecated compatibility setting. v5.5+ never drops failed cycles;
        # completed cycles remain in the durable outbox until delivered.
        drop_on_failure = $false
        max_buffer_events = 5000
        max_buffer_bytes = "5MB"

        retry = @{
            enabled = $false
            max_attempts = 3
            base_delay_ms = 250
            jitter_pct = 20
            backoff = "exponential"  # exponential | fixed
        }

        # Simplified compatibility knobs for older configs
        retry_count = 0
        retry_delay_ms = 250

        # Optional dead-letter file for dropped batches
        dead_letter_path = ""
        dead_letter_rotation_size_mb = 0

        # A normal HTTP 2xx confirms HEC acceptance. Enable indexer ACK when
        # delivery health must mean that Splunk has indexed the batch.
        use_ack = $false
        ack_timeout_seconds = 60
        ack_poll_interval_ms = 1000
        channel = ""                 # blank = stable GUID derived from collector_id
    }

    # ----------------------------------------
    # SPLUNK METRICS
    # ----------------------------------------
    metrics = @{
        enabled = $false
        mode = "dual"                # dual | metrics_only
        index = ""
        hec_url = ""
        token = ""
        verify_ssl = $true
        ssl_protocol = "Default"

        compat_mode = $true
        sourcetype = "ping_monitor:metrics"
        event_name = "metric"
        use_metrics_index = $false

        batch_size = 100
        max_buffer_events = 5000
        max_buffer_bytes = "5MB"
        use_ack = $false
        ack_timeout_seconds = 60
        ack_poll_interval_ms = 1000
        channel = ""
    }

    # ----------------------------------------
    # DURABLE NETWORK DELIVERY
    # ----------------------------------------
    delivery = @{
        spool_path = "./data/outbox"
        max_spool_bytes = "512MB"
        max_envelopes = 10000
        drain_max_envelopes = 100
    }

    # ----------------------------------------
    # DISCOVERY HISTORY AND SCHEDULING
    # ----------------------------------------
    discovery = @{
        history_path = "./data/discovery"
        # 0 disables the scan-count limit. The age limit keeps history bounded.
        retention_scans = 0
        retention_days = 365
        # Optional subnet catalog. Existing CIDR-only schedules remain valid.
        subnets = @(
            # @{
            #     id = "headquarters-users"
            #     cidr = "10.20.30.0/24"
            #     name = "Headquarters User LAN"
            #     vlan = "230"
            #     location = "Main Office"
            #     addressing_mode = "dhcp"  # static | dhcp
            #     routing_domain = "corp"
            # }
        )
        # Weekly schedules remain review-only: scan results are never
        # silently added to the monitored inventory.
        schedules = @(
            # @{
            #     id = "weekly-core"
            #     enabled = $true
            #     targets = @("10.20.0.0/24")
            #     frequency = "weekly"
            #     day = "Sunday"
            #     time = "02:00"
            #     timezone = "America/New_York"
            #     timeout_ms = 500
            #     concurrency = 50
            #     import_policy = "review"
            # }
        )
    }

    # ----------------------------------------
    # NAMING CONVENTION CLASSIFICATION
    # ----------------------------------------
    # Ordered RE2 regex match/assignment pairs. Named captures such as
    # (?P<site>...) can be referenced in assignments as ${site}.
    classification = @{
        rules = @(
            # @{
            #     id = "site-network-device"
            #     enabled = $true
            #     source = "hostname"  # hostname | fqdn | either
            #     pattern = '^(?P<site>[a-z]{3})-(?P<role>sw|fw)-(?P<vendor>[a-z]+)-\d+$'
            #     assignments = @{
            #         group = '${site}'
            #         entitytype = "network"
            #         device = '${role}'
            #         vendor = '${vendor}'
            #     }
            #     overwrite = $false
            #     stop_on_match = $false
            # }
        )
    }
}
