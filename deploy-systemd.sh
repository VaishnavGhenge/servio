#!/bin/bash
set -e

# Configuration
PEM_KEY="/Users/vaishnavghenge/projects/git-visualizer/backend/local/office mac.pem"
HOST="ec2-user@ec2-13-127-136-60.ap-south-1.compute.amazonaws.com"
BINARY_NAME="servio-linux-arm64"

echo "🚀 Building Linux/ARM64 binary..."
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o $BINARY_NAME ./cmd/servio

echo "📦 Uploading files to $HOST..."
scp -o StrictHostKeyChecking=no -i "$PEM_KEY" $BINARY_NAME $HOST:/tmp/servio-new
scp -o StrictHostKeyChecking=no -i "$PEM_KEY" servio.service $HOST:/tmp/servio.service

if [ -f .env ]; then
    echo "📦 Uploading .env file..."
    scp -o StrictHostKeyChecking=no -i "$PEM_KEY" .env $HOST:/tmp/.env
else
    echo "Warning: .env file not found, skipping..."
fi

echo "🔄 Installing servio with systemd..."
ssh -o StrictHostKeyChecking=no -i "$PEM_KEY" $HOST << 'EOF'
    set -e

    echo "Stopping existing servio service..."
    sudo systemctl stop servio || true
    sudo pkill servio || true
    sleep 1

    echo "Ensuring Nginx is installed..."
    if ! command -v nginx &> /dev/null; then
        echo "Installing Nginx..."
        if command -v dnf &> /dev/null; then
            sudo dnf install -y nginx
        elif command -v apt-get &> /dev/null; then
            sudo apt-get update && sudo apt-get install -y nginx
        else
            echo "Error: Package manager not found (expected dnf or apt)"
            exit 1
        fi
        sudo systemctl enable --now nginx
    else
        echo "Nginx already installed"
    fi

    echo "Installing binary..."
    sudo mv /tmp/servio-new /opt/servio
    sudo chmod +x /opt/servio

    echo "Installing .env file..."
    if [ -f /tmp/.env ]; then
        sudo mv /tmp/.env /opt/.env
        sudo chmod 600 /opt/.env
    else
        echo "No .env file to install"
    fi

    echo "Creating directories..."
    sudo mkdir -p /var/lib/servio
    sudo mkdir -p /var/log/servio

    echo "Installing systemd service..."
    sudo mv /tmp/servio.service /etc/systemd/system/servio.service
    sudo chmod 644 /etc/systemd/system/servio.service

    echo "Reloading systemd..."
    sudo systemctl daemon-reload

    echo "Enabling and starting servio service..."
    sudo systemctl enable servio
    sudo systemctl start servio

    echo "Waiting for service to start..."
    sleep 3

    echo "Service status:"
    sudo systemctl status servio --no-pager -l || true

    echo ""
    echo "Checking if servio is listening on port 8080..."
    sudo ss -tlnp | grep :8080 || echo "Warning: Not listening on port 8080 yet"

    echo ""
    echo "Recent logs:"
    sudo journalctl -u servio --no-pager -n 20

    echo ""
    if sudo systemctl is-active --quiet servio; then
        echo "✅ Servio is running!"
    else
        echo "❌ Servio failed to start. Check logs with:"
        echo "   sudo journalctl -u servio -f"
        exit 1
    fi
EOF

echo "✨ Deployment complete!"
echo ""
echo "Useful commands:"
echo "  View logs:    ssh -i \"$PEM_KEY\" $HOST 'sudo journalctl -u servio -f'"
echo "  Service status: ssh -i \"$PEM_KEY\" $HOST 'sudo systemctl status servio'"
echo "  Restart:      ssh -i \"$PEM_KEY\" $HOST 'sudo systemctl restart servio'"
