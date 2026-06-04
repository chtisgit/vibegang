package main

import (
	"fmt"
	"os/exec"
	"strings"
)

func getDBConnStr() string {
	cmd := exec.Command("docker", "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", "vibegang-postgres")
	ipOut, err := cmd.Output()
	if err == nil {
		ip := strings.TrimSpace(string(ipOut))
		if ip != "" {
			return fmt.Sprintf("postgres://postgres:password@%s:5432/vibegang?sslmode=disable", ip)
		}
	}
	return "postgres://postgres:password@localhost:5432/vibegang?sslmode=disable"
}
