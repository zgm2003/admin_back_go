param(
  [string]$SourceRoot = "E:/admin/admin_back",
  [string]$TargetRoot = (Resolve-Path "$PSScriptRoot/..").Path
)

$ErrorActionPreference = "Stop"

$relativeFiles = @(
  "runtime/cert/alipay/appPublicCert.crt",
  "runtime/cert/alipay/alipayPublicCert.crt",
  "runtime/cert/alipay/alipayRootCert.crt"
)

function ConvertTo-SlashPath {
  param([string]$Path)
  return $Path.Replace('\', '/')
}

foreach ($relative in $relativeFiles) {
  $source = Join-Path $SourceRoot $relative
  $target = Join-Path $TargetRoot $relative

  if (-not (Test-Path -LiteralPath $source -PathType Leaf)) {
    throw "missing source cert: $(ConvertTo-SlashPath $source)"
  }

  New-Item -ItemType Directory -Force -Path (Split-Path $target) | Out-Null
  Copy-Item -LiteralPath $source -Destination $target -Force

  $item = Get-Item -LiteralPath $target
  $hash = Get-FileHash -Algorithm SHA256 -LiteralPath $target
  [pscustomobject]@{
    relative = $relative
    target   = ConvertTo-SlashPath $item.FullName
    bytes    = $item.Length
    sha256   = $hash.Hash
  }
}
