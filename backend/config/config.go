package config

import (
	"os"
	"strconv"
)

type Config struct {
	ServerPort     string
	DatabaseURL    string
	WorkspacePath  string
	DockerHost     string
	PortRangeStart int
	PortRangeEnd   int
	NetworkName    string
}

func Load() *Config {
	return &Config{
		ServerPort:     getEnv("SERVER_PORT", "8080"),
		DatabaseURL:    getEnv("DATABASE_URL", "postgres://amp:amp@postgres:5432/amp?sslmode=disable"),
		WorkspacePath:  getEnv("WORKSPACE_PATH", "/workspace"),
		DockerHost:     getEnv("DOCKER_HOST", "unix:///var/run/docker.sock"),
		PortRangeStart: getEnvInt("PORT_RANGE_START", 9001),
		PortRangeEnd:   getEnvInt("PORT_RANGE_END", 9100),
		NetworkName:    getEnv("NETWORK_NAME", "amp-network"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}
