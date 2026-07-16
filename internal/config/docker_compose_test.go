package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDockerFirstComposeProvidesIsolatedRedis(t *testing.T) {
	type service struct {
		Image       string   `yaml:"image"`
		Command     []string `yaml:"command"`
		Ports       []string `yaml:"ports"`
		Volumes     []string `yaml:"volumes"`
		Healthcheck struct {
			Test []string `yaml:"test"`
		} `yaml:"healthcheck"`
		DependsOn map[string]struct {
			Condition string `yaml:"condition"`
		} `yaml:"depends_on"`
	}
	var compose struct {
		Services map[string]service `yaml:"services"`
		Volumes  map[string]any     `yaml:"volumes"`
	}

	content, err := os.ReadFile(filepath.Join("..", "..", "deploy", "docker-first", "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(content, &compose); err != nil {
		t.Fatalf("parse docker-compose.yml: %v", err)
	}

	redis, ok := compose.Services["redis"]
	if !ok {
		t.Fatal("docker-compose.yml must define redis")
	}
	if redis.Image != "docker.m.daocloud.io/library/redis:8.2.7-alpine" {
		t.Fatalf("unexpected Redis image %q", redis.Image)
	}
	if !reflect.DeepEqual(redis.Ports, []string{"127.0.0.1:36379:6379"}) {
		t.Fatalf("Redis must bind only to isolated loopback port 36379, got %v", redis.Ports)
	}
	if !reflect.DeepEqual(redis.Command, []string{"redis-server", "--appendonly", "yes"}) {
		t.Fatalf("Redis must enable AOF persistence, got %v", redis.Command)
	}
	if !reflect.DeepEqual(redis.Volumes, []string{"redis-data:/data"}) {
		t.Fatalf("Redis must use the named data volume, got %v", redis.Volumes)
	}
	if !reflect.DeepEqual(redis.Healthcheck.Test, []string{"CMD", "redis-cli", "ping"}) {
		t.Fatalf("Redis must expose a PING health check, got %v", redis.Healthcheck.Test)
	}
	if _, ok := compose.Volumes["redis-data"]; !ok {
		t.Fatal("docker-compose.yml must declare redis-data")
	}
	for _, name := range []string{"admin-api", "admin-worker"} {
		dependency, ok := compose.Services[name].DependsOn["redis"]
		if !ok || dependency.Condition != "service_healthy" {
			t.Fatalf("%s must wait for healthy Redis", name)
		}
	}
}
