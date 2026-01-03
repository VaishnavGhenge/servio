# Servio Deployment Guide for AWS EC2 (Amazon Linux 2023 ARM64)

## Instance Details

- **Instance Type**: t4g.micro (ARM64 architecture)
- **OS**: Amazon Linux 2023
- **Host**: ec2-13-127-136-60.ap-south-1.compute.amazonaws.com
- **IP**: 13.127.136.60
- **Architecture**: ARM64 (aarch64)

## Prerequisites

### 1. Fix Security Group for SSH Access

**CRITICAL**: The current security group doesn't allow SSH (port 22) access. Add an inbound rule:

- Type: SSH
- Protocol: TCP
- Port: 22
- Source: Your IP (or 0.0.0.0/0 for testing, but restrict in production)

Current security group allows:
- Port 8080 (Servio UI)
- Port 8081 (Git Visualizer)

### 2. Verify SSH Key Permissions

```bash
chmod 400 "/Users/vaishnavghenge/projects/git-visualizer/backend/local/office mac.pem"
```

## Deployment Options

### Option 1: Quick Deployment (Process-based)

Use this for quick testing or development:

```bash
./deploy.sh
```

This will:
- Build ARM64 binary with CGO disabled
- Upload to /opt
- Run servio as a background process using nohup
- Start listening on port 8080

### Option 2: Production Deployment (Systemd-based)

Use this for production:

```bash
chmod +x deploy-systemd.sh
./deploy-systemd.sh
```

This will:
- Build ARM64 binary
- Install as systemd service
- Enable auto-start on boot
- Provide proper logging via journald

## Diagnostic Steps

### 1. Run Diagnostic Script

Upload and run the diagnostic script on the EC2 instance:

```bash
# Copy diagnostic script
scp -i "/Users/vaishnavghenge/projects/git-visualizer/backend/local/office mac.pem" \
    diagnose.sh ec2-user@ec2-13-127-136-60.ap-south-1.compute.amazonaws.com:/tmp/

# Run it
ssh -i "/Users/vaishnavghenge/projects/git-visualizer/backend/local/office mac.pem" \
    ec2-user@ec2-13-127-136-60.ap-south-1.compute.amazonaws.com \
    'chmod +x /tmp/diagnose.sh && sudo /tmp/diagnose.sh'
```

### 2. Manual SSH Diagnostics

```bash
# Connect to instance
ssh -i "/Users/vaishnavghenge/projects/git-visualizer/backend/local/office mac.pem" \
    ec2-user@ec2-13-127-136-60.ap-south-1.compute.amazonaws.com

# Check if servio is running
ps aux | grep servio

# Check systemd service
sudo systemctl status servio

# View logs
sudo journalctl -u servio -f

# Check if listening on port
sudo ss -tlnp | grep 8080

# Test servio manually
cd /opt
sudo ./servio -addr :8080 -db /var/lib/servio/data.db
```

## Common Issues and Solutions

### Issue 1: Binary Architecture Mismatch

**Symptoms**:
- "cannot execute binary file: Exec format error"
- Binary fails to run

**Solution**:
```bash
# Verify binary is ARM64
file /opt/servio
# Should show: "ELF 64-bit LSB executable, ARM aarch64"

# Rebuild with correct architecture
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o servio ./cmd/servio
```

### Issue 2: Permission Denied

**Symptoms**:
- "permission denied" when running servio

**Solution**:
```bash
sudo chmod +x /opt/servio
```

### Issue 3: Port Already in Use

**Symptoms**:
- "address already in use"

**Solution**:
```bash
# Find process using port 8080
sudo ss -tlnp | grep :8080

# Kill the process
sudo pkill servio

# Or restart systemd service
sudo systemctl restart servio
```

### Issue 4: Database Permission Issues

**Symptoms**:
- "unable to open database file"

**Solution**:
```bash
# Create directory and set permissions
sudo mkdir -p /var/lib/servio
sudo chown -R root:root /var/lib/servio
```

### Issue 5: SELinux Blocking

**Symptoms**:
- Binary runs but doesn't start properly
- Permission errors in logs

**Solution**:
```bash
# Check SELinux status
getenforce

# Temporarily disable (testing only)
sudo setenforce 0

# View SELinux denials
sudo ausearch -m avc -ts recent
```

### Issue 6: Service Not Starting with Systemd

**Symptoms**:
- systemctl status shows "failed" or "inactive"

**Solution**:
```bash
# Check service logs
sudo journalctl -u servio --no-pager -n 50

# Verify service file
sudo systemctl cat servio

# Test binary manually first
cd /opt
sudo ./servio -addr :8080 -db /var/lib/servio/data.db

# Reload systemd if service file changed
sudo systemctl daemon-reload
sudo systemctl restart servio
```

## Accessing Servio

Once deployed and running:

```
http://13.127.136.60:8080
```

Or use the public DNS:
```
http://ec2-13-127-136-60.ap-south-1.compute.amazonaws.com:8080
```

## Useful Commands

### Service Management
```bash
# Start service
sudo systemctl start servio

# Stop service
sudo systemctl stop servio

# Restart service
sudo systemctl restart servio

# Enable auto-start
sudo systemctl enable servio

# Disable auto-start
sudo systemctl disable servio

# View status
sudo systemctl status servio
```

### Logs
```bash
# Follow logs in real-time
sudo journalctl -u servio -f

# View last 100 lines
sudo journalctl -u servio -n 100

# View logs from last hour
sudo journalctl -u servio --since "1 hour ago"

# View process logs (if using nohup)
tail -f /opt/servio.log
```

### Monitoring
```bash
# Check process
ps aux | grep servio

# Check port
sudo ss -tlnp | grep 8080

# Check system resources
top -p $(pgrep servio)

# Check connections
sudo ss -tnp | grep servio
```

## Build Flags Explanation

```bash
GOOS=linux         # Target Linux OS
GOARCH=arm64       # Target ARM64 architecture (for t4g instances)
CGO_ENABLED=0      # Disable CGO for static binary (no C dependencies)
go build -o servio # Build binary named 'servio'
```

## Next Steps After Fixing SSH

1. Add SSH rule to security group (port 22)
2. Test SSH connection
3. Run diagnostic script to identify current issues
4. Choose deployment method (process or systemd)
5. Deploy using appropriate script
6. Verify servio is running and accessible

## Rollback

If deployment fails:

```bash
# Process-based deployment
ssh -i "$PEM_KEY" $HOST 'sudo pkill servio && sudo mv /opt/servio.bak.* /opt/servio && cd /opt && sudo nohup ./servio > servio.log 2>&1 &'

# Systemd-based deployment
ssh -i "$PEM_KEY" $HOST 'sudo systemctl stop servio && sudo mv /opt/servio.bak.* /opt/servio && sudo systemctl start servio'
```
