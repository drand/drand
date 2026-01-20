#!/bin/bash

# Install script for drand systemd service
# This script installs the drand systemd service file and creates necessary directories

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Determine privilege helper
if [[ $EUID -eq 0 ]]; then
    SUDO=""
else
    SUDO="sudo"
fi

# Check if systemd is available
if ! command -v systemctl &> /dev/null; then
    echo -e "${RED}systemd is not available on this system${NC}"
    exit 1
fi

# Check if drand binary exists
if ! command -v drand &> /dev/null; then
    echo -e "${YELLOW}Warning: drand binary not found in PATH${NC}"
    echo "Please ensure drand is installed and available in your PATH"
fi

# Create drand user if it doesn't exist
if ! id "drand" &>/dev/null; then
    echo -e "${GREEN}Creating drand user...${NC}"
    ${SUDO} useradd --system --home /var/lib/drand --shell /bin/false drand
fi

# Create drand directories
echo -e "${GREEN}Creating drand directories...${NC}"
${SUDO} mkdir -p /var/lib/drand
${SUDO} mkdir -p /etc/drand
${SUDO} chown drand:drand /var/lib/drand
${SUDO} chmod 755 /var/lib/drand

# Install systemd service file
echo -e "${GREEN}Installing systemd service file...${NC}"
${SUDO} cp contrib/systemd/drand.service /etc/systemd/system/
${SUDO} chmod 644 /etc/systemd/system/drand.service

# Reload systemd
echo -e "${GREEN}Reloading systemd daemon...${NC}"
${SUDO} systemctl daemon-reload

# Enable the service (but don't start it yet)
echo -e "${GREEN}Enabling drand service...${NC}"
${SUDO} systemctl enable drand

echo -e "${GREEN}Installation complete!${NC}"
echo ""
echo "To start the service:"
echo "  sudo systemctl start drand"
echo ""
echo "To check status:"
echo "  sudo systemctl status drand"
echo ""
echo "To view logs:"
echo "  sudo journalctl -u drand -f"
echo ""
echo "To stop the service:"
echo "  sudo systemctl stop drand"
echo ""
echo "Note: You may need to configure the service file with your specific"
echo "drand configuration before starting the service."
