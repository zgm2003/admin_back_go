# Admin Foundation Security Baseline Design

**Status:** Approved in conversation on 2026-07-16; written for final file review.

**Goal:** Move the backend foundation to the smallest stable Go and module versions that close every vulnerability currently reached by the repository-owned verification gate, without broad dependency churn or adopting a release candidate toolchain.

## Decision

The backend baseline becomes Go `1.26.5`. The two vulnerable modules move to the first versions that close all findings currently reachable from this repository:

| Component | Current | Required | Reason |
| --- | --- | --- | --- |
| Go | `1.26.1` | `1.26.5` | `GO-2026-5856` affects Go 1.26 before 1.26.5 |
| `github.com/quic-go/quic-go` | `v0.59.0` | `v0.59.1` | `GO-2026-5676` is fixed in 0.59.1 |
| `golang.org/x/image` | `v0.25.0` | `v0.43.0` | closes both `GO-2026-4961` and `GO-2026-5061` |

Official vulnerability records are the source of truth:

- `https://vuln.go.dev/ID/GO-2026-5856.json`
- `https://vuln.go.dev/ID/GO-2026-5676.json`
- `https://vuln.go.dev/ID/GO-2026-4961.json`
- `https://vuln.go.dev/ID/GO-2026-5061.json`

Go `1.27rc2` is not adopted. It fixes the standard-library finding but would move the project onto a prerelease line and is not supported by the pinned `staticcheck@v0.7.0` verification tool. Keeping Go `1.26.1` is also rejected because it knowingly leaves a reachable standard-library vulnerability in the CI baseline.

## Scope

This correction owns:

- the `go` directive and the two vulnerable module requirements in `go.mod`;
- only the `go.sum` changes required by Go 1.26.5, `quic-go v0.59.1`, `x/image v0.43.0`, and minimum-version selection;
- the default Go build image in `Dockerfile`;
- both Docker-first build-image arguments in `deploy/docker-first/docker-compose.yml`;
- the backend language version documented in `README.md`;
- every active program plan that still declares Go 1.26.1, so later phases do not reintroduce the obsolete baseline;
- the P01 Task 6 `actions/setup-go` version;
- architecture guards that fail when the stable Go baseline or either fixed module version regresses.

It does not upgrade unrelated direct dependencies, adopt Go 1.27, change application behavior, install a local toolchain, or suppress a vulnerability result.

## Dependency update rules

The implementation updates the three explicit version facts first, then runs `go mod tidy`. Every resulting `go.mod` and `go.sum` change must be attributable to those facts through minimum-version selection. In particular, `x/image v0.43.0` requires a newer `x/text`; that transitive movement is allowed. Unrelated direct-version changes are rejected.

`quic-go` remains indirect because the repository reaches it through Gin's HTTP/3 surface. `x/image` remains direct because the AI image module imports `golang.org/x/image/webp`.

## Version consistency

The same Go patch version must appear in all executable build surfaces:

```text
go.mod                                      go 1.26.5
Dockerfile                                 golang:1.26.5-bookworm
deploy/docker-first/docker-compose.yml     .../golang:1.26.5-bookworm
Task 6 GitHub Actions                      go-version: 1.26.5
```

Documentation and all downstream implementation plans use the same value. A mixed 1.26.1/1.26.5 repository is not accepted.

## Verification design

Architecture tests first assert the new baseline and must fail against the current repository. After the minimal update, the implementation runs:

```powershell
go mod tidy
go mod verify
go test ./... -count=1
go vet ./...
go build -trimpath -o $env:TEMP\admin-api-secure-baseline.exe ./cmd/admin-api
go build -trimpath -o $env:TEMP\admin-worker-secure-baseline.exe ./cmd/admin-worker
go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
```

The current workstation runs Go `1.27rc1`. Its final command will continue to report the toolchain-owned `GO-2026-5856` until the workstation toolchain changes, which this task is not authorized to install. The module-owned `quic-go` and `x/image` findings must disappear locally. Task 6 CI, running Go 1.26.5, is the authoritative zero-finding proof for the standard library.

`staticcheck@v0.7.0` remains pinned. It targets Go 1.26 but cannot read Go 1.27rc1 export data, so its authoritative execution also occurs in Task 6 CI on Go 1.26.5. Neither local incompatibility may be converted into a skip in the repository scripts.

Docker is unavailable on the workstation. Dockerfile and Compose version consistency are covered by architecture guards locally; the Task 6 image build is the dynamic Docker proof.

## Commit and review boundaries

The approved sequence is:

1. commit this design specification;
2. correct the existing P01 plan and program-wide Go version declarations;
3. add failing version guards, apply the minimal toolchain/module update, and commit the secure baseline;
4. run specification and quality reviews before Task 6 begins.

Each implementation commit stays on `master`, as explicitly requested by the user. No worktree is created.

## Completion criteria

- all active Go version declarations are `1.26.5`;
- `go.mod` requires `quic-go v0.59.1` and `x/image v0.43.0`;
- no unrelated direct dependency changes are present;
- module verification, full tests, vet, and both builds pass;
- local `govulncheck` no longer reports the `quic-go` or `x/image` findings;
- Task 6 CI on Go 1.26.5 proves `staticcheck` and `govulncheck` without skips;
- the backend and frontend worktrees remain clean apart from the intentional backend commits;
- no runtime credential file or verification artifact is committed.
