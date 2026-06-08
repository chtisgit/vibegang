package main

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/chtisgit/vibegang/pkg/config"
)

func getDBConnStr() string {
	pgName := "vibegang-postgres"
	cfgPath := cfgFile
	if cfgPath == "" {
		cfgPath = "vibegang.yaml"
	}
	cfg, err := config.LoadConfig(cfgPath)
	if err == nil {
		pgName = cfg.GetPostgresContainerName()
	}

	cmd := exec.Command("docker", "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", pgName)
	ipOut, err := cmd.Output()
	if err == nil {
		ip := strings.TrimSpace(string(ipOut))
		if ip != "" {
			return fmt.Sprintf("postgres://postgres:password@%s:5432/vibegang?sslmode=disable", ip)
		}
	}
	return "postgres://postgres:password@localhost:5432/vibegang?sslmode=disable"
}
