[CmdletBinding()]
param(
    [string]$ControllerUrl,

    [string]$ClientTokenFile,

    [string]$WorkerTokenFile,

    [switch]$Help
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Write-Usage {
    @'
Usage:
  pwsh -NoProfile -File scripts/network/validate-controller-ingress.ps1 -ControllerUrl https://example.ts.net -ClientTokenFile .run/secrets/controller-client-token
  pwsh -NoProfile -File scripts/network/validate-controller-ingress.ps1 -ControllerUrl https://example.ts.net -ClientTokenFile .run/secrets/controller-client-token -WorkerTokenFile .run/secrets/controller-worker-token

Checks public HTTPS ingress behavior without printing token values.
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

function Read-TokenFile {
    param([Parameter(Mandatory = $true)][string]$Path)

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "token file missing: $Path"
    }
    $value = [System.IO.File]::ReadAllText((Resolve-Path -LiteralPath $Path).Path)
    if ($value.EndsWith("`r`n")) {
        $value = $value.Substring(0, $value.Length - 2)
    } elseif ($value.EndsWith("`n") -or $value.EndsWith("`r")) {
        $value = $value.Substring(0, $value.Length - 1)
    }
    if ([string]::IsNullOrEmpty($value)) {
        throw "token file is empty: $Path"
    }
    return $value
}

function Invoke-ControllerRequest {
    param(
        [Parameter(Mandatory = $true)][string]$Method,
        [Parameter(Mandatory = $true)][string]$Uri,
        [string]$Token
    )

    $client = [System.Net.Http.HttpClient]::new()
    $request = [System.Net.Http.HttpRequestMessage]::new([System.Net.Http.HttpMethod]::new($Method), $Uri)
    if (-not [string]::IsNullOrEmpty($Token)) {
        $request.Headers.Authorization = [System.Net.Http.Headers.AuthenticationHeaderValue]::new('Bearer', $Token)
    }
    try {
        $response = $client.SendAsync($request).GetAwaiter().GetResult()
        $body = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
        return [pscustomobject]@{
            StatusCode = [int]$response.StatusCode
            Body = $body
        }
    } finally {
        $request.Dispose()
        $client.Dispose()
    }
}

function Assert-Status {
    param(
        [Parameter(Mandatory = $true)]$Response,
        [Parameter(Mandatory = $true)][int[]]$Expected,
        [Parameter(Mandatory = $true)][string]$Label
    )

    if ($Expected -notcontains [int]$Response.StatusCode) {
        throw "$Label returned HTTP $([int]$Response.StatusCode), expected $($Expected -join '/')"
    }
    Write-Host "$Label -> HTTP $([int]$Response.StatusCode)"
}

if ($Help) {
    Write-Usage
    return
}

if ([string]::IsNullOrWhiteSpace($ControllerUrl)) {
    throw 'provide -ControllerUrl'
}

if ([string]::IsNullOrWhiteSpace($ClientTokenFile) -and [string]::IsNullOrWhiteSpace($WorkerTokenFile)) {
    throw 'provide -ClientTokenFile or -WorkerTokenFile'
}

$baseUrl = Get-CanonicalControllerUrl -Value $ControllerUrl
$clientToken = $null
$workerToken = $null
if (-not [string]::IsNullOrWhiteSpace($ClientTokenFile)) {
    $clientToken = Read-TokenFile -Path $ClientTokenFile
}
if (-not [string]::IsNullOrWhiteSpace($WorkerTokenFile)) {
    $workerToken = Read-TokenFile -Path $WorkerTokenFile
}

Assert-Status -Label 'GET /healthz without token' -Expected @(200, 204) -Response (Invoke-ControllerRequest -Method GET -Uri ($baseUrl + '/healthz'))
Assert-Status -Label 'GET /status without token' -Expected @(401) -Response (Invoke-ControllerRequest -Method GET -Uri ($baseUrl + '/status'))

if ($clientToken) {
    Assert-Status -Label 'GET /status with client token' -Expected @(200) -Response (Invoke-ControllerRequest -Method GET -Uri ($baseUrl + '/status') -Token $clientToken)
    Assert-Status -Label 'GET /work/next with client token' -Expected @(403) -Response (Invoke-ControllerRequest -Method GET -Uri ($baseUrl + '/work/next') -Token $clientToken)
}

if ($workerToken) {
    Assert-Status -Label 'GET /work/next with worker token' -Expected @(200, 204) -Response (Invoke-ControllerRequest -Method GET -Uri ($baseUrl + '/work/next') -Token $workerToken)
    Assert-Status -Label 'GET /status with worker token' -Expected @(403) -Response (Invoke-ControllerRequest -Method GET -Uri ($baseUrl + '/status') -Token $workerToken)
}

Write-Host 'Controller ingress validation passed.'
