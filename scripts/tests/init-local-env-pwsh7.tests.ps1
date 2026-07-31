$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

if ($PSVersionTable.PSVersion.Major -ne 7) {
  throw 'PowerShell 7 is required for this regression test'
}

$repositoryRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$initializer = Join-Path $repositoryRoot 'deploy\docker-first\init-local-env.ps1'
$temporaryRoot = Join-Path $env:TEMP ('admin-init-pwsh7-' + [guid]::NewGuid().ToString('N'))
$outputPath = Join-Path $temporaryRoot 'admin-go.env'
[IO.Directory]::CreateDirectory($temporaryRoot) | Out-Null

try {
  & $initializer `
    -OutputPath $outputPath `
    -MySQLDSN 'test_user:test-password@tcp(mysql:3306)/admin?charset=utf8mb4&parseTime=True&loc=Local' `
    -RedisAddress 'redis:6379' `
    -CorsOrigin 'http://localhost:5173'

  if (-not (Test-Path -LiteralPath $outputPath -PathType Leaf)) {
    throw 'PowerShell 7 initializer did not create the runtime env'
  }
  if (-not (Get-Acl -LiteralPath $outputPath).AreAccessRulesProtected) {
    throw 'PowerShell 7 initializer did not protect the runtime env ACL'
  }
  Write-Output 'PowerShell 7 init-local-env regression passed'
}
finally {
  if (Test-Path -LiteralPath $temporaryRoot) {
    Remove-Item -LiteralPath $temporaryRoot -Recurse -Force
  }
}
