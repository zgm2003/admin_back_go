param(
  [string]$Container = "admin-go-state-mysql",
  [string]$Database = "admin",
  [string]$User = "root",
  [string]$Password = "admin_go_local"
)

$ErrorActionPreference = "Stop"

$sql = @'
SELECT 'canvas_pages' AS k, COUNT(*) AS v
FROM permissions
WHERE platform='canvas' AND type=2 AND is_del=2
UNION ALL
SELECT 'canvas_buttons_with_parent', COUNT(*)
FROM permissions
WHERE platform='canvas' AND type=3 AND parent_id > 0 AND is_del=2
UNION ALL
SELECT 'canvas_role_grants', COUNT(*)
FROM role_permissions rp
JOIN permissions p ON p.id=rp.permission_id
WHERE p.platform='canvas' AND p.type IN (2,3) AND p.is_del=2 AND rp.is_del=2
UNION ALL
SELECT 'canvas_orphan_buttons', COUNT(*)
FROM permissions
WHERE platform='canvas' AND type=3 AND parent_id = 0 AND is_del=2;
'@

$output = $sql | docker exec -i -e "MYSQL_PWD=$Password" $Container mysql --protocol=socket "-u$User" --database=$Database --batch --skip-column-names 2>&1
if ($LASTEXITCODE -ne 0) {
  throw "Canvas RBAC query failed: $($output | Out-String)"
}

$counts = @{}
foreach ($line in @($output)) {
  $trimmed = [string]$line
  if ([string]::IsNullOrWhiteSpace($trimmed)) {
    continue
  }
  $parts = $trimmed.Trim().Split("`t")
  if ($parts.Count -ne 2) {
    throw "Unexpected Canvas RBAC query row: $trimmed"
  }
  $counts[$parts[0]] = [int]$parts[1]
}

$thresholds = @{
  canvas_pages = 7
  canvas_buttons_with_parent = 8
  canvas_role_grants = 15
}

$maximums = @{
  canvas_orphan_buttons = 0
}

$failures = New-Object System.Collections.Generic.List[object]
foreach ($key in $thresholds.Keys) {
  if (-not $counts.ContainsKey($key)) {
    $failures.Add([pscustomobject]@{ key = $key; expected_min = $thresholds[$key]; actual = $null })
    continue
  }
  if ($counts[$key] -lt $thresholds[$key]) {
    $failures.Add([pscustomobject]@{ key = $key; expected_min = $thresholds[$key]; actual = $counts[$key] })
  }
}
foreach ($key in $maximums.Keys) {
  if (-not $counts.ContainsKey($key)) {
    $failures.Add([pscustomobject]@{ key = $key; expected_max = $maximums[$key]; actual = $null })
    continue
  }
  if ($counts[$key] -gt $maximums[$key]) {
    $failures.Add([pscustomobject]@{ key = $key; expected_max = $maximums[$key]; actual = $counts[$key] })
  }
}

$result = [pscustomobject]@{
  code = 0
  container = $Container
  database = $Database
  counts = [pscustomobject]$counts
  thresholds = [pscustomobject]$thresholds
  maximums = [pscustomobject]$maximums
}

if ($failures.Count -gt 0) {
  $result.code = 1
  $result | Add-Member -NotePropertyName failures -NotePropertyValue $failures
  $result | ConvertTo-Json -Depth 5
  throw "Canvas RBAC check failed"
}

$result | ConvertTo-Json -Depth 5
