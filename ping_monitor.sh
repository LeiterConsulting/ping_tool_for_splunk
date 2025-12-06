#!/bin/sh
# ============================================================================
# Splunk Ping Monitor - Cross-Platform Edition
# ============================================================================
# A lightweight, portable ping monitoring tool for *nix systems
# Compatible with: Linux, macOS, BSD, Alpine, busybox, iSH (iOS), Raspberry Pi
#
# Version: 1.0.0
# Requires: sh/bash, ping, awk, sed (standard POSIX tools)
# ============================================================================

set -e

# Disable exit-on-error for the main functions (we handle errors explicitly)
set +e

# Default configuration (can be overridden by config file)
CONFIG_FILE="${CONFIG_FILE:-./config.conf}"
ENDPOINTS_FILE="${ENDPOINTS_FILE:-./endpoints.csv}"
PINGS_PER_CYCLE="${PINGS_PER_CYCLE:-4}"
CYCLE_INTERVAL="${CYCLE_INTERVAL:-60}"
PING_TIMEOUT="${PING_TIMEOUT:-2}"
OUTPUT_MODE="${OUTPUT_MODE:-file}"
LOG_PATH="${LOG_PATH:-./logs/ping_results.log}"
LOG_ROTATION_SIZE_MB="${LOG_ROTATION_SIZE_MB:-50}"
HEC_URL="${HEC_URL:-}"
HEC_TOKEN="${HEC_TOKEN:-}"
HEC_INDEX="${HEC_INDEX:-main}"
HEC_SOURCETYPE="${HEC_SOURCETYPE:-ping_monitor}"
HEC_VERIFY_SSL="${HEC_VERIFY_SSL:-true}"
RUN_ONCE="${RUN_ONCE:-false}"
VERBOSE="${VERBOSE:-false}"

# ANSI color codes (disabled if not a terminal)
if [ -t 1 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[0;33m'
    CYAN='\033[0;36m'
    GRAY='\033[0;90m'
    NC='\033[0m' # No Color
else
    RED=''
    GREEN=''
    YELLOW=''
    CYAN=''
    GRAY=''
    NC=''
fi

# ============================================================================
# Helper Functions
# ============================================================================

log_info() {
    printf "${CYAN}%s${NC}\n" "$1"
}

log_success() {
    printf "${GREEN}%s${NC}\n" "$1"
}

log_warning() {
    printf "${YELLOW}WARNING: %s${NC}\n" "$1" >&2
}

log_error() {
    printf "${RED}ERROR: %s${NC}\n" "$1" >&2
}

log_debug() {
    if [ "$VERBOSE" = "true" ]; then
        printf "${GRAY}DEBUG: %s${NC}\n" "$1" >&2
    fi
}

show_help() {
    cat << 'EOF'
Splunk Ping Monitor - Cross-Platform Edition

Usage: ./ping_monitor.sh [OPTIONS]

Options:
  -c, --config FILE      Path to config file (default: ./config.conf)
  -e, --endpoints FILE   Path to endpoints CSV (default: ./endpoints.csv)
  -o, --once             Run single cycle and exit
  -v, --verbose          Enable verbose/debug output
  -h, --help             Show this help message

Environment Variables (override config file):
  PINGS_PER_CYCLE       Number of pings per endpoint (default: 4)
  CYCLE_INTERVAL        Seconds between cycles (default: 60)
  PING_TIMEOUT          Ping timeout in seconds (default: 2)
  OUTPUT_MODE           Output mode: file, hec, both (default: file)
  LOG_PATH              Log file path (default: ./logs/ping_results.log)
  HEC_URL               Splunk HEC URL (for hec/both modes)
  HEC_TOKEN             Splunk HEC token

Examples:
  ./ping_monitor.sh                          # Run with defaults
  ./ping_monitor.sh -o                       # Single cycle
  ./ping_monitor.sh -c /etc/ping/config.conf # Custom config
  PINGS_PER_CYCLE=2 ./ping_monitor.sh        # Override via env

EOF
}

# ============================================================================
# Configuration Loading
# ============================================================================

load_config() {
    config_path="$1"
    
    # Save environment variable overrides (they take precedence)
    _env_PINGS_PER_CYCLE="${PINGS_PER_CYCLE:-}"
    _env_CYCLE_INTERVAL="${CYCLE_INTERVAL:-}"
    _env_PING_TIMEOUT="${PING_TIMEOUT:-}"
    _env_OUTPUT_MODE="${OUTPUT_MODE:-}"
    _env_LOG_PATH="${LOG_PATH:-}"
    _env_LOG_ROTATION_SIZE_MB="${LOG_ROTATION_SIZE_MB:-}"
    _env_HEC_URL="${HEC_URL:-}"
    _env_HEC_TOKEN="${HEC_TOKEN:-}"
    _env_HEC_INDEX="${HEC_INDEX:-}"
    _env_HEC_SOURCETYPE="${HEC_SOURCETYPE:-}"
    _env_HEC_VERIFY_SSL="${HEC_VERIFY_SSL:-}"
    
    if [ -f "$config_path" ]; then
        log_debug "Loading config from: $config_path"
        # Source the config file (shell variable format)
        # shellcheck disable=SC1090
        . "$config_path"
    else
        log_warning "Config file not found: $config_path (using defaults)"
    fi
    
    # Restore environment variable overrides (env vars win over config file)
    [ -n "$_env_PINGS_PER_CYCLE" ] && PINGS_PER_CYCLE="$_env_PINGS_PER_CYCLE"
    [ -n "$_env_CYCLE_INTERVAL" ] && CYCLE_INTERVAL="$_env_CYCLE_INTERVAL"
    [ -n "$_env_PING_TIMEOUT" ] && PING_TIMEOUT="$_env_PING_TIMEOUT"
    [ -n "$_env_OUTPUT_MODE" ] && OUTPUT_MODE="$_env_OUTPUT_MODE"
    [ -n "$_env_LOG_PATH" ] && LOG_PATH="$_env_LOG_PATH"
    [ -n "$_env_LOG_ROTATION_SIZE_MB" ] && LOG_ROTATION_SIZE_MB="$_env_LOG_ROTATION_SIZE_MB"
    [ -n "$_env_HEC_URL" ] && HEC_URL="$_env_HEC_URL"
    [ -n "$_env_HEC_TOKEN" ] && HEC_TOKEN="$_env_HEC_TOKEN"
    [ -n "$_env_HEC_INDEX" ] && HEC_INDEX="$_env_HEC_INDEX"
    [ -n "$_env_HEC_SOURCETYPE" ] && HEC_SOURCETYPE="$_env_HEC_SOURCETYPE"
    [ -n "$_env_HEC_VERIFY_SSL" ] && HEC_VERIFY_SSL="$_env_HEC_VERIFY_SSL"
}

validate_config() {
    # Validate numeric values
    if [ "$PINGS_PER_CYCLE" -lt 1 ] 2>/dev/null; then
        log_warning "Invalid PINGS_PER_CYCLE ($PINGS_PER_CYCLE), using default: 4"
        PINGS_PER_CYCLE=4
    fi
    
    if [ "$CYCLE_INTERVAL" -lt 1 ] 2>/dev/null; then
        log_warning "Invalid CYCLE_INTERVAL ($CYCLE_INTERVAL), using default: 60"
        CYCLE_INTERVAL=60
    fi
    
    if [ "$PING_TIMEOUT" -lt 1 ] 2>/dev/null; then
        log_warning "Invalid PING_TIMEOUT ($PING_TIMEOUT), using default: 2"
        PING_TIMEOUT=2
    fi
    
    # Validate output mode
    case "$OUTPUT_MODE" in
        file|hec|both) ;;
        *)
            log_warning "Invalid OUTPUT_MODE ($OUTPUT_MODE), using default: file"
            OUTPUT_MODE="file"
            ;;
    esac
    
    # Validate HEC config if needed
    if [ "$OUTPUT_MODE" = "hec" ] || [ "$OUTPUT_MODE" = "both" ]; then
        if [ -z "$HEC_URL" ] || [ -z "$HEC_TOKEN" ]; then
            log_error "HEC_URL and HEC_TOKEN required for output mode: $OUTPUT_MODE"
            exit 1
        fi
    fi
}

# ============================================================================
# Endpoint Loading
# ============================================================================

load_endpoints() {
    endpoints_path="$1"
    
    if [ ! -f "$endpoints_path" ]; then
        log_error "Endpoints file not found: $endpoints_path"
        exit 1
    fi
    
    # Read CSV, skip header, handle optional columns
    # Format: ip,hostname[,group][,description]
    ENDPOINTS=""
    line_num=0
    
    while IFS= read -r line || [ -n "$line" ]; do
        line_num=$((line_num + 1))
        
        # Skip header line
        if [ $line_num -eq 1 ]; then
            continue
        fi
        
        # Skip empty lines and comments
        case "$line" in
            ''|\#*) continue ;;
        esac
        
        # Parse CSV (simple parsing - handles basic cases)
        # Remove quotes and parse fields
        clean_line=$(echo "$line" | sed 's/"//g' | tr -d '\r')
        
        ip=$(echo "$clean_line" | cut -d',' -f1 | xargs)
        hostname=$(echo "$clean_line" | cut -d',' -f2 | xargs)
        group=$(echo "$clean_line" | cut -d',' -f3 | xargs)
        description=$(echo "$clean_line" | cut -d',' -f4- | xargs)
        
        # Skip if missing required fields
        if [ -z "$ip" ] || [ -z "$hostname" ]; then
            log_warning "Skipping line $line_num: missing ip or hostname"
            continue
        fi
        
        # Apply defaults for optional fields
        [ -z "$group" ] && group="default"
        [ -z "$description" ] && description=""
        
        # Store as pipe-delimited (safer than comma for internal use)
        if [ -z "$ENDPOINTS" ]; then
            ENDPOINTS="${ip}|${hostname}|${group}|${description}"
        else
            ENDPOINTS="${ENDPOINTS}
${ip}|${hostname}|${group}|${description}"
        fi
    done < "$endpoints_path"
    
    ENDPOINT_COUNT=$(echo "$ENDPOINTS" | grep -c '^' || echo "0")
    log_debug "Loaded $ENDPOINT_COUNT endpoints"
}

# ============================================================================
# Ping Functions (Cross-Platform)
# ============================================================================

detect_ping_style() {
    # Detect which ping syntax to use (BSD/macOS vs Linux/GNU)
    # BSD/macOS: ping -c count -W timeout_ms
    # Linux: ping -c count -W timeout_sec
    # BusyBox: ping -c count -W timeout_sec
    
    if ping -h 2>&1 | grep -q 'W.*timeout.*milliseconds'; then
        PING_STYLE="bsd"
        log_debug "Detected BSD/macOS ping style"
    else
        PING_STYLE="linux"
        log_debug "Detected Linux/GNU ping style"
    fi
}

do_ping() {
    target_ip="$1"
    timeout_sec="$2"
    
    case "$PING_STYLE" in
        bsd)
            # BSD/macOS: -W is in milliseconds
            timeout_ms=$((timeout_sec * 1000))
            ping -c 1 -W "$timeout_ms" "$target_ip" 2>/dev/null
            ;;
        linux|*)
            # Linux/BusyBox: -W is in seconds
            ping -c 1 -W "$timeout_sec" "$target_ip" 2>/dev/null
            ;;
    esac
}

parse_ping_result() {
    ping_output="$1"
    
    # Extract latency from ping output
    # Handles both "time=X.X ms" and "time=X ms" formats
    latency=$(echo "$ping_output" | grep -oE 'time[=<][0-9]+\.?[0-9]*' | head -1 | grep -oE '[0-9]+\.?[0-9]*')
    
    if [ -n "$latency" ]; then
        # Round to integer
        latency=$(printf "%.0f" "$latency" 2>/dev/null || echo "$latency" | cut -d'.' -f1)
        echo "$latency"
    else
        echo "-1"
    fi
}

parse_ttl() {
    ping_output="$1"
    
    # Extract TTL from ping output
    ttl=$(echo "$ping_output" | grep -oiE 'ttl[=:][0-9]+' | head -1 | grep -oE '[0-9]+')
    
    if [ -n "$ttl" ]; then
        echo "$ttl"
    else
        echo "-1"
    fi
}

ping_endpoint() {
    ip="$1"
    hostname="$2"
    group="$3"
    description="$4"
    count="$5"
    
    success_count=0
    total_latency=0
    min_latency=999999
    max_latency=0
    last_ttl=-1
    
    i=0
    while [ $i -lt "$count" ]; do
        i=$((i + 1))
        timestamp=$(date -u +"%Y-%m-%dT%H:%M:%S.000Z")
        
        ping_output=$(do_ping "$ip" "$PING_TIMEOUT" 2>&1) || true
        latency=$(parse_ping_result "$ping_output")
        ttl=$(parse_ttl "$ping_output")
        
        if [ "$latency" != "-1" ] && [ "$latency" -ge 0 ] 2>/dev/null; then
            success_count=$((success_count + 1))
            total_latency=$((total_latency + latency))
            last_ttl=$ttl
            
            if [ "$latency" -lt "$min_latency" ]; then
                min_latency=$latency
            fi
            if [ "$latency" -gt "$max_latency" ]; then
                max_latency=$latency
            fi
            
            log_debug "  Ping $i to $hostname: ${latency}ms (ttl=$ttl)"
        else
            log_debug "  Ping $i to $hostname: FAILED"
        fi
    done
    
    # Calculate statistics
    failed_count=$((count - success_count))
    
    if [ $success_count -gt 0 ]; then
        avg_latency=$((total_latency / success_count))
        packet_loss=$(awk "BEGIN {printf \"%.1f\", ($failed_count / $count) * 100}")
    else
        avg_latency=-1
        min_latency=-1
        max_latency=-1
        packet_loss="100.0"
    fi
    
    # Generate summary JSON
    summary_timestamp=$(date -u +"%Y-%m-%dT%H:%M:%S.000Z")
    
    # Escape description for JSON
    desc_escaped=$(printf '%s' "$description" | sed 's/"/\\"/g')
    
    # Output JSON (using printf to avoid heredoc line ending issues)
    printf '{"timestamp":"%s","target_ip":"%s","hostname":"%s","group":"%s","description":"%s","record_type":"summary","pings_sent":%d,"pings_successful":%d,"pings_failed":%d,"packet_loss_pct":%s,"avg_latency_ms":%d,"min_latency_ms":%d,"max_latency_ms":%d}\n' \
        "$summary_timestamp" "$ip" "$hostname" "$group" "$desc_escaped" \
        "$count" "$success_count" "$failed_count" "$packet_loss" \
        "$avg_latency" "$min_latency" "$max_latency"
    
    # Always return success - the JSON output contains the status
    return 0
}

# ============================================================================
# Output Functions
# ============================================================================

ensure_log_dir() {
    log_dir=$(dirname "$LOG_PATH")
    if [ ! -d "$log_dir" ]; then
        mkdir -p "$log_dir"
        log_debug "Created log directory: $log_dir"
    fi
}

rotate_log() {
    if [ ! -f "$LOG_PATH" ]; then
        return
    fi
    
    # Get file size in MB (portable method)
    if command -v stat >/dev/null 2>&1; then
        # Try GNU stat first, then BSD stat
        size_bytes=$(stat -c %s "$LOG_PATH" 2>/dev/null || stat -f %z "$LOG_PATH" 2>/dev/null || echo "0")
    else
        # Fallback using ls
        size_bytes=$(ls -l "$LOG_PATH" | awk '{print $5}')
    fi
    
    size_mb=$((size_bytes / 1048576))
    
    if [ "$size_mb" -ge "$LOG_ROTATION_SIZE_MB" ]; then
        timestamp=$(date +"%Y%m%d_%H%M%S")
        archive_path="${LOG_PATH%.log}_${timestamp}.log"
        
        log_warning "Rotating log file (Size: ${size_mb}MB)"
        mv "$LOG_PATH" "$archive_path"
        
        # Keep only last 5 rotated logs
        log_dir=$(dirname "$LOG_PATH")
        log_base=$(basename "$LOG_PATH" .log)
        
        # shellcheck disable=SC2012
        ls -t "$log_dir/${log_base}_"*.log 2>/dev/null | tail -n +6 | while read -r old_log; do
            rm -f "$old_log"
            log_debug "Removed old log: $old_log"
        done
    fi
}

write_to_log() {
    json_line="$1"
    echo "$json_line" >> "$LOG_PATH"
}

send_to_hec() {
    json_data="$1"
    
    # Build HEC event payload
    hec_payload="{\"index\":\"$HEC_INDEX\",\"sourcetype\":\"$HEC_SOURCETYPE\",\"event\":$json_data}"
    
    # Determine curl SSL options
    if [ "$HEC_VERIFY_SSL" = "false" ]; then
        ssl_opt="-k"
    else
        ssl_opt=""
    fi
    
    # Send to HEC
    # shellcheck disable=SC2086
    response=$(curl -s -w "\n%{http_code}" $ssl_opt \
        -X POST "$HEC_URL" \
        -H "Authorization: Splunk $HEC_TOKEN" \
        -H "Content-Type: application/json" \
        -d "$hec_payload" 2>/dev/null) || true
    
    http_code=$(echo "$response" | tail -1)
    
    if [ "$http_code" = "200" ]; then
        log_debug "HEC send successful"
        return 0
    else
        log_warning "HEC send failed (HTTP $http_code)"
        return 1
    fi
}

# ============================================================================
# Main Execution
# ============================================================================

parse_args() {
    while [ $# -gt 0 ]; do
        case "$1" in
            -c|--config)
                CONFIG_FILE="$2"
                shift 2
                ;;
            -e|--endpoints)
                ENDPOINTS_FILE="$2"
                shift 2
                ;;
            -o|--once)
                RUN_ONCE="true"
                shift
                ;;
            -v|--verbose)
                VERBOSE="true"
                shift
                ;;
            -h|--help)
                show_help
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done
}

main() {
    parse_args "$@"
    
    # Load and validate configuration
    load_config "$CONFIG_FILE"
    validate_config
    
    # Load endpoints
    load_endpoints "$ENDPOINTS_FILE"
    
    if [ -z "$ENDPOINTS" ]; then
        log_error "No valid endpoints found in: $ENDPOINTS_FILE"
        exit 1
    fi
    
    # Detect ping style for this platform
    detect_ping_style
    
    # Ensure log directory exists
    if [ "$OUTPUT_MODE" = "file" ] || [ "$OUTPUT_MODE" = "both" ]; then
        ensure_log_dir
    fi
    
    # Display startup info
    printf "${CYAN}========================================${NC}\n"
    printf "${CYAN}  Splunk Ping Monitor (Unix Edition)${NC}\n"
    printf "${CYAN}========================================${NC}\n"
    printf "Endpoints: %s\n" "$ENDPOINT_COUNT"
    printf "Pings per cycle: %s\n" "$PINGS_PER_CYCLE"
    printf "Cycle interval: %s seconds\n" "$CYCLE_INTERVAL"
    printf "Output mode: %s\n" "$OUTPUT_MODE"
    printf "${GRAY}----------------------------------------${NC}\n"
    
    cycle_count=0
    
    while true; do
        cycle_count=$((cycle_count + 1))
        cycle_time=$(date +"%Y-%m-%d %H:%M:%S")
        
        printf "\n[%s] Starting cycle #%d...\n" "$cycle_time" "$cycle_count"
        
        success_total=0
        partial_total=0
        failed_total=0
        
        # Check for log rotation
        if [ "$OUTPUT_MODE" = "file" ] || [ "$OUTPUT_MODE" = "both" ]; then
            rotate_log
        fi
        
        # Process each endpoint
        echo "$ENDPOINTS" | while IFS='|' read -r ip hostname group description; do
            [ -z "$ip" ] && continue
            
            log_debug "Pinging $hostname ($ip)..."
            
            # Ping and capture result
            result=$(ping_endpoint "$ip" "$hostname" "$group" "$description" "$PINGS_PER_CYCLE" 2>&1)
            
            # Output based on mode
            if [ -n "$result" ]; then
                case "$OUTPUT_MODE" in
                    file)
                        write_to_log "$result"
                        ;;
                    hec)
                        send_to_hec "$result" || true
                        ;;
                    both)
                        write_to_log "$result"
                        send_to_hec "$result" || true
                        ;;
                esac
            fi
        done
        
        # Simple success message
        if [ "$OUTPUT_MODE" = "file" ] || [ "$OUTPUT_MODE" = "both" ]; then
            log_success "Results written to: $LOG_PATH"
        fi
        
        printf "Cycle #%d complete\n" "$cycle_count"
        
        # Exit if run-once mode
        if [ "$RUN_ONCE" = "true" ]; then
            printf "\n${GREEN}Ping Monitor completed.${NC}\n"
            exit 0
        fi
        
        # Wait for next cycle
        log_debug "Sleeping for $CYCLE_INTERVAL seconds..."
        sleep "$CYCLE_INTERVAL"
    done
}

# Run main function
main "$@"
