[CmdletBinding()]
param(
    [string]$ControllerUrl,

    [string]$OutputPath,

    [switch]$Help
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Write-Usage {
    @'
Usage:
  pwsh -NoProfile -File scripts/network/render-controller-url-override.ps1 -ControllerUrl https://example.ts.net
  pwsh -NoProfile -File scripts/network/render-controller-url-override.ps1 -ControllerUrl https://example.ts.net -OutputPath .run/laptop-test-controller-url.json

Writes a local controller JSON fragment that sets controller_config.controller_url.
The script validates that the URL is HTTPS and never edits committed config.
'@ | Write-Host
}

function Get-CanonicalControllerUrl {
    param([Parameter(Mandatory = $true)][string]$Value)

    $uri = [System.Uri]::new($Value)
    if (-not $uri.IsAbsoluteUri) {
        throw 'controller URL must be absolute'
    }
    if ($uri.Scheme -ne 'https') {
        throw 'controller URL must use https'
    }
    if ([string]::IsNullOrWhiteSpace($uri.Host)) {
        throw 'controller URL must include a host'
    }
    if (-not [string]::IsNullOrEmpty($uri.Query)) {
        throw 'controller URL must not include a query string'
    }
    if (-not [string]::IsNullOrEmpty($uri.Fragment)) {
        throw 'controller URL must not include a fragment'
    }

    return $uri.GetLeftPart([System.UriPartial]::Path).TrimEnd('/')
}

if ($Help) {
    Write-Usage
    return
}

if ([string]::IsNullOrWhiteSpace($ControllerUrl)) {
    throw 'provide -ControllerUrl'
}

$canonical = Get-CanonicalControllerUrl -Value $ControllerUrl
$document = [ordered]@{
    api_version = 'goet/v1alpha1'
    kind = 'Controller'
    variables = @(
        [ordered]@{
            name = [ordered]@{
                namespace = 'controller_config'
                key = 'controller_url'
            }
            type = 'string'
            expression = $canonical
        }
    )
}

$json = $document | ConvertTo-Json -Depth 10
if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $json
    return
}

$directory = Split-Path -Parent $OutputPath
if (-not [string]::IsNullOrWhiteSpace($directory)) {
    New-Item -ItemType Directory -Force -Path $directory | Out-Null
}
[System.IO.File]::WriteAllText($OutputPath, $json + [Environment]::NewLine, [System.Text.UTF8Encoding]::new($false))
Write-Host "Wrote controller URL override: $OutputPath"
