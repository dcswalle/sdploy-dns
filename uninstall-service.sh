#!/bin/bash

# Uninstallation script for go-dns systemd service

set -e

SERVICE_NAME="go-dns"
BINARY_NAME="go-dns-server"
INSTALL_DIR="/usr/local/bin"
SERVICE_DIR="/etc/systemd/system"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}Uninstalling Go DNS Server systemd service...${NC}"

# Check if running as root
if [ "$EUID" -ne 0 ]; then 
    echo -e "${RED}Please run as root (use sudo)${NC}"
    exit 1
fi

# Stop and disable service
if systemctl is-active --quiet "${SERVICE_NAME}"; then
    echo -e "${GREEN}Stopping ${SERVICE_NAME} service...${NC}"
    systemctl stop "${SERVICE_NAME}"
fi

if systemctl is-enabled --quiet "${SERVICE_NAME}"; then
    echo -e "${GREEN}Disabling ${SERVICE_NAME} service...${NC}"
    systemctl disable "${SERVICE_NAME}"
fi

# Remove service file
if [ -f "${SERVICE_DIR}/${SERVICE_NAME}.service" ]; then
    echo -e "${GREEN}Removing service file...${NC}"
    rm -f "${SERVICE_DIR}/${SERVICE_NAME}.service"
    systemctl daemon-reload
fi

# Remove binary
if [ -f "${INSTALL_DIR}/${BINARY_NAME}" ]; then
    echo -e "${GREEN}Removing binary...${NC}"
    rm -f "${INSTALL_DIR}/${BINARY_NAME}"
fi

echo -e "${GREEN}Uninstallation complete!${NC}"
echo -e "${YELLOW}Note: Config files in /etc/go-dns were not removed${NC}"
