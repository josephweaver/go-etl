[CmdletBinding()]
param(
    [string]$ControllerUrl,

    [string]$TokenFile,

    [string]$HttpUrl,

    [string]$LocalControllerUrl = 'http://127.0.0.1:8080',

    [switch]$SkipLocalLoopbackCheck,

    [switch]$Help
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Write-Usage {
    @'
Usage:
  pwsh -NoProfile -File scripts/network/smoke-controller-endpoint.ps1 -ControllerUrl https://controller.example.org -TokenFile /etc/goet/secrets/controller-client-token
  pwsh -NoProfile -File scripts/network/smoke-controller-endpoint.ps1 -ControllerUrl https://controller.example.org -HttpUrl http://controller.example.org -TokenFile /etc/goet/secrets/controller-client-token

Checks a production-like HTTPS controller endpoint without printing token values.
'@ | Write-Host
}

function Get-CanonicalUrl {
    param(
        [Parameter(Mandatory = $true)][string]$Value,
        [Parameter(Mandatory = $true)][string]$RequiredScheme,
        [Parameter(Mandatory = $true)][string]$Label
    )

    $uri = [System.Uri]::new($Value)
    if (-not $uri.IsAbsoluteUri) {
        throw "$Label must be absolute"
    }
    if ($uri.Scheme -ne $RequiredScheme) {
        throw "$Label must use $RequiredScheme"
    }
    if ([string]::IsNullOrWhiteSpace($uri.Host)) {
        throw "$Label must include a host"
    }
    if (-not [string]::IsNullOrEmpty($uri.Query)) {
        throw "$Label must not include a query string"
    }
    if (-not [string]::IsNullOrEmpty($uri.Fragment)) {
        throw "$Label must not include a fragment"
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
        [string]$Token,
        [switch]$AllowRedirect
    )

    $handler = [System.Net.Http.HttpClientHandler]::new()
    $handler.AllowAutoRedirect = [bool]$AllowRedirect
    $client = [System.Net.Http.HttpClient]::new($handler)
    $request = [System.Net.Http.HttpRequestMessage]::new([System.Net.Http.HttpMethod]::new($Method), $Uri)
    if (-not [string]::IsNullOrEmpty($Token)) {
        $request.Headers.Authorization = [System.Net.Http.Headers.AuthenticationHeaderValue]::new('Bearer', $Token)
    }
    try {
        $response = $client.SendAsync($request).GetAwaiter().GetResult()
        return [pscustomobject]@{
            StatusCode = [int]$response.StatusCode
            Location = $response.Headers.Location
        }
    } finally {
        $request.Dispose()
        $client.Dispose()
        $handler.Dispose()
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
    throw 'ControllerUrl is required unless -Help is supplied.'
}
if ([string]::IsNullOrWhiteSpace($TokenFile)) {
    throw 'TokenFile is required unless -Help is supplied.'
}

$baseUrl = Get-CanonicalUrl -Value $ControllerUrl -RequiredScheme https -Label 'controller URL'
$token = Read-TokenFile -Path $TokenFile

Assert-Status -Label 'GET /healthz over HTTPS' -Expected @(200, 204) -Response (Invoke-ControllerRequest -Method GET -Uri ($baseUrl + '/healthz'))
Assert-Status -Label 'GET /status without token over HTTPS' -Expected @(401) -Response (Invoke-ControllerRequest -Method GET -Uri ($baseUrl + '/status'))
Assert-Status -Label 'GET /status with token over HTTPS' -Expected @(200) -Response (Invoke-ControllerRequest -Method GET -Uri ($baseUrl + '/status') -Token $token)

if (-not [string]::IsNullOrWhiteSpace($HttpUrl)) {
    $plainUrl = Get-CanonicalUrl -Value $HttpUrl -RequiredScheme http -Label 'HTTP URL'
    $response = Invoke-ControllerRequest -Method GET -Uri ($plainUrl + '/healthz')
    if ([int]$response.StatusCode -notin @(301, 302, 307, 308)) {
        throw "GET /healthz over HTTP returned HTTP $([int]$response.StatusCode), expected redirect"
    }
    if ($null -eq $response.Location -or $response.Location.Scheme -ne 'https') {
        throw 'HTTP redirect did not point to HTTPS'
    }
    Write-Host "GET /healthz over HTTP -> HTTP $([int]$response.StatusCode) redirect to HTTPS"
}

if (-not $SkipLocalLoopbackCheck) {
    $localUrl = Get-CanonicalUrl -Value $LocalControllerUrl -RequiredScheme http -Label 'local controller URL'
    Assert-Status -Label 'GET /healthz through loopback controller listener' -Expected @(200, 204) -Response (Invoke-ControllerRequest -Method GET -Uri ($localUrl + '/healthz'))
    Assert-Status -Label 'GET /status through loopback without token' -Expected @(401) -Response (Invoke-ControllerRequest -Method GET -Uri ($localUrl + '/status'))
}

Write-Host 'Controller endpoint smoke passed.'
