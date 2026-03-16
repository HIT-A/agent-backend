#!/bin/bash
# Complete RAG Pipeline Test
# This script starts the server and runs the full pipeline test

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

echo "=== RAG Pipeline Complete Test ==="
echo ""

# Check for required environment variables
check_env() {
    local missing=()
    
    # Qdrant (optional, will use mock if not set)
    if [ -z "$QDRANT_URL" ]; then
        echo "⚠️  QDRANT_URL not set, using mock"
    fi
    
    # BigModel (required for embeddings)
    if [ -z "$BIGMODEL_API_KEY" ]; then
        missing+=("BIGMODEL_API_KEY")
    fi
    
    # GitHub (required for fetching)
    if [ -z "$GITHUB_TOKEN" ]; then
        echo "⚠️  GITHUB_TOKEN not set, may hit rate limits"
    fi
    
    if [ ${#missing[@]} -gt 0 ]; then
        echo "❌ Missing required environment variables:"
        for var in "${missing[@]}"; do
            echo "   - $var"
        done
        echo ""
        echo "Set them with:"
        echo "  export BIGMODEL_API_KEY=your-key"
        echo "  export GITHUB_TOKEN=your-token  # optional but recommended"
        exit 1
    fi
}

# Start server in background
start_server() {
    echo "📦 Starting agent-backend server..."
    
    cd "$PROJECT_DIR"
    
    # Build if needed
    if [ ! -f "./server" ]; then
        echo "  Building server..."
        go build -o server ./cmd/server
    fi
    
    # Start server
    ./server &
    SERVER_PID=$!
    echo "  Server PID: $SERVER_PID"
    
    # Wait for server to be ready
    echo "  Waiting for server to start..."
    for i in {1..30}; do
        if curl -s http://localhost:8080/health > /dev/null 2>&1; then
            echo "  ✅ Server is ready"
            return 0
        fi
        sleep 1
    done
    
    echo "  ❌ Server failed to start"
    kill $SERVER_PID 2>/dev/null || true
    exit 1
}

# Stop server
stop_server() {
    if [ -n "$SERVER_PID" ]; then
        echo ""
        echo "🛑 Stopping server..."
        kill $SERVER_PID 2>/dev/null || true
        echo "  ✅ Server stopped"
    fi
}

# Run the test
run_test() {
    echo ""
    echo "🧪 Running RAG pipeline test..."
    echo ""
    
    cd "$PROJECT_DIR"
    
    # Run test binary
    ./test_rag_pipeline
    
    echo ""
    echo "✅ Test completed!"
}

# Main
trap stop_server EXIT

echo "Step 1: Checking environment..."
check_env

echo ""
echo "Step 2: Starting server..."
start_server

echo ""
echo "Step 3: Running tests..."
run_test

echo ""
echo "=== All Done ==="