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
	ip := strings.TrimSpace(string(ipOut))

	if err != nil || ip == "" {
		// Try auto-detecting any running vibegang postgres container
		out, err := exec.Command("docker", "ps", "--filter", "name=-postgres", "--format", "{{.Names}}").Output()
		if err == nil {
			names := strings.Split(strings.TrimSpace(string(out)), "\n")
			var matchedNames []string
			for _, name := range names {
				name = strings.TrimSpace(name)
				if strings.HasPrefix(name, "vibegang-") && strings.HasSuffix(name, "-postgres") {
					matchedNames = append(matchedNames, name)
				}
			}
			// Only auto-detect if exactly one matching container is running.
			// If multiple stacks are running and we can't load the config, do not guess.
			if len(matchedNames) == 1 {
				pgName = matchedNames[0]
				cmdInspect := exec.Command("docker", "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", pgName)
				if ipOut2, err2 := cmdInspect.Output(); err2 == nil {
					ip = strings.TrimSpace(string(ipOut2))
				}
			}
		}
	}

	if ip != "" {
		return fmt.Sprintf("postgres://postgres:password@%s:5432/vibegang?sslmode=disable", ip)
	}
	return "postgres://postgres:password@localhost:5432/vibegang?sslmode=disable"
}
