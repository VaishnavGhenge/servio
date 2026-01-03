package monitor

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Stats represents VPS health statistics
type Stats struct {
	CPUUsage    float64                `json:"cpu_usage"`
	MemoryUsage float64                `json:"memory_usage"`
	MemoryTotal float64                `json:"memory_total"` // in GB
	MemoryUsed  float64                `json:"memory_used"`  // in GB
	DiskUsage   float64                `json:"disk_usage"`
	DiskTotal   float64                `json:"disk_total"` // in GB
	DiskUsed    float64                `json:"disk_used"`  // in GB
	Uptime      string                 `json:"uptime"`
	OSName      string                 `json:"os_name"`
	OSVersion   string                 `json:"os_version"`
	Services    map[string]ServiceStat `json:"services,omitempty"`
}

type ServiceStat struct {
	MemoryUsage float64 `json:"memory_usage"` // in MB
	CPUUsage    float64 `json:"cpu_usage"`    // percentage
	ActiveState string  `json:"active_state"`
}

// GetStats collects basic VPS stats using standard OS files/commands
func GetStats(serviceNames ...string) Stats {
	if runtime.GOOS == "darwin" {
		return getMacStats(serviceNames...)
	}
	return getLinuxStats(serviceNames...)
}

func getLinuxStats(serviceNames ...string) Stats {
	cpu := getLinuxCPU()
	memUsage, memTotal, memUsed := getLinuxMemory()
	diskUsage, diskTotal, diskUsed := getLinuxDisk()
	uptime := getLinuxUptime()
	osName, osVer := getLinuxOSInfo()

	var services map[string]ServiceStat
	if len(serviceNames) > 0 {
		services = getLinuxServiceStats(serviceNames)
	}

	return Stats{
		CPUUsage:    cpu,
		MemoryUsage: memUsage,
		MemoryTotal: memTotal,
		MemoryUsed:  memUsed,
		DiskUsage:   diskUsage,
		DiskTotal:   diskTotal,
		DiskUsed:    diskUsed,
		Uptime:      uptime,
		OSName:      osName,
		OSVersion:   osVer,
		Services:    services,
	}
}

func getMacStats(serviceNames ...string) Stats {
	services := make(map[string]ServiceStat)
	for _, name := range serviceNames {
		services[name] = ServiceStat{
			MemoryUsage: 45.0 + float64(time.Now().Unix()%10),
			CPUUsage:    1.2 + float64(time.Now().Unix()%5),
			ActiveState: "active",
		}
	}

	return Stats{
		CPUUsage:    getMacCPU(),
		MemoryUsage: 45.0,
		MemoryTotal: 16.0,
		MemoryUsed:  7.2,
		DiskUsage:   getMacDisk(),
		DiskTotal:   512.0,
		DiskUsed:    128.0,
		Uptime:      getMacUptime(),
		OSName:      "macOS",
		OSVersion:   "14.0",
		Services:    services,
	}
}

// --- Linux Implementations ---

func getLinuxCPU() float64 {
	readStat := func() (uint64, uint64) {
		data, err := os.ReadFile("/proc/stat")
		if err != nil {
			return 0, 0
		}
		lines := strings.Split(string(data), "\n")
		if len(lines) == 0 {
			return 0, 0
		}
		fields := strings.Fields(lines[0])
		if len(fields) < 5 {
			return 0, 0
		}

		var total uint64
		// fields[0] is "cpu"
		// indices 1-10 are supported in modern kernels:
		// user, nice, system, idle, iowait, irq, softirq, steal, guest, guest_nice
		for i := 1; i < len(fields); i++ {
			val, _ := strconv.ParseUint(fields[i], 10, 64)
			total += val
		}
		idle, _ := strconv.ParseUint(fields[4], 10, 64)
		iowait, _ := strconv.ParseUint(fields[5], 10, 64)

		return total, idle + iowait
	}

	t1, i1 := readStat()
	time.Sleep(500 * time.Millisecond) // Increased to 500ms for better accuracy
	t2, i2 := readStat()

	if t2 <= t1 {
		return 0
	}

	totalDiff := float64(t2 - t1)
	idleDiff := float64(i2 - i1)

	if totalDiff == 0 {
		return 0
	}

	usage := (totalDiff - idleDiff) / totalDiff * 100
	if usage < 0 {
		return 0
	}
	return usage
}

func getLinuxMemory() (float64, float64, float64) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, 0
	}
	var total, available uint64
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				total, _ = strconv.ParseUint(fields[1], 10, 64)
			}
		} else if strings.HasPrefix(line, "MemAvailable:") || strings.HasPrefix(line, "MemFree:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				val, _ := strconv.ParseUint(fields[1], 10, 64)
				if strings.HasPrefix(line, "MemAvailable:") {
					available = val
				} else if available == 0 {
					available = val
				}
			}
		}
	}
	if total == 0 {
		return 0, 0, 0
	}
	used := total - available
	// Convert KB to GB for UI readability
	totalGB := float64(total) / 1024 / 1024
	usedGB := float64(used) / 1024 / 1024
	return (float64(used) / float64(total)) * 100, totalGB, usedGB
}

func getLinuxDisk() (float64, float64, float64) {
	out, err := exec.Command("df", "-k", "/").Output()
	if err != nil {
		return 0, 0, 0
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) < 2 {
		return 0, 0, 0
	}
	fields := strings.Fields(lines[1])
	if len(fields) < 5 {
		return 0, 0, 0
	}

	// 1-Kblocks Used Available Use%
	totalKB, _ := strconv.ParseFloat(fields[1], 64)
	usedKB, _ := strconv.ParseFloat(fields[2], 64)
	useStr := strings.TrimSuffix(fields[4], "%")
	usage, _ := strconv.ParseFloat(useStr, 64)

	return usage, totalKB / 1024 / 1024, usedKB / 1024 / 1024
}

func getLinuxUptime() string {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return "unknown"
	}
	var uptimeSeconds float64
	fmt.Sscanf(string(data), "%f", &uptimeSeconds)
	return formatUptime(uptimeSeconds)
}

func getLinuxOSInfo() (string, string) {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "Linux", "unknown"
	}

	lines := strings.Split(string(data), "\n")
	var name, version string
	for _, line := range lines {
		if strings.HasPrefix(line, "NAME=") {
			name = strings.Trim(strings.TrimPrefix(line, "NAME="), `"`)
		} else if strings.HasPrefix(line, "VERSION_ID=") {
			version = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), `"`)
		}
	}

	if name == "" {
		name = "Linux"
	}
	return name, version
}

// --- Mac Implementations (Simulated/Basic) ---

func getMacCPU() float64 {
	// Use top command for more accurate CPU on Mac
	out, err := exec.Command("sh", "-c", "top -l 2 -n 0 | grep 'CPU usage' | tail -1").Output()
	if err == nil && len(out) > 0 {
		// Format: CPU usage: 10.0% user, 5.0% sys, 85.0% idle
		line := string(out)
		if idx := strings.Index(line, "idle"); idx > 0 {
			parts := strings.Fields(line)
			for i, p := range parts {
				if strings.Contains(p, "idle") && i > 0 {
					idleStr := strings.TrimSuffix(parts[i-1], "%")
					idle, _ := strconv.ParseFloat(idleStr, 64)
					return 100 - idle
				}
			}
		}
	}

	// Fallback to load average
	out, err = exec.Command("sysctl", "-n", "vm.loadavg").Output()
	if err != nil {
		return 0
	}
	// Format: { 1.50 1.60 1.70 }
	parts := strings.Fields(string(out))
	if len(parts) < 2 {
		return 0
	}
	load, _ := strconv.ParseFloat(parts[1], 64)
	ncpu := float64(runtime.NumCPU())
	usage := (load / ncpu) * 100
	if usage > 100 {
		usage = 100
	}
	return usage
}

func getMacMemory() float64 {
	// Very simple heuristic for Mac
	return 45.0
}

func getMacDisk() float64 {
	return 25.0
}

func getMacUptime() string {
	out, err := exec.Command("sysctl", "-n", "kern.boottime").Output()
	if err != nil {
		return "unknown"
	}
	// Format: { sec = 1625068800, usec = 0 } Mon Jun 30 16:00:00 2021
	parts := strings.Split(string(out), ",")
	if len(parts) < 1 {
		return "unknown"
	}
	secStr := strings.TrimPrefix(parts[0], "{ sec = ")
	sec, _ := strconv.ParseInt(secStr, 10, 64)
	if sec == 0 {
		return "unknown"
	}

	uptimeSeconds := float64(time.Now().Unix() - sec)
	return formatUptime(uptimeSeconds)
}

func formatUptime(uptimeSeconds float64) string {
	d := time.Duration(uptimeSeconds) * time.Second
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24

	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	return fmt.Sprintf("%dh", hours)
}

// --- Service Resource Tracking ---

var (
	serviceCPUMap   sync.Map // map[string]*cpuEntry
	systemTotalCPUs = runtime.NumCPU()
)

type cpuEntry struct {
	lastNS   uint64
	lastTime time.Time
}

func getLinuxServiceStats(serviceNames []string) map[string]ServiceStat {
	stats := make(map[string]ServiceStat)

	for _, name := range serviceNames {
		// systemctl show servio-NAME.service -p CPUUsageNS,MemoryCurrent,ActiveState
		out, err := exec.Command("systemctl", "show", name, "-p", "CPUUsageNS,MemoryCurrent,ActiveState").Output()
		if err != nil {
			continue
		}

		lines := strings.Split(string(out), "\n")
		var s ServiceStat
		var cpuNS uint64
		var memBytes uint64

		for _, line := range lines {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) < 2 {
				continue
			}
			val := parts[1]
			switch parts[0] {
			case "ActiveState":
				s.ActiveState = val
			case "MemoryCurrent":
				if val != "[not set]" {
					v, _ := strconv.ParseUint(val, 10, 64)
					memBytes = v
				}
			case "CPUUsageNS":
				v, _ := strconv.ParseUint(val, 10, 64)
				cpuNS = v
			}
		}

		// Calculate CPU percentage
		now := time.Now()
		if entry, ok := serviceCPUMap.Load(name); ok {
			e := entry.(*cpuEntry)
			diffNS := cpuNS - e.lastNS
			diffTime := now.Sub(e.lastTime).Nanoseconds()

			if diffTime > 0 {
				// CPU usage as percentage (100% = 1 core fully used)
				s.CPUUsage = (float64(diffNS) / float64(diffTime)) * 100
			}
		}

		// Update cache
		serviceCPUMap.Store(name, &cpuEntry{
			lastNS:   cpuNS,
			lastTime: now,
		})

		s.MemoryUsage = float64(memBytes) / 1024 / 1024 // MB
		stats[name] = s
	}

	return stats
}
