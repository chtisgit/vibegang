package orchestrator

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/chtisgit/vibegang/pkg/config"
	"github.com/chtisgit/vibegang/pkg/db"

	"github.com/lib/pq"
)

type Orchestrator struct {
	Config *config.Config
	Reset  bool
}

func NewOrchestrator(cfg *config.Config, reset bool) (*Orchestrator, error) {
	return &Orchestrator{
		Config: cfg,
		Reset:  reset,
	}, nil
}

func (o *Orchestrator) StartSystem(ctx context.Context) error {
	log.Println("Creating Docker network...")
	if err := o.ensureNetwork("vibegang-net"); err != nil {
		return err
	}

	log.Println("Starting PostgreSQL database...")
	dbIP, err := o.startPostgres()
	if err != nil {
		return err
	}

	log.Println("Waiting for Postgres to be ready...")
	time.Sleep(5 * time.Second) // Wait for PG to start up

	dbConnStr := fmt.Sprintf("postgres://postgres:password@%s:5432/vibegang?sslmode=disable", dbIP)
	log.Println("Initializing Database schema...")
	dbClient, err := db.NewDB(dbConnStr)
	if err != nil {
		return fmt.Errorf("failed to connect to postgres: %w", err)
	}
	defer dbClient.Close()

	if o.Reset {
		log.Println("Resetting Database (clearing all tables)...")
		if err := dbClient.ClearTables(); err != nil {
			return fmt.Errorf("failed to reset database: %w", err)
		}
	} else {
		if err := dbClient.SetupSchema(); err != nil {
			return fmt.Errorf("failed to setup schema: %w", err)
		}
	}

	log.Println("Building Agent Docker image...")
	if err := o.buildAgentImage(); err != nil {
		return fmt.Errorf("failed to build agent image: %w", err)
	}

	log.Println("Starting Agent containers...")
	for _, agent := range o.Config.Agents {
		err := o.startAgent(agent, dbConnStr)
		if err != nil {
			log.Printf("Failed to start agent %s: %v", agent.Name, err)
		} else {
			log.Printf("Agent %s started successfully.", agent.Name)
		}
	}

	log.Println("Listening for agent activity logs...")
	if err := o.listenToLogs(ctx, dbConnStr); err != nil {
		return fmt.Errorf("failed to listen to logs: %w", err)
	}

	return nil
}

func (o *Orchestrator) ensureNetwork(name string) error {
	// Check if network exists
	out, err := exec.Command("docker", "network", "ls", "--format", "{{.Name}}").Output()
	if err == nil && strings.Contains(string(out), name) {
		return nil // Network exists
	}
	return exec.Command("docker", "network", "create", name).Run()
}

func (o *Orchestrator) startPostgres() (string, error) {
	pgName := o.Config.GetPostgresContainerName()
	// Check if container already exists
	out, err := exec.Command("docker", "ps", "-a", "--format", "{{.Names}}").Output()
	if err == nil && strings.Contains(string(out), pgName) {
		// Just start it
		exec.Command("docker", "start", pgName).Run()
	} else {
		// Create and start
		cmd := exec.Command("docker", "run", "-d", "--name", pgName,
			"--network", "vibegang-net",
			"-v", fmt.Sprintf("%s-data:/var/lib/postgresql/data", pgName),
			"-e", "POSTGRES_PASSWORD=password",
			"-e", "POSTGRES_DB=vibegang",
			"postgres:15-alpine")
		if err := cmd.Run(); err != nil {
			return "", err
		}
	}

	// Get IP
	ipOut, err := exec.Command("docker", "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", pgName).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(ipOut)), nil
}

func (o *Orchestrator) buildAgentImage() error {
	buildCmd := exec.Command("go", "build", "-o", "vibegang-agent", "./cmd/vibegang-agent")
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to compile agent: %w", err)
	}

	cmd := exec.Command("docker", "build", "-t", "vibegang-agent:latest", ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (o *Orchestrator) getContainerName(email string) string {
	localPart := email
	if idx := strings.Index(email, "@"); idx != -1 {
		localPart = email[:idx]
	}
	localPart = strings.ReplaceAll(localPart, ".", "-")

	return fmt.Sprintf("vibegang-%s-%s", o.Config.GetCompanyShortform(), localPart)
}

func (o *Orchestrator) startAgent(agent config.AgentConfig, dbConnStr string) error {
	containerName := o.getContainerName(agent.Email)
	localPart := agent.Email
	if idx := strings.Index(agent.Email, "@"); idx != -1 {
		localPart = agent.Email[:idx]
	}
	localPart = strings.ReplaceAll(localPart, ".", "-")

	hostWorkspace := filepath.Join("/tmp/vibegang", localPart)
	os.MkdirAll(hostWorkspace, 0755)

	sshKeyPath := o.Config.SSHKeyPath
	toolsJoined := strings.Join(agent.Tools, ",")

	agentModel := agent.Model
	if agentModel == "" {
		agentModel = o.Config.Model
	}

	// Remove old container if it exists
	exec.Command("docker", "rm", "-f", containerName).Run()

	args := []string{
		"run", "-d",
		"--name", containerName,
		"--network", "vibegang-net",
		"-v", fmt.Sprintf("%s:/workspace", hostWorkspace),
	}

	if sshKeyPath != "" && sshKeyPath != "<none>" {
		args = append(args, "-v", fmt.Sprintf("%s:/root/.ssh/id_rsa:ro", sshKeyPath))
	}

	for _, envVar := range []string{"GEMINI_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "TOGETHER_API_KEY", "CUSTOM_API_KEY", "CUSTOM_PROVIDER", "CUSTOM_BASE_URL", "CUSTOM_MODEL"} {
		if val := os.Getenv(envVar); val != "" {
			args = append(args, "-e", fmt.Sprintf("%s=%s", envVar, val))
		}
	}

	fullPrompt := config.GetTeamSummary(o.Config) + agent.SystemPrompt

	args = append(args,
		"-e", fmt.Sprintf("AGENT_NAME=%s", agent.Name),
		"-e", fmt.Sprintf("AGENT_EMAIL=%s", agent.Email),
		"-e", fmt.Sprintf("AGENT_ROLE=%s", agent.Role),
		"-e", fmt.Sprintf("AGENT_PROMPT=%s", fullPrompt),
		"-e", fmt.Sprintf("AGENT_TOOLS=%s", toolsJoined),
		"-e", fmt.Sprintf("AGENT_MODEL=%s", agentModel),
		"-e", fmt.Sprintf("DB_CONN_STR=%s", dbConnStr),
		"vibegang-agent:latest",
	)

	cmd := exec.Command("docker", args...)

	return cmd.Run()
}

func (o *Orchestrator) listenToLogs(ctx context.Context, dbConnStr string) error {
	reportProblem := func(ev pq.ListenerEventType, err error) {
		if err != nil {
			log.Printf("Log Listener error: %v", err)
		}
	}

	listener := pq.NewListener(dbConnStr, 10*time.Second, time.Minute, reportProblem)
	defer listener.Close()

	err := listener.Listen("log_events")
	if err != nil {
		return err
	}

	dbClient, err := db.NewDB(dbConnStr)
	if err != nil {
		return err
	}
	defer dbClient.Close()

	for {
		select {
		case n := <-listener.Notify:
			var id int
			if _, err := fmt.Sscanf(n.Extra, "%d", &id); err == nil {
				t, email, action, err := dbClient.GetLog(id)
				if err == nil {
					fmt.Printf("[%s] [%s] (#%d): %s\n", t.Local().Format("2006-01-02 15:04:05"), email, id, action)
				} else {
					log.Printf("Failed to retrieve log: %v", err)
				}
			} else {
				fmt.Println(n.Extra)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (o *Orchestrator) StopSystem() {
	log.Println("Terminating agent containers...")
	for _, agent := range o.Config.Agents {
		containerName := o.getContainerName(agent.Email)

		log.Printf("Stopping container %s...", containerName)
		exec.Command("docker", "rm", "-f", containerName).Run()
	}

	log.Println("Terminating database container...")
	exec.Command("docker", "rm", "-f", o.Config.GetPostgresContainerName()).Run()
}
