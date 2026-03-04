package services

import (
	"bufio"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/go-connections/nat"
	"github.com/poc-amp/backend/models"
)

//go:embed templates/*
var templateFS embed.FS

type DockerService struct {
	client      *client.Client
	networkName string
}

func NewDockerService(networkName string) (*DockerService, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	ds := &DockerService{
		client:      cli,
		networkName: networkName,
	}

	if err := ds.ensureNetwork(); err != nil {
		return nil, err
	}

	return ds, nil
}

func (d *DockerService) ensureNetwork() error {
	ctx := context.Background()
	networks, err := d.client.NetworkList(ctx, types.NetworkListOptions{
		Filters: filters.NewArgs(filters.Arg("name", d.networkName)),
	})
	if err != nil {
		return fmt.Errorf("failed to list networks: %w", err)
	}

	for _, net := range networks {
		if net.Name == d.networkName {
			return nil
		}
	}

	_, err = d.client.NetworkCreate(ctx, d.networkName, types.NetworkCreate{
		Driver: "bridge",
		Labels: map[string]string{
			"amp.managed": "true",
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create network: %w", err)
	}

	return nil
}

func (d *DockerService) BuildImage(ctx context.Context, agentID, repoPath string, agentType models.AgentType) (string, error) {
	templateName := fmt.Sprintf("templates/Dockerfile.%s", agentType)
	templateContent, err := templateFS.ReadFile(templateName)
	if err != nil {
		return "", fmt.Errorf("failed to read dockerfile template: %w", err)
	}

	dockerfilePath := filepath.Join(repoPath, "Dockerfile.amp")
	if err := os.WriteFile(dockerfilePath, templateContent, 0644); err != nil {
		return "", fmt.Errorf("failed to write dockerfile: %w", err)
	}
	defer os.Remove(dockerfilePath)

	caCertContent, err := templateFS.ReadFile("templates/ca.crt")
	if err != nil {
		return "", fmt.Errorf("failed to read CA certificate: %w", err)
	}

	caCertPath := filepath.Join(repoPath, "ca.crt")
	if err := os.WriteFile(caCertPath, caCertContent, 0644); err != nil {
		return "", fmt.Errorf("failed to write CA certificate: %w", err)
	}
	defer os.Remove(caCertPath)

	tar, err := archive.TarWithOptions(repoPath, &archive.TarOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create tar archive: %w", err)
	}
	defer tar.Close()

	imageName := fmt.Sprintf("amp-agent-%s:latest", agentID)

	buildOptions := types.ImageBuildOptions{
		Dockerfile: "Dockerfile.amp",
		Tags:       []string{imageName},
		Remove:     true,
		Labels: map[string]string{
			"amp.managed": "true",
			"amp.agent":   agentID,
		},
	}

	resp, err := d.client.ImageBuild(ctx, tar, buildOptions)
	if err != nil {
		return "", fmt.Errorf("failed to build image: %w", err)
	}
	defer resp.Body.Close()

	var lastLine string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		lastLine = scanner.Text()
		var msg struct {
			Stream string `json:"stream"`
			Error  string `json:"error"`
		}
		if err := json.Unmarshal([]byte(lastLine), &msg); err == nil {
			if msg.Error != "" {
				return "", fmt.Errorf("build error: %s", msg.Error)
			}
		}
	}

	return imageName, nil
}

func (d *DockerService) StartContainer(ctx context.Context, agent *models.Agent, repoPath string, port int) (string, error) {
	containerName := fmt.Sprintf("amp-agent-%s", agent.ID)

	portStr := fmt.Sprintf("%d/tcp", 8000)
	hostPortStr := fmt.Sprintf("%d", port)

	exposedPorts := nat.PortSet{
		nat.Port(portStr): struct{}{},
	}

	portBindings := nat.PortMap{
		nat.Port(portStr): []nat.PortBinding{
			{
				HostIP:   "0.0.0.0",
				HostPort: hostPortStr,
			},
		},
	}

	var envVars []string
	if agent.EnvContent != "" {
		lines := strings.Split(agent.EnvContent, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				envVars = append(envVars, line)
			}
		}
	}
	// Inject AMP-required environment variables
	envVars = append(envVars, fmt.Sprintf("PORT=%d", 8000))
	envVars = append(envVars, fmt.Sprintf("AGENT_ID=%s", agent.ID))
	envVars = append(envVars, "AMP_URL=http://backend:8080")
	envVars = append(envVars, "HTTP_PROXY=http://envoy:10000")
	envVars = append(envVars, "HTTPS_PROXY=http://envoy:10000")
	envVars = append(envVars, "REQUESTS_CA_BUNDLE=/usr/local/share/ca-certificates/amp-proxy-ca.crt")
	envVars = append(envVars, "NODE_EXTRA_CA_CERTS=/usr/local/share/ca-certificates/amp-proxy-ca.crt")

	config := &container.Config{
		Image:        fmt.Sprintf("amp-agent-%s:latest", agent.ID),
		ExposedPorts: exposedPorts,
		Env:          envVars,
		Labels: map[string]string{
			"amp.managed": "true",
			"amp.agent":   agent.Name,
			"amp.id":      agent.ID,
		},
	}

	hostConfig := &container.HostConfig{
		PortBindings: portBindings,
		RestartPolicy: container.RestartPolicy{
			Name: "unless-stopped",
		},
	}

	networkConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			d.networkName: {},
		},
	}

	resp, err := d.client.ContainerCreate(ctx, config, hostConfig, networkConfig, nil, containerName)
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	if err := d.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		d.client.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		return "", fmt.Errorf("failed to start container: %w", err)
	}

	return resp.ID, nil
}

func (d *DockerService) StopContainer(ctx context.Context, containerID string) error {
	timeout := 10
	stopOptions := container.StopOptions{Timeout: &timeout}
	return d.client.ContainerStop(ctx, containerID, stopOptions)
}

func (d *DockerService) RemoveContainer(ctx context.Context, containerID string) error {
	return d.client.ContainerRemove(ctx, containerID, container.RemoveOptions{
		Force:         true,
		RemoveVolumes: true,
	})
}

func (d *DockerService) RemoveImage(ctx context.Context, imageID string) error {
	_, err := d.client.ImageRemove(ctx, imageID, image.RemoveOptions{Force: true})
	return err
}

func (d *DockerService) GetContainerLogs(ctx context.Context, containerID string, follow bool) (io.ReadCloser, error) {
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Timestamps: true,
		Tail:       "100",
	}
	return d.client.ContainerLogs(ctx, containerID, options)
}

func (d *DockerService) GetContainerStatus(ctx context.Context, containerID string) (string, error) {
	inspect, err := d.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", err
	}
	return inspect.State.Status, nil
}

func (d *DockerService) IsContainerRunning(ctx context.Context, containerID string) bool {
	status, err := d.GetContainerStatus(ctx, containerID)
	if err != nil {
		return false
	}
	return status == "running"
}

func (d *DockerService) WaitForContainer(ctx context.Context, containerID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if d.IsContainerRunning(ctx, containerID) {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("container did not start within timeout")
}

func (d *DockerService) Close() error {
	return d.client.Close()
}
