#!/bin/bash

# Installation script for go-dns systemd service
# This script installs the DNS server as a systemd service

set -e

SERVICE_NAME="go-dns"
SERVICE_FILE="${SERVICE_NAME}.service"
BINARY_NAME="go-dns-server"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/go-dns"
SERVICE_DIR="/etc/systemd/system"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Installing Go DNS Server as systemd service...${NC}"

# Check if running as root
if [ "$EUID" -ne 0 ]; then 
    echo -e "${RED}Please run as root (use sudo)${NC}"
    exit 1
fi

# Check if binary exists
if [ ! -f "./${BINARY_NAME}" ]; then
    echo -e "${YELLOW}Binary not found. Building...${NC}"
    go build -o ${BINARY_NAME} main.go
    if [ $? -ne 0 ]; then
        echo -e "${RED}Build failed!${NC}"
        exit 1
    fi
fi

# Create config directory
echo -e "${GREEN}Creating config directory: ${CONFIG_DIR}${NC}"
mkdir -p ${CONFIG_DIR}

# Copy binary
echo -e "${GREEN}Installing binary to ${INSTALL_DIR}${NC}"
cp ${BINARY_NAME} ${INSTALL_DIR}/${BINARY_NAME}
chmod +x ${INSTALL_DIR}/${BINARY_NAME}

# Copy config file if it doesn't exist
if [ ! -f "${CONFIG_DIR}/config.yml" ]; then
    echo -e "${GREEN}Copying config file to ${CONFIG_DIR}${NC}"
    cp config.yml ${CONFIG_DIR}/config.yml
    echo -e "${YELLOW}Please edit ${CONFIG_DIR}/config.yml to configure the server${NC}"
    echo -e "${YELLOW}Note: Update file paths in config.yml to use absolute paths or paths relative to ${CONFIG_DIR}${NC}"
else
    echo -e "${YELLOW}Config file already exists at ${CONFIG_DIR}/config.yml${NC}"
fi

# Copy hosts.txt if it exists and doesn't exist in config dir
if [ -f "hosts.txt" ] && [ ! -f "${CONFIG_DIR}/hosts.txt" ]; then
    echo -e "${GREEN}Copying hosts.txt to ${CONFIG_DIR}${NC}"
    cp hosts.txt ${CONFIG_DIR}/hosts.txt
fi

# Copy service file
echo -e "${GREEN}Installing systemd service file${NC}"
cp ${SERVICE_FILE} ${SERVICE_DIR}/${SERVICE_FILE}

# Reload systemd
echo -e "${GREEN}Reloading systemd daemon...${NC}"
systemctl daemon-reload

# Enable service
echo -e "${GREEN}Enabling ${SERVICE_NAME} service...${NC}"
systemctl enable ${SERVICE_NAME}.service

echo -e "${GREEN}Installation complete!${NC}"
echo ""
echo -e "To start the service: ${YELLOW}sudo systemctl start ${SERVICE_NAME}${NC}"
echo -e "To check status: ${YELLOW}sudo systemctl status ${SERVICE_NAME}${NC}"
echo -e "To view logs: ${YELLOW}sudo journalctl -u ${SERVICE_NAME} -f${NC}"
echo -e "To stop the service: ${YELLOW}sudo systemctl stop ${SERVICE_NAME}${NC}"
