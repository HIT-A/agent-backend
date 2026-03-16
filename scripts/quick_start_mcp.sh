#!/bin/bash

# Quick Start Script for MCP Integration

set -e

echo "====================================="
echo "  MCP Integration Quick Start"
echo "====================================="
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Functions
print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_info() {
    echo -e "${NC}ℹ${NC} $1"
}

# Step 1: Check prerequisites
echo "Step 1: Checking prerequisites..."

if ! command -v docker &> /dev/null; then
    print_error "Docker is not installed. Please install Docker first."
    exit 1
fi
print_success "Docker is installed"

if ! command -v npx &> /dev/null; then
    print_error "npx is not installed. Please install Node.js and npm first."
    exit 1
fi
print_success "npx is available"

# Step 2: Create HITA_Project directory
echo ""
echo "Step 2: Creating HITA_Project directory..."

mkdir -p ./HITA_Project
print_success "Created ./HITA_Project directory"

# Step 3: Set up environment variables
echo ""
echo "Step 3: Setting up environment variables..."

export MCP_SERVERS=$(cat config/mcp_servers_recommended.json)
print_success "MCP_SERVERS configured"

if [ -z "$GITHUB_TOKEN" ]; then
    print_warning "GITHUB_TOKEN not set. GitHub MCP will have read-only access."
    print_info "Set it with: export GITHUB_TOKEN=\"your-token\""
else
    print_success "GITHUB_TOKEN is set"
fi

if [ -z "$BRAVE_API_KEY" ]; then
    print_warning "BRAVE_API_KEY not set. Brave Search MCP will be disabled."
    print_info "Get API key from: https://api.search.brave.com/app/dashboard"
    print_info "Set it with: export BRAVE_API_KEY=\"your-api-key\""
else
    print_success "BRAVE_API_KEY is set"
fi

# Step 4: Start services with Docker Compose
echo ""
echo "Step 4: Starting services..."

if [ "$1" == "--dev" ]; then
    echo "Development mode: Starting only agent-backend..."
    go run cmd/server/main.go &
    AGENT_PID=$!
    echo "Agent backend started with PID: $AGENT_PID"
else
    echo "Production mode: Starting all services with Docker Compose..."
    docker-compose -f docker-compose.mcp.yml up -d
    print_success "Services started"
fi

# Step 5: Wait for services to be ready
echo ""
echo "Step 5: Waiting for services to be ready..."

echo "Waiting for agent-backend (http://localhost:8080)..."
sleep 3

if [ "$1" == "--dev" ]; then
    echo "Agent backend should be ready now."
else
    echo "Checking services status..."
    docker-compose -f docker-compose.mcp.yml ps
fi

# Step 6: Test MCP skills
echo ""
echo "Step 6: Testing MCP skills..."

echo "Test 1: List registered MCP servers..."
curl -s -X POST http://localhost:8080/v1/skills/mcp.list_servers:invoke \
    -H "Content-Type: application/json" \
    -d '{"input": {}}' | jq '.' || echo "Failed (is jq installed?)"

echo ""
echo "Test 2: Register filesystem MCP..."
curl -s -X POST http://localhost:8080/v1/skills/mcp.register_server:invoke \
    -H "Content-Type: application/json" \
    -d '{"input":{"name":"filesystem","transport":"stdio","command":["npx","-y","@modelcontextprotocol/server-filesystem","./HITA_Project"]}}' | jq '.' || echo "Failed"

echo ""
echo "Test 3: Register GitHub MCP..."
curl -s -X POST http://localhost:8080/v1/skills/mcp.register_server:invoke \
    -H "Content-Type: application/json" \
    -d '{"input":{"name":"github","transport":"stdio","command":["npx","-y","@modelcontextprotocol/server-github"]}}' | jq '.' || echo "Failed"

# Step 7: Display summary
echo ""
echo "====================================="
echo "  MCP Integration Ready!"
echo "====================================="
echo ""
echo "Available MCP Skills:"
echo "  - mcp.list_servers"
echo "  - mcp.register_server"
echo "  - mcp.unregister_server"
echo "  - mcp.list_tools"
echo "  - mcp.call_tool"
echo ""
echo "Example Usage:"
echo ""
echo "1. List all tools:"
echo "   curl -X POST http://localhost:8080/v1/skills/mcp.list_tools:invoke \\"
echo "     -H 'Content-Type: application/json' \\"
echo "     -d '{\"input\": {}}'"
echo ""
echo "2. Call a tool:"
echo "   curl -X POST http://localhost:8080/v1/skills/mcp.call_tool:invoke \\"
echo "     -H 'Content-Type: application/json' \\"
echo "     -d '{\"input\":{\"server\":\"filesystem\",\"tool\":\"read_file\",\"arguments\":{\"path\":\"/data/test.txt\"}}}'"
echo ""
echo "For more information, see:"
echo "  - docs/MCP_SERVERS_INTEGRATION.md"
echo "  - docs/MCP_USAGE.md"
echo "  - docs/MCP_README.md"
echo ""

# Stop services on exit
trap cleanup EXIT

cleanup() {
    echo ""
    if [ "$1" == "--dev" ]; then
        echo "Stopping agent backend..."
        if [ ! -z "$AGENT_PID" ]; then
            kill $AGENT_PID 2>/dev/null || true
        fi
    else
        echo "Stopping Docker services..."
        docker-compose -f docker-compose.mcp.yml down
    fi
    print_success "Services stopped"
}

echo "Press Ctrl+C to stop services..."

# Keep script running
wait
