package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/chtisgit/vibegang/pkg/config"
)

func findConfigPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	cwd, err := os.Getwd()
	if err != nil {
		return path
	}

	current := cwd
	for {
		target := filepath.Join(current, path)
		if _, err := os.Stat(target); err == nil {
			return target
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return path
}

func getDBConnStr() string {
	pgName := "vibegang-postgres"
	cfgPath := cfgFile
	if cfgPath == "" {
		cfgPath = "vibegang.yaml"
	}
	
	resolvedPath := findConfigPath(cfgPath)
	cfg, err := config.LoadConfig(resolvedPath)
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
