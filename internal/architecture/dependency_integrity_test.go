package architecture

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const asynqmonV072Sum = "github.com/hibiken/asynqmon v0.7.2 h1:YohWgTIPwtMyZ6khBDcVUz9BdSdQW2Dxn8SoxtbmjSg="

func requireWindowsPowerShell(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("Windows PowerShell behavior test")
	}
	powerShell, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Skipf("powershell.exe is unavailable: %v", err)
	}
	return powerShell
}

func copyTestFile(t *testing.T, source, destination string) {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeFakeGoCommand(t *testing.T, directory string) {
	t.Helper()
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "@echo off\r\nif \"%FAKE_GO_EXIT%\"==\"23\" exit /b 23\r\nexit /b 0\r\n"
	if err := os.WriteFile(filepath.Join(directory, "go.cmd"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runBackendVerificationWithFakeGo(t *testing.T, powerShell, sandbox, script, fakeBin string) ([]byte, error) {
	t.Helper()
	harness := filepath.Join(sandbox, "run-backend-verification.ps1")
	harnessSource := `param(
  [Parameter(Mandatory = $true)]
  [string]$ScriptPath,
  [Parameter(Mandatory = $true)]
  [string]$FakeBin
)

$ErrorActionPreference = "Stop"
$env:Path = $FakeBin + [IO.Path]::PathSeparator + $env:Path
$env:FAKE_GO_EXIT = "23"
& $ScriptPath
`
	if err := os.WriteFile(harness, []byte(harnessSource), 0o600); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(
		powerShell,
		"-NoProfile",
		"-ExecutionPolicy", "Bypass",
		"-File", harness,
		"-ScriptPath", script,
		"-FakeBin", fakeBin,
	)
	return command.CombinedOutput()
}

func containsExactLine(data []byte, expected string) bool {
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSuffix(line, "\r") == expected {
			return true
		}
	}
	return false
}

func goModLineFields(line string) []string {
	if comment := strings.Index(line, "//"); comment >= 0 {
		line = line[:comment]
	}
	line = strings.ReplaceAll(line, "(", " ( ")
	line = strings.ReplaceAll(line, ")", " ) ")
	return strings.Fields(line)
}

func goModTokenValue(token string) (string, bool) {
	if value, err := strconv.Unquote(token); err == nil {
		return value, true
	}
	return strings.Trim(token, "\"`"), !strings.ContainsAny(token, "\"`")
}

func goModTopLevelDirectiveValue(data []byte, key string) (string, bool) {
	inBlock := false
	for _, line := range strings.Split(string(data), "\n") {
		fields := goModLineFields(line)
		if len(fields) == 0 {
			continue
		}
		if inBlock {
			if fields[0] == ")" {
				inBlock = false
			}
			continue
		}
		if len(fields) >= 2 && fields[1] == "(" {
			inBlock = true
			continue
		}
		if len(fields) >= 2 && fields[0] == key {
			return fields[1], true
		}
	}
	return "", false
}

func goModValue(data []byte, key string) (string, bool) {
	if key == "go" {
		return goModTopLevelDirectiveValue(data, key)
	}
	inRequireBlock := false
	for _, line := range strings.Split(string(data), "\n") {
		fields := goModLineFields(line)
		if len(fields) == 0 {
			continue
		}
		if inRequireBlock {
			if fields[0] == ")" {
				inRequireBlock = false
				continue
			}
			if len(fields) >= 2 && fields[0] == key {
				return fields[1], true
			}
			continue
		}
		if fields[0] != "require" || len(fields) < 2 {
			continue
		}
		if fields[1] == "(" {
			inRequireBlock = true
			continue
		}
		if len(fields) >= 3 && fields[1] == key {
			return fields[2], true
		}
	}
	return "", false
}

func protectedGoModReplacements(data []byte) []string {
	var protected []string
	inReplaceBlock := false
	for _, line := range strings.Split(string(data), "\n") {
		fields := goModLineFields(line)
		if len(fields) == 0 {
			continue
		}
		original := ""
		if inReplaceBlock {
			if fields[0] == ")" {
				inReplaceBlock = false
				continue
			}
			original = fields[0]
		} else if fields[0] == "replace" && len(fields) >= 2 {
			if fields[1] == "(" {
				inReplaceBlock = true
				continue
			}
			original = fields[1]
		}
		original, valid := goModTokenValue(original)
		for _, modulePath := range []string{"github.com/quic-go/quic-go", "golang.org/x/image"} {
			if original == modulePath || (!valid && strings.HasPrefix(original, modulePath)) {
				protected = append(protected, modulePath)
				break
			}
		}
	}
	return protected
}

func dockerfileSecureFoundationProblems(data []byte) []string {
	const expectedArg = "GO_BUILD_IMAGE=golang:1.26.5-bookworm"

	var problems []string
	argCount := 0
	validGlobalArg := false
	firstFromFound := false
	firstFromImage := ""
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(rawLine, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch strings.ToUpper(fields[0]) {
		case "ARG":
			argument := strings.TrimSpace(line[len(fields[0]):])
			name, _, _ := strings.Cut(argument, "=")
			if strings.TrimSpace(name) != "GO_BUILD_IMAGE" {
				continue
			}
			argCount++
			if !firstFromFound && argument == expectedArg {
				validGlobalArg = true
			}
		case "FROM":
			if firstFromFound {
				continue
			}
			firstFromFound = true
			for _, field := range fields[1:] {
				if !strings.HasPrefix(field, "--") {
					firstFromImage = field
					break
				}
			}
		}
	}
	if argCount != 1 {
		problems = append(problems, fmt.Sprintf("Dockerfile must declare exactly one GO_BUILD_IMAGE ARG, found %d", argCount))
	} else if !validGlobalArg {
		problems = append(problems, "Dockerfile GO_BUILD_IMAGE ARG must be the approved global ARG before the first FROM")
	}
	if !firstFromFound {
		problems = append(problems, "Dockerfile must contain an active FROM")
	} else if firstFromImage != "${GO_BUILD_IMAGE}" {
		problems = append(problems, fmt.Sprintf("Dockerfile first FROM image is %q, want ${GO_BUILD_IMAGE}", firstFromImage))
	}
	return problems
}

func uniqueYAMLMappingValue(node *yaml.Node, key string) (*yaml.Node, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("parent is %v, want mapping", node.Kind)
	}
	var value *yaml.Node
	matches := 0
	for index := 0; index+1 < len(node.Content); index += 2 {
		keyNode := node.Content[index]
		if keyNode.Kind != yaml.ScalarNode || keyNode.Tag != "!!str" {
			return nil, fmt.Errorf("mapping key is kind %v with tag %q, want string scalar", keyNode.Kind, keyNode.Tag)
		}
		if keyNode.Value == key {
			matches++
			value = node.Content[index+1]
		}
	}
	if matches != 1 {
		return nil, fmt.Errorf("key %q occurs %d times, want exactly one", key, matches)
	}
	return value, nil
}

func singleYAMLMapping(data []byte, label string) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document, extra yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("%s is invalid YAML: %w", label, err)
	}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return nil, fmt.Errorf("%s is invalid YAML: %w", label, err)
		}
		return nil, fmt.Errorf("%s must contain one YAML document", label)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s root must be one mapping", label)
	}
	return document.Content[0], nil
}

func exactYAMLMapping(node *yaml.Node, path string, keys ...string) error {
	if node.Kind != yaml.MappingNode || len(node.Content) != len(keys)*2 {
		return fmt.Errorf("%s must contain exactly keys %v", path, keys)
	}
	for _, key := range keys {
		if _, err := uniqueYAMLMappingValue(node, key); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return nil
}

func yamlPath(node *yaml.Node, path ...string) (*yaml.Node, error) {
	current := node
	for index, key := range path {
		next, err := uniqueYAMLMappingValue(current, key)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", strings.Join(path[:index+1], "."), err)
		}
		current = next
	}
	return current, nil
}

type yamlExpectation struct {
	path, tag, value string
}

func requireYAMLValues(node *yaml.Node, checks ...yamlExpectation) error {
	for _, check := range checks {
		target, err := yamlPath(node, strings.Split(check.path, ".")...)
		if err != nil {
			return err
		}
		if target.Kind != yaml.ScalarNode || target.Tag != check.tag || target.Value != check.value {
			return fmt.Errorf("%s must have tag=%s value=%q", check.path, check.tag, check.value)
		}
	}
	return nil
}

func exactYAMLMappingAt(node *yaml.Node, path string, keys ...string) error {
	target := node
	var err error
	if path != "" {
		target, err = yamlPath(node, strings.Split(path, ".")...)
		if err != nil {
			return err
		}
	}
	return exactYAMLMapping(target, path, keys...)
}

var fullActionSHAReference = regexp.MustCompile("^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)*@[0-9a-f]{40}$")

func validateBackendWorkflow(data []byte) error {
	root, err := singleYAMLMapping(data, ".github/workflows/verify-backend.yml")
	if err != nil {
		return err
	}
	for _, mapping := range []struct {
		path string
		keys []string
	}{
		{"", []string{"name", "on", "env", "permissions", "concurrency", "jobs"}},
		{"on", []string{"pull_request", "push"}},
		{"on.push", []string{"branches"}},
		{"env", []string{"GOTOOLCHAIN", "GOWORK"}},
		{"permissions", []string{"contents"}},
		{"concurrency", []string{"group", "cancel-in-progress"}},
		{"jobs", []string{"verify"}},
		{"jobs.verify", []string{"runs-on", "timeout-minutes", "steps"}},
	} {
		if err := exactYAMLMappingAt(root, mapping.path, mapping.keys...); err != nil {
			return err
		}
	}
	if err := requireYAMLValues(root,
		yamlExpectation{"name", "!!str", "Verify backend"},
		yamlExpectation{"on.pull_request", "!!null", ""},
		yamlExpectation{"env.GOTOOLCHAIN", "!!str", "local"},
		yamlExpectation{"env.GOWORK", "!!str", "off"},
		yamlExpectation{"permissions.contents", "!!str", "read"},
		yamlExpectation{"concurrency.group", "!!str", "backend-" + "$" + "{{ github.ref }}"},
		yamlExpectation{"concurrency.cancel-in-progress", "!!bool", "true"},
		yamlExpectation{"jobs.verify.runs-on", "!!str", "ubuntu-latest"},
		yamlExpectation{"jobs.verify.timeout-minutes", "!!int", "30"},
	); err != nil {
		return err
	}
	branches, _ := yamlPath(root, "on", "push", "branches")
	if branches.Kind != yaml.SequenceNode || len(branches.Content) != 1 ||
		branches.Content[0].Kind != yaml.ScalarNode || branches.Content[0].Tag != "!!str" || branches.Content[0].Value != "master" {
		return fmt.Errorf("on.push.branches must contain exactly master")
	}

	steps, _ := yamlPath(root, "jobs", "verify", "steps")
	if steps.Kind != yaml.SequenceNode || len(steps.Content) != 6 {
		return fmt.Errorf("verify.steps must contain exactly six blocking steps")
	}
	stepSpecs := []struct {
		keys   []string
		action string
	}{
		{[]string{"uses", "with"}, "actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5"},
		{[]string{"uses", "with"}, "actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff"},
		{[]string{"name", "shell", "run"}, ""},
		{[]string{"uses"}, "docker/setup-buildx-action@8d2750c68a42422c14e847fe6c8ac0403b4cbd6f"},
		{[]string{"name", "uses", "with"}, "docker/build-push-action@10e90e3645eae34f1e60eeb005ba3a3d33f178e8"},
		{[]string{"uses", "with"}, "actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02"},
	}
	for index, spec := range stepSpecs {
		step := steps.Content[index]
		if err := exactYAMLMapping(step, fmt.Sprintf("verify.steps[%d]", index), spec.keys...); err != nil {
			return err
		}
		if spec.action == "" {
			continue
		}
		uses, _ := yamlPath(step, "uses")
		if uses.Kind != yaml.ScalarNode || uses.Tag != "!!str" || uses.Value != spec.action ||
			!fullActionSHAReference.MatchString(uses.Value) {
			return fmt.Errorf("verify.steps[%d].uses must equal its approved 40-hex action SHA", index)
		}
	}
	for _, mapping := range []struct {
		step int
		keys []string
	}{
		{0, []string{"persist-credentials"}},
		{1, []string{"go-version", "cache"}},
		{4, []string{"context", "push", "tags", "outputs", "build-args"}},
		{5, []string{"name", "path", "if-no-files-found", "retention-days"}},
	} {
		with, _ := yamlPath(steps.Content[mapping.step], "with")
		if err := exactYAMLMapping(with, fmt.Sprintf("verify.steps[%d].with", mapping.step), mapping.keys...); err != nil {
			return err
		}
	}
	sha := "$" + "{{ github.sha }}"
	stepValues := map[int][]yamlExpectation{
		0: {{"with.persist-credentials", "!!bool", "false"}},
		1: {{"with.go-version", "!!str", "1.26.5"}, {"with.cache", "!!bool", "false"}},
		2: {{"shell", "!!str", "pwsh"}, {"run", "!!str", "./scripts/verify-go-clean.ps1"}},
		4: {
			{"with.context", "!!str", "."},
			{"with.push", "!!bool", "false"},
			{"with.tags", "!!str", "admin-go-backend:" + sha},
			{"with.outputs", "!!str", "type=docker,dest=/tmp/admin-go-backend.tar"},
			{"with.build-args", "!!str", "BUILD_REVISION=" + sha},
		},
		5: {
			{"with.name", "!!str", "admin-go-backend-" + sha},
			{"with.path", "!!str", "/tmp/admin-go-backend.tar"},
			{"with.if-no-files-found", "!!str", "error"},
			{"with.retention-days", "!!int", "7"},
		},
	}
	for step, checks := range stepValues {
		if err := requireYAMLValues(steps.Content[step], checks...); err != nil {
			return fmt.Errorf("verify.steps[%d]: %w", step, err)
		}
	}
	return nil
}

func validateDependabotConfig(data []byte) error {
	root, err := singleYAMLMapping(data, ".github/dependabot.yml")
	if err != nil {
		return err
	}
	if err := exactYAMLMapping(root, "dependabot", "version", "updates"); err != nil {
		return err
	}
	if err := requireYAMLValues(root, yamlExpectation{"version", "!!int", "2"}); err != nil {
		return err
	}
	updates, _ := yamlPath(root, "updates")
	if updates.Kind != yaml.SequenceNode || len(updates.Content) != 3 {
		return fmt.Errorf("dependabot.updates must contain exactly three entries")
	}
	seen := make(map[string]bool, 3)
	for index, update := range updates.Content {
		path := fmt.Sprintf("dependabot.updates[%d]", index)
		if err := exactYAMLMapping(update, path, "package-ecosystem", "directory", "schedule", "open-pull-requests-limit"); err != nil {
			return err
		}
		if err := exactYAMLMappingAt(update, "schedule", "interval"); err != nil {
			return err
		}
		if err := requireYAMLValues(update,
			yamlExpectation{"directory", "!!str", "/"},
			yamlExpectation{"schedule.interval", "!!str", "weekly"},
			yamlExpectation{"open-pull-requests-limit", "!!int", "5"},
		); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		ecosystem, _ := yamlPath(update, "package-ecosystem")
		if ecosystem.Kind != yaml.ScalarNode || ecosystem.Tag != "!!str" ||
			(ecosystem.Value != "gomod" && ecosystem.Value != "docker" && ecosystem.Value != "github-actions") ||
			seen[ecosystem.Value] {
			return fmt.Errorf("%s has invalid or duplicate package ecosystem", path)
		}
		seen[ecosystem.Value] = true
	}
	return nil
}

type dockerInstruction struct {
	keyword string
	value   string
}

type dockerStage struct {
	name, base   string
	instructions []dockerInstruction
}

type parsedDockerfile struct {
	globals []dockerInstruction
	stages  []*dockerStage
}

func parseDockerfile(data []byte) (*parsedDockerfile, error) {
	var instructions []dockerInstruction
	var pending strings.Builder
	for _, raw := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			name, _, directive := strings.Cut(strings.TrimSpace(strings.TrimPrefix(line, "#")), "=")
			switch strings.ToLower(strings.TrimSpace(name)) {
			case "syntax", "escape", "check":
				if directive {
					return nil, fmt.Errorf("Dockerfile parser directive %q is forbidden", strings.TrimSpace(name))
				}
			}
			continue
		}
		if pending.Len() != 0 {
			pending.WriteByte(' ')
		}
		continued := strings.HasSuffix(line, "\\")
		pending.WriteString(strings.TrimSpace(strings.TrimSuffix(line, "\\")))
		if continued {
			continue
		}
		logical := pending.String()
		pending.Reset()
		fields := strings.Fields(logical)
		if len(fields) != 0 {
			instructions = append(instructions, dockerInstruction{
				keyword: strings.ToUpper(fields[0]),
				value:   strings.TrimSpace(logical[len(fields[0]):]),
			})
		}
	}
	if pending.Len() != 0 {
		return nil, fmt.Errorf("Dockerfile has an unterminated continuation")
	}

	model := &parsedDockerfile{}
	var current *dockerStage
	for _, instruction := range instructions {
		if instruction.keyword != "FROM" {
			if current == nil {
				model.globals = append(model.globals, instruction)
			} else {
				current.instructions = append(current.instructions, instruction)
			}
			continue
		}
		fields := strings.Fields(instruction.value)
		for len(fields) > 0 && strings.HasPrefix(fields[0], "--") {
			fields = fields[1:]
		}
		if len(fields) != 3 || !strings.EqualFold(fields[1], "AS") {
			return nil, fmt.Errorf("every Dockerfile FROM must name its stage")
		}
		stage := &dockerStage{name: strings.ToLower(fields[2]), base: fields[0]}
		model.stages = append(model.stages, stage)
		current = stage
	}
	if len(model.stages) == 0 {
		return nil, fmt.Errorf("Dockerfile has no FROM stage")
	}
	return model, nil
}

func dockerInstructions(lines ...string) []dockerInstruction {
	instructions := make([]dockerInstruction, 0, len(lines))
	for _, line := range lines {
		keyword, value, _ := strings.Cut(line, " ")
		instructions = append(instructions, dockerInstruction{keyword: keyword, value: value})
	}
	return instructions
}

func dockerInstructionSequenceProblems(scope string, got, want []dockerInstruction) []string {
	var problems []string
	if len(got) != len(want) {
		problems = append(problems, fmt.Sprintf("Dockerfile %s has %d instructions, want exactly %d", scope, len(got), len(want)))
	}
	for index := 0; index < len(got) && index < len(want); index++ {
		gotValue := strings.Join(strings.Fields(got[index].value), " ")
		wantValue := strings.Join(strings.Fields(want[index].value), " ")
		if got[index].keyword != want[index].keyword || gotValue != wantValue {
			problems = append(problems, fmt.Sprintf(
				"Dockerfile %s instruction %d is %q, want %q",
				scope,
				index+1,
				got[index].keyword+" "+gotValue,
				want[index].keyword+" "+wantValue,
			))
		}
	}
	return problems
}

func dockerBuildIntegrityProblems(data []byte) []string {
	problems := append([]string(nil), dockerfileSecureFoundationProblems(data)...)
	model, err := parseDockerfile(data)
	if err != nil {
		return append(problems, err.Error())
	}

	problems = append(problems, dockerInstructionSequenceProblems("global instructions", model.globals, dockerInstructions(
		"ARG GO_BUILD_IMAGE=golang:1.26.5-bookworm",
		"ARG GO_RUNTIME_IMAGE=debian:bookworm-slim",
		"ARG GO_MODULE_PROXY=https://goproxy.io,https://proxy.golang.org,direct",
		"ARG BUILD_REVISION=unknown",
	))...)

	expectedStages := []struct {
		name, base   string
		instructions []dockerInstruction
	}{
		{
			name: "test",
			base: "${GO_BUILD_IMAGE}",
			instructions: dockerInstructions(
				"ARG GO_MODULE_PROXY",
				"WORKDIR /src",
				"ENV CGO_ENABLED=0",
				"ENV GOFLAGS=-trimpath",
				"ENV GOPROXY=${GO_MODULE_PROXY}",
				"ENV GOSUMDB=sum.golang.org",
				"ENV GOTOOLCHAIN=local",
				"ENV GOWORK=off",
				"COPY go.mod go.sum ./",
				"RUN --mount=type=cache,target=/go/pkg/mod go mod download",
				"COPY . .",
				"RUN --mount=type=cache,target=/root/.cache/go-build go test ./...",
			),
		},
		{
			name: "build",
			base: "test",
			instructions: dockerInstructions(
				"RUN --mount=type=cache,target=/root/.cache/go-build go build -ldflags=\"-s -w\" -o /out/admin-api ./cmd/admin-api && go build -ldflags=\"-s -w\" -o /out/admin-worker ./cmd/admin-worker",
			),
		},
		{
			name: "runtime",
			base: "${GO_RUNTIME_IMAGE}",
			instructions: dockerInstructions(
				"ARG BUILD_REVISION",
				"LABEL org.opencontainers.image.revision=\"${BUILD_REVISION}\"",
				"RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata curl && rm -rf /var/lib/apt/lists/* && groupadd -r -g 10001 app && useradd -r -u 10001 -g app -d /app -s /usr/sbin/nologin app",
				"WORKDIR /app",
				"RUN mkdir -p /app/runtime/logs /app/runtime/cert/alipay /app/exports && chown -R app:app /app",
				"COPY --chown=app:app --from=build /out/admin-api /app/admin-api",
				"COPY --chown=app:app --from=build /out/admin-worker /app/admin-worker",
				"ENV APP_ENV=production",
				"ENV HTTP_ADDR=:8080",
				"ENV TZ=Asia/Shanghai",
				"EXPOSE 8080",
				"USER app",
				"CMD [\"/app/admin-api\"]",
			),
		},
	}
	if len(model.stages) != len(expectedStages) {
		problems = append(problems, fmt.Sprintf("Dockerfile has %d stages, want exactly %d", len(model.stages), len(expectedStages)))
	}
	for index := 0; index < len(model.stages) && index < len(expectedStages); index++ {
		got, want := model.stages[index], expectedStages[index]
		if got.name != want.name || got.base != want.base {
			problems = append(problems, fmt.Sprintf(
				"Dockerfile stage %d is %s from %s, want %s from %s",
				index+1, got.name, got.base, want.name, want.base,
			))
		}
		problems = append(problems, dockerInstructionSequenceProblems("stage "+want.name, got.instructions, want.instructions)...)
	}
	return problems
}
func composeSecureFoundationProblems(data []byte) []string {
	const expected = "docker.m.daocloud.io/library/golang:1.26.5-bookworm"
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return []string{fmt.Sprintf("deploy/docker-first/docker-compose.yml is invalid YAML: %v", err)}
	}
	var extraDocument yaml.Node
	if err := decoder.Decode(&extraDocument); err != io.EOF {
		if err != nil {
			return []string{fmt.Sprintf("deploy/docker-first/docker-compose.yml is invalid YAML: %v", err)}
		}
		return []string{"deploy/docker-first/docker-compose.yml must contain one YAML document"}
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return []string{"deploy/docker-first/docker-compose.yml must contain one YAML document"}
	}

	var problems []string
	for _, service := range []string{"admin-api", "admin-worker"} {
		node := document.Content[0]
		segments := []string{"services", service, "build", "args", "GO_BUILD_IMAGE"}
		for index, segment := range segments {
			next, err := uniqueYAMLMappingValue(node, segment)
			if err != nil {
				problems = append(problems, fmt.Sprintf("deploy/docker-first/docker-compose.yml %s: %v", strings.Join(segments[:index+1], "."), err))
				node = nil
				break
			}
			node = next
		}
		if node == nil {
			continue
		}
		path := strings.Join(segments, ".")
		if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
			problems = append(problems, fmt.Sprintf("deploy/docker-first/docker-compose.yml %s must be a string scalar", path))
		} else if node.Value != expected {
			problems = append(problems, fmt.Sprintf("deploy/docker-first/docker-compose.yml %s=%q, want %q", path, node.Value, expected))
		}
	}
	return problems
}

func markdownHTMLCommentLine(line string, inBlockComment *bool) bool {
	if *inBlockComment {
		if strings.Contains(line, "-->") {
			*inBlockComment = false
		}
		return true
	}

	withoutIndent := strings.TrimLeft(line, " ")
	if len(line)-len(withoutIndent) > 3 || !strings.HasPrefix(withoutIndent, "<!--") {
		return false
	}
	if !strings.Contains(withoutIndent[len("<!--"):], "-->") {
		*inBlockComment = true
	}
	return true
}

func markdownFence(line string) (byte, int, string, bool) {
	withoutIndent := strings.TrimLeft(line, " ")
	if len(line)-len(withoutIndent) > 3 || withoutIndent == "" {
		return 0, 0, "", false
	}
	marker := withoutIndent[0]
	if marker != '`' && marker != '~' {
		return 0, 0, "", false
	}
	length := 0
	for length < len(withoutIndent) && withoutIndent[length] == marker {
		length++
	}
	if length < 3 {
		return 0, 0, "", false
	}
	rest := withoutIndent[length:]
	if marker == '`' && strings.Contains(rest, "`") {
		return 0, 0, "", false
	}
	return marker, length, rest, true
}

func readmeSecureFoundationProblems(data []byte) []string {
	const expected = "| Language | Go `1.26.5` |"

	var rows []string
	inHTMLBlockComment := false
	var fenceMarker byte
	fenceLength := 0
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSuffix(rawLine, "\r")
		if fenceMarker != 0 {
			if marker, length, rest, ok := markdownFence(line); ok && marker == fenceMarker && length >= fenceLength && strings.TrimSpace(rest) == "" {
				fenceMarker = 0
				fenceLength = 0
			}
			continue
		}
		if markdownHTMLCommentLine(line, &inHTMLBlockComment) {
			continue
		}
		if marker, length, _, ok := markdownFence(line); ok {
			fenceMarker = marker
			fenceLength = length
			continue
		}
		if strings.HasPrefix(line, "| Language |") {
			rows = append(rows, line)
		}
	}

	var problems []string
	if inHTMLBlockComment {
		problems = append(problems, "README.md has an unterminated HTML comment")
	}
	if fenceMarker != 0 {
		problems = append(problems, "README.md has an unterminated fenced code block")
	}
	if len(rows) != 1 {
		problems = append(problems, fmt.Sprintf("README.md must contain exactly one active Language table row, found %d", len(rows)))
	} else if rows[0] != expected {
		problems = append(problems, fmt.Sprintf("README.md active Language row is %q, want %q", rows[0], expected))
	}
	return problems
}

func TestSecureFoundationGuardRejectsProtectedReplacements(t *testing.T) {
	for name, testCase := range map[string]struct {
		goMod string
		want  string
	}{
		"quic single line with original version": {
			goMod: "replace github.com/quic-go/quic-go v0.59.1 => github.com/quic-go/quic-go v0.59.0\r\n",
			want:  "github.com/quic-go/quic-go",
		},
		"image block without original version": {
			goMod: "replace (\r\n\tgolang.org/x/image => golang.org/x/image v0.42.0\r\n)\r\n",
			want:  "golang.org/x/image",
		},
		"quic no-space block": {
			goMod: "replace(\r\n\tgithub.com/quic-go/quic-go => github.com/quic-go/quic-go v0.59.0\r\n)\r\n",
			want:  "github.com/quic-go/quic-go",
		},
		"quic quoted single line": {
			goMod: "replace \"github.com/quic-go/quic-go\" => github.com/quic-go/quic-go v0.59.0\r\n",
			want:  "github.com/quic-go/quic-go",
		},
		"image quoted block": {
			goMod: "replace (\r\n\t\"golang.org/x/image\" v0.43.0 => golang.org/x/image v0.42.0\r\n)\r\n",
			want:  "golang.org/x/image",
		},
		"quic malformed quoted token": {
			goMod: "replace \"github.com/quic-go/quic-go\\q\" => github.com/quic-go/quic-go v0.59.0\r\n",
			want:  "github.com/quic-go/quic-go",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := protectedGoModReplacements([]byte(testCase.goMod))
			if len(got) != 1 || got[0] != testCase.want {
				t.Fatalf("protected replacements = %v, want [%s]", got, testCase.want)
			}
		})
	}
}

func TestSecureGoFoundationVersions(t *testing.T) {
	for _, problem := range secureGoFoundationProblems(backendRoot(t)) {
		t.Error(problem)
	}
}

func secureGoFoundationProblems(root string) []string {
	var problems []string
	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		problems = append(problems, fmt.Sprintf("read go.mod: %v", err))
	} else {
		for key, want := range map[string]string{
			"go":                         "1.26.5",
			"github.com/quic-go/quic-go": "v0.59.1",
			"golang.org/x/image":         "v0.43.0",
		} {
			got, ok := goModValue(goMod, key)
			if !ok {
				problems = append(problems, fmt.Sprintf("go.mod value %q not found", key))
			} else if got != want {
				problems = append(problems, fmt.Sprintf("go.mod %s=%s, want %s", key, got, want))
			}
		}
		if toolchain, exists := goModTopLevelDirectiveValue(goMod, "toolchain"); exists {
			problems = append(problems, fmt.Sprintf("go.mod toolchain directive %q is forbidden", toolchain))
		}
		for _, modulePath := range protectedGoModReplacements(goMod) {
			problems = append(problems, fmt.Sprintf("go.mod replace directive for protected module %s is forbidden", modulePath))
		}
	}
	if _, err := os.Lstat(filepath.Join(root, "go.work")); err == nil {
		problems = append(problems, "root go.work is forbidden")
	} else if !os.IsNotExist(err) {
		problems = append(problems, fmt.Sprintf("inspect root go.work: %v", err))
	}

	for _, surface := range []struct {
		rel      string
		validate func([]byte) []string
	}{
		{rel: "Dockerfile", validate: dockerfileSecureFoundationProblems},
		{rel: "deploy/docker-first/docker-compose.yml", validate: composeSecureFoundationProblems},
		{rel: "README.md", validate: readmeSecureFoundationProblems},
	} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(surface.rel)))
		if err != nil {
			problems = append(problems, fmt.Sprintf("read %s: %v", surface.rel, err))
			continue
		}
		problems = append(problems, surface.validate(data)...)
	}
	return problems
}

func writeSecureGoFoundationFixture(t *testing.T, overrides map[string]string) string {
	t.Helper()
	files := map[string]string{
		"go.mod":                                 "module example.com/secure-foundation-fixture\n\ngo 1.26.5\n\nrequire (\n\tgithub.com/quic-go/quic-go v0.59.1\n\tgolang.org/x/image v0.43.0\n)\n",
		"Dockerfile":                             "# secure build fixture\n\nARG GO_BUILD_IMAGE=golang:1.26.5-bookworm\nFROM --platform=$BUILDPLATFORM ${GO_BUILD_IMAGE} AS task-6-build\n",
		"deploy/docker-first/docker-compose.yml": "# secure compose fixture\nx-build-decoy:\n  GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.1-bookworm\nservices:\n  admin-api:\n    build:\n      args:\n        GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.5-bookworm\n  admin-worker:\n    build:\n      args:\n        GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.5-bookworm\n",
		"README.md":                              "## Technology\n\n<!-- | Language | Go `1.26.1` | -->\n```markdown\n| Language | Go `1.26.1` |\n```\n| Type | Choice |\n| --- | --- |\n| Language | Go `1.26.5` |\n",
	}
	for rel, content := range overrides {
		files[rel] = content
	}

	root := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		content = strings.ReplaceAll(content, "\n", "\r\n")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestSecureGoFoundationValidatorRejectsSemanticBuildSurfaceDecoys(t *testing.T) {
	const compose = "deploy/docker-first/docker-compose.yml"
	testCases := []struct{ name, rel, content string }{
		{"go.mod toolchain directive", "go.mod", "module example.com/secure-foundation-fixture\n\ngo 1.26.5\ntoolchain go1.27rc1\n\nrequire (\n\tgithub.com/quic-go/quic-go v0.59.1\n\tgolang.org/x/image v0.43.0\n)\n"},
		{"root go.work override", "go.work", "go 1.26.5\n\nuse .\n\nreplace github.com/quic-go/quic-go => ./local/quic-go\n"},
		{"Dockerfile hard-coded old first build stage", "Dockerfile", "ARG GO_BUILD_IMAGE=golang:1.26.5-bookworm\nFROM golang:1.26.1-bookworm AS build\n"},
		{"Compose extension decoys", compose,
			"x-admin-api-build:\n  GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.5-bookworm\n" +
				"x-admin-worker-build:\n  GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.5-bookworm\n" +
				"services:\n  admin-api:\n    build:\n      args:\n        GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.1-bookworm\n" +
				"  admin-worker:\n    build:\n      args:\n        GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.1-bookworm\n"},
		{"README fenced target decoy", "README.md", "## Technology\n\n| Type | Choice |\n| --- | --- |\n| Language | Go `1.26.1` |\n\n```markdown\n| Language | Go `1.26.5` |\n```\n"},
		{"README invalid backtick fence info decoy", "README.md", "```markdown`invalid\n| Language | Go `1.26.1` |\n```\n| Language | Go `1.26.5` |\n"},
		{"Dockerfile ARG after first FROM", "Dockerfile", "FROM ${GO_BUILD_IMAGE} AS build\nARG GO_BUILD_IMAGE=golang:1.26.5-bookworm\n"},
		{"Dockerfile duplicate alternate ARG", "Dockerfile", "ARG GO_BUILD_IMAGE=golang:1.26.5-bookworm\nARG GO_BUILD_IMAGE=golang:1.26.1-bookworm\nFROM ${GO_BUILD_IMAGE} AS build\n"},
		{"Compose missing worker hidden by extension", compose,
			"x-worker-build:\n  GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.5-bookworm\n" +
				"services:\n  admin-api:\n    build:\n      args:\n        GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.5-bookworm\n"},
		{"Compose duplicate target key", compose,
			"services:\n  admin-api:\n    build:\n      args:\n" +
				"        GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.5-bookworm\n" +
				"        GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.5-bookworm\n" +
				"  admin-worker:\n    build:\n      args:\n        GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.5-bookworm\n"},
		{"Compose aliased target key override", compose,
			"services:\n  admin-api:\n    build:\n      args:\n" +
				"        &imageKey GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.5-bookworm\n" +
				"        *imageKey: docker.m.daocloud.io/library/golang:1.26.1-bookworm\n" +
				"  admin-worker:\n    build:\n      args:\n        GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.5-bookworm\n"},
		{"Compose dotted top-level key decoys", compose,
			"services.admin-api.build.args.GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.5-bookworm\n" +
				"services.admin-worker.build.args.GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.5-bookworm\n"},
		{"Compose additional document decoy", compose,
			"services:\n  admin-api:\n    build:\n      args:\n        GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.5-bookworm\n" +
				"  admin-worker:\n    build:\n      args:\n        GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.5-bookworm\n" +
				"---\nservices:\n  admin-api:\n    build:\n      args:\n        GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.1-bookworm\n" +
				"  admin-worker:\n    build:\n      args:\n        GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.1-bookworm\n"},
		{"Compose alias leaf", compose,
			"x-go-image: &go_image docker.m.daocloud.io/library/golang:1.26.5-bookworm\n" +
				"services:\n  admin-api:\n    build:\n      args:\n        GO_BUILD_IMAGE: *go_image\n" +
				"  admin-worker:\n    build:\n      args:\n        GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.5-bookworm\n"},
		{"Compose sequence leaf", compose,
			"services:\n  admin-api:\n    build:\n      args:\n        GO_BUILD_IMAGE: [docker.m.daocloud.io/library/golang:1.26.5-bookworm]\n" +
				"  admin-worker:\n    build:\n      args:\n        GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.5-bookworm\n"},
		{"README duplicate active Language rows", "README.md", "| Language | Go `1.26.5` |\n| Language | Go `1.26.1` |\n"},
		{"README tilde fenced target decoy", "README.md", "| Language | Go `1.26.1` |\n\n~~~markdown\n| Language | Go `1.26.5` |\n~~~\n"},
		{"README indented code target decoy", "README.md", "| Language | Go `1.26.1` |\n\n    | Language | Go `1.26.5` |\n"},
		{"README indented-code comment marker decoy", "README.md", "| Language | Go `1.26.5` |\n\n    <!--\n| Language | Go `1.26.1` |\n    -->\n"},
		{"README inline-code comment marker decoy", "README.md", "| Language | Go `1.26.5` |\n\n`<!--`\n| Language | Go `1.26.1` |\n`-->`\n"},
		{"README mixed indent target decoy", "README.md", " \t| Language | Go `1.26.5` |\n"},
		{"README list fenced target decoy", "README.md", "- ```markdown\n  | Language | Go `1.26.5` |\n- ```\n"},
		{"README indented top-level fence target decoy", "README.md", "   ```markdown\n| Language | Go `1.26.5` |\n   ```\n"},
		{"README multiline comment target decoy", "README.md", "| Language | Go `1.26.1` |\n<!--\n| Language | Go `1.26.5` |\n-->\n"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			root := writeSecureGoFoundationFixture(t, map[string]string{testCase.rel: testCase.content})
			problems := strings.Join(secureGoFoundationProblems(root), "\n")
			if problems == "" {
				t.Fatal("secure foundation validator accepted the semantic decoy")
			}
			if !strings.Contains(problems, testCase.rel) {
				t.Fatalf("secure foundation validator failed without identifying %s:\n%s", testCase.rel, problems)
			}
		})
	}
}

func TestSecureGoFoundationValidatorAcceptsValidCRLFRoot(t *testing.T) {
	const compose = "deploy/docker-first/docker-compose.yml"
	for name, overrides := range map[string]map[string]string{
		"default": nil,
		"valid YAML indentation": {
			compose: "services:\n   admin-api:\n      build:\n         args:\n            GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.5-bookworm\n" +
				"   admin-worker:\n      build:\n         args:\n            GO_BUILD_IMAGE: docker.m.daocloud.io/library/golang:1.26.5-bookworm\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := writeSecureGoFoundationFixture(t, overrides)
			if problems := secureGoFoundationProblems(root); len(problems) != 0 {
				t.Fatalf("valid CRLF secure foundation fixture was rejected:\n%s", strings.Join(problems, "\n"))
			}
		})
	}
}

func readBackendArchitectureFile(t *testing.T, rel string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(backendRoot(t), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestBackendCIContract(t *testing.T) {
	valid := readBackendArchitectureFile(t, ".github/workflows/verify-backend.yml")
	if err := validateBackendWorkflow(valid); err != nil {
		t.Fatal(err)
	}
	root, err := singleYAMLMapping(valid, ".github/workflows/verify-backend.yml")
	if err != nil {
		t.Fatal(err)
	}
	steps, err := yamlPath(root, "jobs", "verify", "steps")
	if err != nil {
		t.Fatal(err)
	}
	checkoutWith, err := yamlPath(steps.Content[0], "with")
	if err != nil {
		t.Fatal("checkout must set persist-credentials: false")
	}
	if err := exactYAMLMapping(checkoutWith, "verify.steps[0].with", "persist-credentials"); err != nil {
		t.Fatal(err)
	}
	if err := requireYAMLValues(checkoutWith, yamlExpectation{"persist-credentials", "!!bool", "false"}); err != nil {
		t.Fatal(err)
	}
	workflowEnv, err := yamlPath(root, "env")
	if err != nil {
		t.Fatal("workflow must lock GOTOOLCHAIN=local and GOWORK=off")
	}
	if err := exactYAMLMapping(workflowEnv, "workflow.env", "GOTOOLCHAIN", "GOWORK"); err != nil {
		t.Fatal(err)
	}
	if err := requireYAMLValues(
		workflowEnv,
		yamlExpectation{"GOTOOLCHAIN", "!!str", "local"},
		yamlExpectation{"GOWORK", "!!str", "off"},
	); err != nil {
		t.Fatal(err)
	}
	for name, fixture := range map[string][]byte{
		"unpinned action":     bytes.Replace(valid, []byte("actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5"), []byte("actions/checkout@v4"), 1),
		"image push enabled":  bytes.Replace(valid, []byte("          push: false"), []byte("          push: true"), 1),
		"automatic toolchain": bytes.Replace(valid, []byte("GOTOOLCHAIN: local"), []byte("GOTOOLCHAIN: auto"), 1),
		"workspace enabled":   bytes.Replace(valid, []byte("GOWORK: \"off\""), []byte("GOWORK: ./go.work"), 1),
		"workspace lock missing": bytes.Replace(
			valid,
			[]byte("  GOWORK: \"off\"\n"),
			nil,
			1,
		),
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateBackendWorkflow(fixture); err == nil {
				t.Fatal("workflow validator accepted an unsafe fixture")
			}
		})
	}
}

func TestDependabotContract(t *testing.T) {
	valid := readBackendArchitectureFile(t, ".github/dependabot.yml")
	if err := validateDependabotConfig(valid); err != nil {
		t.Fatal(err)
	}
	for name, fixture := range map[string][]byte{
		"daily schedule": bytes.Replace(valid, []byte("interval: weekly"), []byte("interval: daily"), 1),
		"wrong PR limit": bytes.Replace(valid, []byte("open-pull-requests-limit: 5"), []byte("open-pull-requests-limit: 6"), 1),
		"auto merge key": bytes.Replace(valid, []byte("    open-pull-requests-limit: 5"), []byte("    open-pull-requests-limit: 5\n    auto-merge: true"), 1),
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateDependabotConfig(fixture); err == nil {
				t.Fatal("Dependabot validator accepted an unsafe fixture")
			}
		})
	}
}

func TestDockerBuildIntegrity(t *testing.T) {
	valid := readBackendArchitectureFile(t, "Dockerfile")
	valid = bytes.ReplaceAll(valid, []byte("\r\n"), []byte("\n"))
	for _, problem := range dockerBuildIntegrityProblems(valid) {
		t.Error(problem)
	}
	replace := func(old, replacement string) []byte {
		t.Helper()
		fixture := bytes.Replace(valid, []byte(old), []byte(replacement), 1)
		if bytes.Equal(fixture, valid) {
			t.Fatalf("Dockerfile fixture source not found: %q", old)
		}
		return fixture
	}
	untested := replace(
		"FROM ${GO_RUNTIME_IMAGE} AS runtime",
		"FROM ${GO_BUILD_IMAGE} AS untested\n\nWORKDIR /src\nCOPY . .\nRUN --mount=type=cache,target=/root/.cache/go-build \\\n    go build -o /out/admin-api ./cmd/admin-api && \\\n    go build -o /out/admin-worker ./cmd/admin-worker\n\nFROM ${GO_RUNTIME_IMAGE} AS runtime",
	)
	untested = bytes.Replace(
		untested,
		[]byte("COPY --chown=app:app --from=build /out/admin-worker /app/admin-worker"),
		[]byte("COPY --chown=app:app --from=build /out/admin-worker /app/admin-worker\nCOPY --chown=app:app --from=untested /out/admin-api /app/admin-api\nCOPY --chown=app:app --from=untested /out/admin-worker /app/admin-worker"),
		1,
	)
	maliciousSyntax := append([]byte("# SyNtAx = attacker.example/custom-frontend:latest\n"), valid...)
	for name, fixture := range map[string][]byte{
		"build bypasses test": bytes.Replace(valid, []byte("FROM test AS build"), []byte("FROM $"+"{GO_BUILD_IMAGE} AS build"), 1),
		"GOSUMDB disabled":    bytes.Replace(valid, []byte("ENV GOSUMDB=sum.golang.org"), []byte("ENV GOSUMDB=off"), 1),
		"echo test decoy": replace(
			"RUN --mount=type=cache,target=/root/.cache/go-build \\\n    go test ./...",
			"RUN --mount=type=cache,target=/root/.cache/go-build echo \"go test ./...\"",
		),
		"runtime copies untested binaries": untested,
		"fake runtime revision metadata": replace(
			"ARG BUILD_REVISION\n\nLABEL org.opencontainers.image.revision=\"${BUILD_REVISION}\"",
			"ARG NOT_BUILD_REVISION\n\nLABEL fake=\"org.opencontainers.image.revision=${BUILD_REVISION}\"",
		),
		"duplicate GOSUMDB override": replace(
			"ENV GOSUMDB=sum.golang.org",
			"ENV GOSUMDB=sum.golang.org GOSUMDB=off",
		),
		"GOPRIVATE bypass": replace(
			"ENV GOPROXY=${GO_MODULE_PROXY}\nENV GOSUMDB=sum.golang.org",
			"ENV GOPROXY=${GO_MODULE_PROXY}\nENV GOPRIVATE=*\nENV GOSUMDB=sum.golang.org",
		),
		"test altering GOFLAGS": replace(
			"ENV GOFLAGS=-trimpath",
			"ENV GOFLAGS=\"-trimpath -run=DO_NOT_MATCH\"",
		),
		"SHELL wrapper": replace(
			"COPY . .\nRUN --mount=type=cache,target=/root/.cache/go-build",
			"COPY . .\nSHELL [\"/bin/sh\", \"-c\", \"true #\"]\nRUN --mount=type=cache,target=/root/.cache/go-build",
		),
		"build mutation after compilation": replace(
			"FROM ${GO_RUNTIME_IMAGE} AS runtime",
			"RUN printf tampered > /out/admin-api\n\nFROM ${GO_RUNTIME_IMAGE} AS runtime",
		),
		"runtime overwrite after copies": replace(
			"COPY --chown=app:app --from=build /out/admin-worker /app/admin-worker",
			"COPY --chown=app:app --from=build /out/admin-worker /app/admin-worker\nRUN cp /bin/false /app/admin-api",
		),
		"duplicate revision label": replace(
			"LABEL org.opencontainers.image.revision=\"${BUILD_REVISION}\"",
			"LABEL org.opencontainers.image.revision=\"${BUILD_REVISION}\"\nLABEL org.opencontainers.image.revision=\"override\"",
		),
		"malicious syntax directive": maliciousSyntax,
		"automatic Go execution context": replace(
			"ENV GOTOOLCHAIN=local\nENV GOWORK=off",
			"ENV GOTOOLCHAIN=auto\nENV GOWORK=/tmp/go.work",
		),
	} {
		t.Run(name, func(t *testing.T) {
			if problems := dockerBuildIntegrityProblems(fixture); len(problems) == 0 {
				t.Fatal("Dockerfile validator accepted an unsafe fixture")
			}
		})
	}
}

func TestAsynqmonChecksumMatchesTransparencyLog(t *testing.T) {
	root := backendRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	if !containsExactLine(data, asynqmonV072Sum) {
		t.Fatalf("go.sum must contain verified sum %q", asynqmonV072Sum)
	}
}

func TestBackendVerificationEntrypointsExist(t *testing.T) {
	root := backendRoot(t)
	for _, rel := range []string{
		"scripts/verify-go-clean.ps1",
		"scripts/verify-backend.ps1",
	} {
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Errorf("backend verification entrypoint %s: %v", rel, err)
			continue
		}
		if !info.Mode().IsRegular() {
			t.Errorf("backend verification entrypoint %s must be a regular file", rel)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("backend verification entrypoint %s must not be empty", rel)
		}
	}
}

func TestBackendVerificationDocumentationCoversSupportedPowerShellHosts(t *testing.T) {
	root := backendRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}

	for _, command := range []string{
		"pwsh -NoProfile -File scripts/verify-backend.ps1",
		"powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/verify-backend.ps1",
		"pwsh -NoProfile -File scripts/verify-go-clean.ps1",
		"powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/verify-go-clean.ps1",
		"pwsh -NoProfile -File scripts/verify-go-clean.ps1 -KeepScratch",
		"powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/verify-go-clean.ps1 -KeepScratch",
	} {
		if !strings.Contains(string(data), command) {
			t.Errorf("README.md must document %q", command)
		}
	}
}

func TestCleanVerificationRestoresGOMODCACHEState(t *testing.T) {
	powerShell := requireWindowsPowerShell(t)
	root := backendRoot(t)
	sandbox := t.TempDir()
	cleanScript := filepath.Join(sandbox, "scripts", "verify-go-clean.ps1")
	copyTestFile(t, filepath.Join(root, "scripts", "verify-go-clean.ps1"), cleanScript)
	fakeBin := filepath.Join(sandbox, "fake-bin")
	writeFakeGoCommand(t, fakeBin)

	harness := filepath.Join(sandbox, "verify-gomodcache-state.ps1")
	harnessSource := `[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)]
  [string]$ScriptPath,
  [Parameter(Mandatory = $true)]
  [string]$FakeBin,
  [Parameter(Mandatory = $true)]
  [ValidateSet("unset", "empty", "nonempty")]
  [string]$State,
  [switch]$ExpectFailure
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if ($null -eq ("AdminGoVerificationTestNativeEnvironment" -as [type])) {
  Add-Type -TypeDefinition @'
using System.Runtime.InteropServices;

public static class AdminGoVerificationTestNativeEnvironment
{
    [DllImport("kernel32.dll", EntryPoint = "SetEnvironmentVariableW", CharSet = CharSet.Unicode, SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    public static extern bool SetEnvironmentVariable(string name, string value);
}
'@
}

$env:Path = $FakeBin + [IO.Path]::PathSeparator + $env:Path
if ($ExpectFailure) {
  $env:FAKE_GO_EXIT = "23"
} else {
  $env:FAKE_GO_EXIT = "0"
}

switch ($State) {
  "unset" {
    [Environment]::SetEnvironmentVariable("GOMODCACHE", $null, [EnvironmentVariableTarget]::Process)
    $expectedPresent = $false
    $expectedValue = $null
  }
  "empty" {
    if (-not [AdminGoVerificationTestNativeEnvironment]::SetEnvironmentVariable("GOMODCACHE", [string]::Empty)) {
      throw "SetEnvironmentVariableW failed with Win32 error $([Runtime.InteropServices.Marshal]::GetLastWin32Error())."
    }
    $expectedPresent = $true
    $expectedValue = ""
  }
  "nonempty" {
    $expectedValue = "admin-go-original-modcache"
    [Environment]::SetEnvironmentVariable("GOMODCACHE", $expectedValue, [EnvironmentVariableTarget]::Process)
    $expectedPresent = $true
  }
}

$failure = $null
try {
  & $ScriptPath
} catch {
  $failure = $_
}

if ($ExpectFailure) {
  if ($null -eq $failure -or -not $failure.Exception.Message.Contains("exit code 23")) {
    throw "expected fake go exit code 23"
  }
} elseif ($null -ne $failure) {
  throw $failure
}

$environment = [Environment]::GetEnvironmentVariables([EnvironmentVariableTarget]::Process)
$actualPresent = $environment.Contains("GOMODCACHE")
if ($actualPresent -ne $expectedPresent) {
  throw "GOMODCACHE presence mismatch for state $State after script"
}
if ($expectedPresent) {
  $actualValue = [Environment]::GetEnvironmentVariable("GOMODCACHE", [EnvironmentVariableTarget]::Process)
  if ($actualValue -cne $expectedValue) {
    throw "GOMODCACHE value mismatch for state $State after script"
  }
}
`
	if err := os.WriteFile(harness, []byte(harnessSource), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, state := range []string{"unset", "empty", "nonempty"} {
		for _, expectFailure := range []bool{false, true} {
			name := state + "/success"
			if expectFailure {
				name = state + "/failure"
			}
			t.Run(name, func(t *testing.T) {
				arguments := []string{
					"-NoProfile",
					"-ExecutionPolicy", "Bypass",
					"-File", harness,
					"-ScriptPath", cleanScript,
					"-FakeBin", fakeBin,
					"-State", state,
				}
				if expectFailure {
					arguments = append(arguments, "-ExpectFailure")
				}
				command := exec.Command(powerShell, arguments...)
				if output, err := command.CombinedOutput(); err != nil {
					t.Fatalf("PowerShell harness failed: %v\n%s", err, output)
				}
			})
		}
	}
}

func TestBackendVerificationClearsStaleBinariesBeforeRunning(t *testing.T) {
	powerShell := requireWindowsPowerShell(t)
	root := backendRoot(t)
	sandbox := t.TempDir()
	backendScript := filepath.Join(sandbox, "scripts", "verify-backend.ps1")
	copyTestFile(t, filepath.Join(root, "scripts", "verify-backend.ps1"), backendScript)
	fakeBin := filepath.Join(sandbox, "fake-bin")
	writeFakeGoCommand(t, fakeBin)

	verifyBin := filepath.Join(sandbox, ".tmp", "verify-bin")
	if err := os.MkdirAll(verifyBin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"admin-api.exe", "admin-worker.exe"} {
		if err := os.WriteFile(filepath.Join(verifyBin, name), []byte("stale"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	output, err := runBackendVerificationWithFakeGo(t, powerShell, sandbox, backendScript, fakeBin)
	if err == nil {
		t.Fatalf("backend verification must propagate the fake go failure\n%s", output)
	}
	if !strings.Contains(string(output), "exit code 23") {
		t.Fatalf("backend verification failed for the wrong reason: %v\n%s", err, output)
	}

	entries, err := os.ReadDir(verifyBin)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("verification output directory contains stale entries after early failure: %v", entries)
	}
}

func TestBackendVerificationRefusesReparseOutputDirectory(t *testing.T) {
	powerShell := requireWindowsPowerShell(t)
	root := backendRoot(t)
	sandbox := t.TempDir()
	backendScript := filepath.Join(sandbox, "scripts", "verify-backend.ps1")
	copyTestFile(t, filepath.Join(root, "scripts", "verify-backend.ps1"), backendScript)
	fakeBin := filepath.Join(sandbox, "fake-bin")
	writeFakeGoCommand(t, fakeBin)

	temporaryDirectory := filepath.Join(sandbox, ".tmp")
	if err := os.MkdirAll(temporaryDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(sandbox, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(outside, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(temporaryDirectory, "verify-bin")); err != nil {
		t.Skipf("cannot create directory symlink for reparse-point test: %v", err)
	}

	output, err := runBackendVerificationWithFakeGo(t, powerShell, sandbox, backendScript, fakeBin)
	if err == nil {
		t.Fatalf("backend verification must reject a reparse output directory\n%s", output)
	}
	if !strings.Contains(strings.ToLower(string(output)), "reparse point") {
		t.Fatalf("backend verification failed without identifying the reparse point: %v\n%s", err, output)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("reparse target was modified: %v", err)
	}
}

func TestAsynqmonChecksumMatchesTransparencyLogAcceptsCRLF(t *testing.T) {
	root := t.TempDir()
	architectureDir := filepath.Join(root, "internal", "architecture")
	if err := os.MkdirAll(architectureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.sum"), []byte(asynqmonV072Sum+"\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	previousWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(architectureDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previousWorkingDirectory); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	TestAsynqmonChecksumMatchesTransparencyLog(t)
}

func TestContainsExactLineRejectsModifiedLines(t *testing.T) {
	for name, content := range map[string]string{
		"old checksum": "github.com/hibiken/asynqmon v0.7.2 h1:EfLRppj5GlklMPzdCjdonpXz/D23meW0Pk6NAtkOPhw=\r\n",
		"prefix":       "prefix" + asynqmonV072Sum + "\r\n",
		"suffix":       asynqmonV072Sum + "suffix\n",
		"tampered":     strings.Replace(asynqmonV072Sum, "YohW", "YohX", 1) + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			if containsExactLine([]byte(content), asynqmonV072Sum) {
				t.Fatalf("modified line must not match: %q", content)
			}
		})
	}
}
