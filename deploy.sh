#!/bin/bash
set -e

# Configuration
PEM_KEY="/Users/vaishnavghenge/projects/git-repository-visualizer/local/office mac.pem"
HOST="ec2-user@ec2-13-235-64-198.ap-south-1.compute.amazonaws.com"
BINARY_NAME="servio-linux"
REMOTE_DIR="/home/ec2-user"

echo "🚀 Building Linux/ARM64 binary..."
GOOS=linux GOARCH=arm64 go build -o $BINARY_NAME ./cmd/servio

echo "📦 Uploading binary to $HOST..."
scp -o StrictHostKeyChecking=no -i "$PEM_KEY" $BINARY_NAME $HOST:$REMOTE_DIR/servio-new

echo "🔄 Restarting service on remote..."
ssh -o StrictHostKeyChecking=no -i "$PEM_KEY" $HOST << EOF
    # Stop existing service
    sudo pkill servio || true
    
    # Backup old binary
    mv servio servio.bak || true
    
    # Move new binary into place
    mv servio-new servio
    chmod +x servio
    
    # Start new service
    sudo nohup ./servio > servio.log 2>&1 &
    
    # Verify it started
    echo "Sleeping 2s to verify startup..."
    sleep 2
    ps -C servio
    echo "✅ Servio deployed and running!"
EOF

echo "✨ Deployment complete!"
