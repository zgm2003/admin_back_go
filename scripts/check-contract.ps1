param(
  [string[]]$Paths = @()
)

$ErrorActionPreference = 'Stop'

$BackendRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$WorkspaceRoot = (Resolve-Path (Join-Path $BackendRoot '..')).Path

if ($Paths.Count -eq 0) {
  $Paths = @((Join-Path $WorkspaceRoot 'docs/contracts/admin-api-v1.md'))
}

$rules = @(
  @{
    Name = 'camel-case admin path segment'
    Regex = [regex]::new('/api/admin/v[0-9]+/[A-Z][A-Za-z0-9_]*(?:[/\s`]|$)')
  },
  @{
    Name = 'legacy action path under admin v1'
    Regex = [regex]::new('/api/admin/v[0-9]+/[a-z0-9][a-z0-9-]*/(?:list|add|edit|del|batchEdit|batch_edit)(?:[/\s`]|$)', [System.Text.RegularExpressions.RegexOptions]::IgnoreCase)
  }
)

$violations = New-Object System.Collections.Generic.List[object]

foreach ($path in $Paths) {
  $resolved = (Resolve-Path -LiteralPath $path).Path
  $lines = Get-Content -LiteralPath $resolved
  for ($i = 0; $i -lt $lines.Count; $i++) {
    $line = $lines[$i]
    foreach ($rule in $rules) {
      if ($rule.Regex.IsMatch($line)) {
        $violations.Add([pscustomobject]@{
          file = $resolved
          line = $i + 1
          rule = $rule.Name
          text = $line.Trim()
        })
      }
    }
  }
}

if ($violations.Count -gt 0) {
  $violations | ConvertTo-Json -Depth 4
  throw "Contract check failed: found $($violations.Count) legacy-style admin API path(s)."
}

[pscustomobject]@{
  code = 0
  checked_files = $Paths.Count
  msg = 'contract check passed'
} | ConvertTo-Json -Depth 4
