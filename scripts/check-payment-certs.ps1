param(
  [switch]$NoDb,
  [switch]$DisallowLegacyRoot,
  [string]$RepoRoot = (Resolve-Path "$PSScriptRoot/..").Path,
  [string]$CertBaseDir = $env:PAYMENT_CERT_BASE_DIR
)

$ErrorActionPreference = "Stop"

$requiredFallbackPaths = @(
  "runtime/cert/alipay/appPublicCert.crt",
  "runtime/cert/alipay/alipayPublicCert.crt",
  "runtime/cert/alipay/alipayRootCert.crt"
)

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
	PublicCertPath   string `json:"public_cert_path"`
	PlatformCertPath string `json:"platform_cert_path"`
	RootCertPath     string `json:"root_cert_path"`
}

func main() {
	_ = config.LoadDotEnv()
	cfg := config.Load()
	if cfg.MySQL.DSN == "" {
		fmt.Fprintln(os.Stderr, "MYSQL_DSN is empty; pass -NoDb for file-only cert checks")
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
  cfg.app_cert_path AS public_cert_path,
  cfg.alipay_cert_path AS platform_cert_path,
  cfg.alipay_root_cert_path AS root_cert_path
FROM payment_channel_configs AS cfg
JOIN payment_channels AS ch ON ch.id = cfg.channel_id
WHERE ch.provider = 'alipay'
  AND ch.status = 1
  AND ch.is_del = 2
ORDER BY ch.id`)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer rows.Close()

	var out []certRow
	for rows.Next() {
		var row certRow
		if err := rows.Scan(&row.PublicCertPath, &row.PlatformCertPath, &row.RootCertPath); err != nil {
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
		fmt.Fprintln(os.Stderr, "no active Alipay payment_channels/payment_channel_configs rows")
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
        throw "read active Alipay payment channel cert paths failed: $($output | Out-String)"
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
          throw "active Alipay payment channel config has an empty cert path"
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

$storedPaths = if ($NoDb) {
  $requiredFallbackPaths
} else {
  Get-DbAlipayCertPaths $RepoRoot
}

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
