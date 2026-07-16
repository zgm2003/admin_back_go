package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

type composeService struct {
	Image string `yaml:"image"`
	Build struct {
		Context string `yaml:"context"`
	} `yaml:"build"`
	Ports    []string `yaml:"ports"`
	Volumes  []string `yaml:"volumes"`
	Networks []string `yaml:"networks"`
}

type composeContract struct {
	Name     string                    `yaml:"name"`
	Services map[string]composeService `yaml:"services"`
	Networks map[string]struct {
		Name     string `yaml:"name"`
		External bool   `yaml:"external"`
	} `yaml:"networks"`
}

func readComposeContract(t *testing.T, parts ...string) composeContract {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatal(err)
	}
	var contract composeContract
	if err := yaml.Unmarshal(content, &contract); err != nil {
		t.Fatal(err)
	}
	return contract
}

func TestDockerStateComposeOwnsStateServices(t *testing.T) {
	contract := readComposeContract(t, "..", "..", "deploy", "docker-state", "docker-compose.yml")
	if contract.Name != "admin-state" {
		t.Fatalf("name=%q", contract.Name)
	}
	if len(contract.Services) != 2 {
		t.Fatalf("services=%v", contract.Services)
	}

	mysql, mysqlOK := contract.Services["mysql"]
	redis, redisOK := contract.Services["redis"]
	if !mysqlOK || mysql.Image != "mysql:8.4.10" ||
		!reflect.DeepEqual(mysql.Ports, []string{"127.0.0.1:${ADMIN_MYSQL_HOST_PORT:-33306}:3306"}) ||
		!reflect.DeepEqual(mysql.Volumes, []string{"mysql-data:/var/lib/mysql"}) {
		t.Fatal("invalid MySQL contract")
	}
	if !redisOK || redis.Image != "redis:8.2.7-alpine" ||
		!reflect.DeepEqual(redis.Ports, []string{"127.0.0.1:${ADMIN_REDIS_HOST_PORT:-36379}:6379"}) ||
		!reflect.DeepEqual(redis.Volumes, []string{"redis-data:/data"}) {
		t.Fatal("invalid Redis contract")
	}
	if contract.Networks["platform"].Name != "admin-platform" || contract.Networks["platform"].External {
		t.Fatal("state must own admin-platform")
	}
}

func TestDockerAppComposeOwnsOnlyApplicationServices(t *testing.T) {
	contract := readComposeContract(t, "..", "..", "deploy", "docker-first", "docker-compose.yml")
	if contract.Name != "admin-app" {
		t.Fatalf("name=%q", contract.Name)
	}
	for _, name := range []string{"frontend", "admin-api", "admin-worker"} {
		if _, ok := contract.Services[name]; !ok {
			t.Fatalf("missing %s", name)
		}
	}
	for _, name := range []string{"mysql", "redis"} {
		if _, ok := contract.Services[name]; ok {
			t.Fatalf("app owns %s", name)
		}
	}

	frontend := contract.Services["frontend"]
	if frontend.Build.Context != "../../../admin_front_ts" ||
		!reflect.DeepEqual(frontend.Ports, []string{"127.0.0.1:5173:8080"}) {
		t.Fatal("invalid frontend contract")
	}
	if !contract.Networks["platform"].External || contract.Networks["platform"].Name != "admin-platform" {
		t.Fatal("app must consume external admin-platform")
	}
}
