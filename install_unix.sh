#!/bin/sh
# ============================================================================
# Splunk Ping Monitor - Installation Script (Unix Edition)
# ============================================================================
# Quick installer for various *nix systems
# Supports: Linux, macOS, BSD, Alpine, Raspberry Pi OS, iSH (iOS)
# ============================================================================

set -e

# Default installation directory
INSTALL_DIR="${INSTALL_DIR:-/opt/ping_monitor}"
SERVICE_USER="${SERVICE_USER:-ping_monitor}"
CREATE_SERVICE="${CREATE_SERVICE:-true}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m'

log_info() { printf "${CYAN}[INFO]${NC} %s\n" "$1"; }
log_success() { printf "${GREEN}[OK]${NC} %s\n" "$1"; }
log_warning() { printf "${YELLOW}[WARN]${NC} %s\n" "$1"; }
log_error() { printf "${RED}[ERROR]${NC} %s\n" "$1"; exit 1; }

# Detect OS/Distribution
detect_os() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS_NAME="$ID"
        OS_VERSION="$VERSION_ID"
    elif [ "$(uname)" = "Darwin" ]; then
        OS_NAME="macos"
        OS_VERSION="$(sw_vers -productVersion 2>/dev/null || echo 'unknown')"
    elif [ -f /etc/alpine-release ]; then
        OS_NAME="alpine"
        OS_VERSION="$(cat /etc/alpine-release)"
    elif command -v apk >/dev/null 2>&1; then
        # iSH uses Alpine's apk
        OS_NAME="ish"
        OS_VERSION="alpine-based"
    else
        OS_NAME="$(uname -s | tr '[:upper:]' '[:lower:]')"
        OS_VERSION="unknown"
    fi
    
    log_info "Detected OS: $OS_NAME ($OS_VERSION)"
}

# Check dependencies
check_dependencies() {
    log_info "Checking dependencies..."
    
    missing=""
    
    # Required: ping
    if ! command -v ping >/dev/null 2>&1; then
        missing="$missing ping"
    fi
    
    # Required: awk
    if ! command -v awk >/dev/null 2>&1; then
        missing="$missing awk"
    fi
    
    # Required: sed
    if ! command -v sed >/dev/null 2>&1; then
        missing="$missing sed"
    fi
    
    # Optional but recommended: curl (for HEC)
    if ! command -v curl >/dev/null 2>&1; then
        log_warning "curl not found - HEC output will not work"
    fi
    
    if [ -n "$missing" ]; then
        log_error "Missing required dependencies:$missing"
    fi
    
    log_success "All required dependencies found"
}

# Install missing dependencies based on OS
install_dependencies() {
    log_info "Installing dependencies for $OS_NAME..."
    
    case "$OS_NAME" in
        ubuntu|debian|raspbian)
            apt-get update
            apt-get install -y iputils-ping curl
            ;;
        alpine|ish)
            apk add --no-cache iputils curl
            ;;
        centos|rhel|fedora|rocky|alma)
            if command -v dnf >/dev/null 2>&1; then
                dnf install -y iputils curl
            else
                yum install -y iputils curl
            fi
            ;;
        macos)
            # macOS has ping built-in, curl usually available
            if ! command -v curl >/dev/null 2>&1; then
                log_warning "Install curl via: brew install curl"
            fi
            ;;
        freebsd)
            pkg install -y curl
            ;;
        openbsd)
            pkg_add curl
            ;;
        *)
            log_warning "Unknown OS - please install 'ping' and 'curl' manually"
            ;;
    esac
}

# Create installation directory and copy files
install_files() {
    log_info "Installing to $INSTALL_DIR..."
    
    # Create directory
    mkdir -p "$INSTALL_DIR"
    mkdir -p "$INSTALL_DIR/logs"
    
    # Copy files
    cp ping_monitor.sh "$INSTALL_DIR/"
    cp config.conf "$INSTALL_DIR/"
    cp endpoints.csv "$INSTALL_DIR/" 2>/dev/null || cp endpoints_unix.csv "$INSTALL_DIR/endpoints.csv"
    
    # Make executable
    chmod +x "$INSTALL_DIR/ping_monitor.sh"
    
    log_success "Files installed to $INSTALL_DIR"
}

# Create systemd service (Linux)
create_systemd_service() {
    log_info "Creating systemd service..."
    
    cat > /etc/systemd/system/ping_monitor.service << EOF
[Unit]
Description=Splunk Ping Monitor
After=network.target

[Service]
Type=simple
User=$SERVICE_USER
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/ping_monitor.sh
Restart=always
RestartSec=10

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$INSTALL_DIR/logs

[Install]
WantedBy=multi-user.target
EOF
    
    # Create service user if needed
    if ! id "$SERVICE_USER" >/dev/null 2>&1; then
        useradd -r -s /bin/false "$SERVICE_USER" || true
    fi
    
    # Set permissions
    chown -R "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR"
    
    # Reload and enable
    systemctl daemon-reload
    systemctl enable ping_monitor
    
    log_success "Systemd service created"
    log_info "Start with: systemctl start ping_monitor"
    log_info "View logs with: journalctl -u ping_monitor -f"
}

# Create launchd service (macOS)
create_launchd_service() {
    log_info "Creating launchd service..."
    
    plist_path="/Library/LaunchDaemons/com.splunk.ping_monitor.plist"
    
    cat > "$plist_path" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.splunk.ping_monitor</string>
    <key>ProgramArguments</key>
    <array>
        <string>$INSTALL_DIR/ping_monitor.sh</string>
    </array>
    <key>WorkingDirectory</key>
    <string>$INSTALL_DIR</string>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>$INSTALL_DIR/logs/stdout.log</string>
    <key>StandardErrorPath</key>
    <string>$INSTALL_DIR/logs/stderr.log</string>
</dict>
</plist>
EOF
    
    log_success "Launchd service created"
    log_info "Load with: sudo launchctl load $plist_path"
    log_info "Unload with: sudo launchctl unload $plist_path"
}

# Create OpenRC service (Alpine)
create_openrc_service() {
    log_info "Creating OpenRC service..."
    
    cat > /etc/init.d/ping_monitor << 'EOF'
#!/sbin/openrc-run

name="ping_monitor"
description="Splunk Ping Monitor"
command="/opt/ping_monitor/ping_monitor.sh"
command_background=true
pidfile="/run/${RC_SVCNAME}.pid"
directory="/opt/ping_monitor"

depend() {
    need net
    after firewall
}
EOF
    
    chmod +x /etc/init.d/ping_monitor
    rc-update add ping_monitor default
    
    log_success "OpenRC service created"
    log_info "Start with: rc-service ping_monitor start"
}

# Create simple cron-based runner (for iSH and minimal systems)
create_cron_runner() {
    log_info "Creating cron-based runner..."
    
    # Create a wrapper that runs once per minute
    cat > "$INSTALL_DIR/run_once.sh" << 'EOF'
#!/bin/sh
cd /opt/ping_monitor
./ping_monitor.sh --once >> /opt/ping_monitor/logs/cron.log 2>&1
EOF
    chmod +x "$INSTALL_DIR/run_once.sh"
    
    # Add to crontab
    (crontab -l 2>/dev/null || true; echo "* * * * * $INSTALL_DIR/run_once.sh") | crontab -
    
    log_success "Cron job created (runs every minute)"
    log_info "View logs: tail -f $INSTALL_DIR/logs/cron.log"
    log_info "Remove with: crontab -e (and delete the line)"
}

# Main installation
main() {
    echo ""
    printf "${CYAN}========================================${NC}\n"
    printf "${CYAN}  Ping Monitor Installer (Unix)${NC}\n"
    printf "${CYAN}========================================${NC}\n"
    echo ""
    
    # Check if running as root (needed for service installation)
    if [ "$(id -u)" -ne 0 ] && [ "$CREATE_SERVICE" = "true" ]; then
        log_warning "Not running as root - service installation will be skipped"
        CREATE_SERVICE="false"
    fi
    
    # Detect OS
    detect_os
    
    # Check dependencies
    check_dependencies
    
    # Install files
    if [ "$(id -u)" -eq 0 ]; then
        install_files
    else
        log_warning "Not root - installing to current directory instead"
        INSTALL_DIR="$(pwd)/ping_monitor"
        mkdir -p "$INSTALL_DIR/logs"
        cp ping_monitor.sh config.conf "$INSTALL_DIR/"
        cp endpoints.csv "$INSTALL_DIR/" 2>/dev/null || cp endpoints_unix.csv "$INSTALL_DIR/endpoints.csv" 2>/dev/null || true
        chmod +x "$INSTALL_DIR/ping_monitor.sh"
    fi
    
    # Create service based on OS
    if [ "$CREATE_SERVICE" = "true" ]; then
        case "$OS_NAME" in
            ubuntu|debian|raspbian|centos|rhel|fedora|rocky|alma)
                if command -v systemctl >/dev/null 2>&1; then
                    create_systemd_service
                else
                    create_cron_runner
                fi
                ;;
            macos)
                create_launchd_service
                ;;
            alpine)
                if command -v rc-service >/dev/null 2>&1; then
                    create_openrc_service
                else
                    create_cron_runner
                fi
                ;;
            ish|*)
                create_cron_runner
                ;;
        esac
    fi
    
    echo ""
    printf "${GREEN}========================================${NC}\n"
    printf "${GREEN}  Installation Complete!${NC}\n"
    printf "${GREEN}========================================${NC}\n"
    echo ""
    log_info "Installation directory: $INSTALL_DIR"
    log_info "Configuration: $INSTALL_DIR/config.conf"
    log_info "Endpoints: $INSTALL_DIR/endpoints.csv"
    echo ""
    log_info "Quick test: cd $INSTALL_DIR && ./ping_monitor.sh --once"
    echo ""
}

main "$@"
