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
	ip := strings.TrimSpace(string(ipOut))

	if err != nil || ip == "" {
		// Try auto-detecting any running vibegang postgres container
		out, err := exec.Command("docker", "ps", "--filter", "name=-postgres", "--format", "{{.Names}}").Output()
		if err == nil {
			names := strings.Split(strings.TrimSpace(string(out)), "\n")
			for _, name := range names {
				name = strings.TrimSpace(name)
				if strings.HasPrefix(name, "vibegang-") && strings.HasSuffix(name, "-postgres") {
					pgName = name
					cmdInspect := exec.Command("docker", "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", pgName)
					if ipOut2, err2 := cmdInspect.Output(); err2 == nil {
						ip = strings.TrimSpace(string(ipOut2))
						if ip != "" {
							break
						}
					}
				}
			}
		}
	}

	if ip != "" {
		return fmt.Sprintf("postgres://postgres:password@%s:5432/vibegang?sslmode=disable", ip)
	}
	return "postgres://postgres:password@localhost:5432/vibegang?sslmode=disable"
}
