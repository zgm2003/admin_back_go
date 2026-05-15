param(
  [switch]$NoDb,
  [switch]$DisallowLegacyRoot,
  [string]$RepoRoot = (Resolve-Path "$PSScriptRoot/..").Path,
  [string]$CertBaseDir = $env:PAYMENT_CERT_BASE_DIR
)

$ErrorActionPreference = "Stop"

function ConvertTo-SlashPath {
  param([string]$Path)
  return $Path.Replace('\', '/')
}

function Read-DotEnv {
  param([string]$Path)

  $values = @{}
  if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
    return $values
  }

  foreach ($line in Get-Content -LiteralPath $Path) {
    $trimmed = $line.Trim()
    if ($trimmed -eq "" -or $trimmed.StartsWith("#")) {
      continue
    }

    $parts = $trimmed.Split("=", 2)
    if ($parts.Count -ne 2) {
      continue
    }

    $key = $parts[0].Trim()
    $value = $parts[1].Trim()
    if (($value.StartsWith('"') -and $value.EndsWith('"')) -or ($value.StartsWith("'") -and $value.EndsWith("'"))) {
      $value = $value.Substring(1, $value.Length - 2)
    }
    $values[$key] = $value
  }
  return $values
}

function Import-DotEnvForGo {
  param([hashtable]$Values)

  foreach ($key in $Values.Keys) {
    if ([string]::IsNullOrEmpty([Environment]::GetEnvironmentVariable($key, "Process"))) {
      [Environment]::SetEnvironmentVariable($key, $Values[$key], "Process")
    }
  }
}

function Get-DbAlipayCertPaths {
  param([string]$Root)

  $tmpDir = Join-Path $Root ".tmp"
  New-Item -ItemType Directory -Force -Path $tmpDir | Out-Null
  $probe = Join-Path $tmpDir "check-payment-certs-db.go"

  @'
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"admin_back_go/internal/config"

	_ "github.com/go-sql-driver/mysql"
)

type certRow struct {
	Code             string `json:"code"`
	PublicCertPath   string `json:"public_cert_path"`
	PlatformCertPath string `json:"platform_cert_path"`
	RootCertPath     string `json:"root_cert_path"`
}

func main() {
	_ = config.LoadDotEnv()
	cfg := config.Load()
	if cfg.MySQL.DSN == "" {
		fmt.Fprintln(os.Stderr, "MYSQL_DSN is empty; payment_alipay_configs cert paths require DB")
		os.Exit(2)
	}

	db, err := sql.Open("mysql", cfg.MySQL.DSN)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, `
SELECT
  code,
  app_cert_path AS public_cert_path,
  alipay_cert_path AS platform_cert_path,
  alipay_root_cert_path AS root_cert_path
FROM payment_alipay_configs
WHERE status = 1
  AND is_del = 2
ORDER BY id
LIMIT 1`)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer rows.Close()

	var out []certRow
	for rows.Next() {
		var row certRow
		if err := rows.Scan(&row.Code, &row.PublicCertPath, &row.PlatformCertPath, &row.RootCertPath); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(out) == 0 {
		fmt.Fprintln(os.Stderr, "no enabled payment_alipay_configs row")
		os.Exit(3)
	}
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
'@ | Set-Content -LiteralPath $probe -Encoding UTF8

  try {
    Push-Location $Root
    try {
      $output = go run $probe 2>&1
      if ($LASTEXITCODE -ne 0) {
        throw "read enabled Alipay payment config cert paths failed: $($output | Out-String)"
      }
    } finally {
      Pop-Location
    }

    $rows = @($output | Out-String | ConvertFrom-Json)
    $paths = New-Object System.Collections.Generic.List[string]
    foreach ($row in $rows) {
      foreach ($value in @($row.public_cert_path, $row.platform_cert_path, $row.root_cert_path)) {
        $path = [string]$value
        if ([string]::IsNullOrWhiteSpace($path)) {
          throw "enabled Alipay payment config has an empty cert path"
        }
        if (-not $paths.Contains($path)) {
          $paths.Add($path)
        }
      }
    }
    return $paths.ToArray()
  } finally {
    Remove-Item -LiteralPath $probe -Force -ErrorAction SilentlyContinue
  }
}

function Resolve-CertPath {
  param(
    [string]$StoredPath,
    [string]$BaseDir,
    [string]$Root
  )

  $clean = $StoredPath.Trim().Replace('\', '/')
  if ($clean -eq "") {
    throw "empty cert path"
  }

  if ([System.IO.Path]::IsPathRooted($clean)) {
    $candidate = [System.IO.Path]::GetFullPath($clean)
    if (Test-Path -LiteralPath $candidate -PathType Leaf) {
      return ConvertTo-SlashPath $candidate
    }
    throw "missing cert file: $(ConvertTo-SlashPath $candidate)"
  }

  $bases = New-Object System.Collections.Generic.List[string]
  foreach ($base in @($BaseDir, $Root)) {
    if ([string]::IsNullOrWhiteSpace($base)) {
      continue
    }
    $fullBase = [System.IO.Path]::GetFullPath($base)
    if (-not $bases.Contains($fullBase)) {
      $bases.Add($fullBase)
    }
  }

  foreach ($base in $bases) {
    $candidate = [System.IO.Path]::GetFullPath((Join-Path $base $clean))
    if (Test-Path -LiteralPath $candidate -PathType Leaf) {
      return ConvertTo-SlashPath $candidate
    }
  }

  throw "missing cert file: $clean"
}

$RepoRoot = [System.IO.Path]::GetFullPath($RepoRoot)
$envValues = Read-DotEnv (Join-Path $RepoRoot ".env")
Import-DotEnvForGo $envValues

if ([string]::IsNullOrWhiteSpace($CertBaseDir) -and $envValues.ContainsKey("PAYMENT_CERT_BASE_DIR")) {
  $CertBaseDir = $envValues["PAYMENT_CERT_BASE_DIR"]
}
if ([string]::IsNullOrWhiteSpace($CertBaseDir)) {
  $CertBaseDir = $RepoRoot
}

if ($NoDb) {
  throw "payment config certificate check requires DB because certificate paths are stored in payment_alipay_configs"
}

$storedPaths = Get-DbAlipayCertPaths $RepoRoot

foreach ($storedPath in $storedPaths) {
  $resolved = Resolve-CertPath -StoredPath $storedPath -BaseDir $CertBaseDir -Root $RepoRoot
  if ($DisallowLegacyRoot -and $resolved.ToLowerInvariant().Contains("e:/admin/admin_back")) {
    throw "cert resolved through legacy PHP root: $resolved"
  }

  $item = Get-Item -LiteralPath $resolved
  $hash = Get-FileHash -Algorithm SHA256 -LiteralPath $resolved
  [pscustomobject]@{
    path   = ConvertTo-SlashPath $item.FullName
    bytes  = $item.Length
    sha256 = $hash.Hash
  }
}
