package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

func detectMemoryLimitLinux() int {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return detectMemoryLimitDefault()
	}
	defer f.Close()

	var totalKB, availableKB int64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			totalKB = parseKB(line)
		case strings.HasPrefix(line, "MemAvailable:"):
			availableKB = parseKB(line)
		}
	}

	if totalKB == 0 {
		return detectMemoryLimitDefault()
	}

	avail := availableKB
	if avail == 0 {
		avail = totalKB
	}

	availGB := float64(avail) / 1024 / 1024

	switch {
	case availGB < 2:
		return 2
	case availGB < 4:
		return 5
	default:
		return 20
	}
}

func detectMemoryLimitDefault() int {
	return 4
}

func parseKB(line string) int64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	v, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return v
}
