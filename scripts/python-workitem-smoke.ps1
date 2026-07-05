[CmdletBinding()]
param(
    [switch]$Help
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Write-Usage {
    @'
Usage:
  pwsh -NoProfile -File scripts/python-workitem-smoke.ps1
  pwsh -NoProfile -File scripts/python-workitem-smoke.ps1 -Help

What it does:
  - validates the sibling ../go-etl-demo-project fixture
  - validates the demo JSON files
  - compiles scripts/hello.py when python3 is available
  - starts the controller from cmd/controller/demo-config.json
  - submits the python-hello workflow submission
  - waits for the controller to become idle
  - verifies the worker output file and attempt logs
  - shuts down the controller cleanly
'@ | Write-Host
}

function Resolve-AbsolutePath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    return (Resolve-Path -LiteralPath $Path).Path
}

function Assert-LeafExists {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,

        [Parameter(Mandatory = $true)]
        [string]$Label
    )

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "$Label missing: $Path"
    }
}

function Assert-DirectoryExists {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,

        [Parameter(Mandatory = $true)]
        [string]$Label
    )

    if (-not (Test-Path -LiteralPath $Path -PathType Container)) {
        throw "$Label missing: $Path"
    }
}

function Remove-TreeIfPresent {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    if (Test-Path -LiteralPath $Path) {
        Remove-Item -LiteralPath $Path -Recurse -Force
    }
}

function Test-JsonSyntax {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    $python3 = Get-Command python3 -ErrorAction SilentlyContinue
    if ($python3) {
        & $python3.Path -m json.tool $Path | Out-Null
        return 'python3 -m json.tool'
    }

    $python = Get-Command python -ErrorAction SilentlyContinue
    if ($python) {
        & $python.Path -m json.tool $Path | Out-Null
        return 'python -m json.tool'
    }

    Get-Content -Raw -LiteralPath $Path | ConvertFrom-Json | Out-Null
    return 'ConvertFrom-Json'
}

function Assert-Python3Available {
    $python3 = Get-Command python3 -ErrorAction SilentlyContinue
    if (-not $python3) {
        throw "python3 is required because cmd/worker/demo-config.json defaults to python3"
    }
    return $python3.Path
}

function Wait-ForHttpGet {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Uri,

        [Parameter(Mandatory = $true)]
        [int]$ExpectedStatusCode,

        [Parameter(Mandatory = $true)]
        [int]$TimeoutSeconds
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        try {
            $response = Invoke-WebRequest -Method Get -Uri $Uri -TimeoutSec 5
            if ($response.StatusCode -eq $ExpectedStatusCode) {
                return
            }
        } catch {
            Start-Sleep -Milliseconds 500
            continue
        }

        Start-Sleep -Milliseconds 500
    }

    throw "timed out waiting for $Uri"
}

function Get-ControllerStatus {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ControllerUrl
    )

    return Invoke-RestMethod -Method Get -Uri ($ControllerUrl.TrimEnd('/') + '/status') -TimeoutSec 5
}

function Wait-ForControllerIdle {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ControllerUrl,

        [Parameter(Mandatory = $true)]
        [int]$TimeoutSeconds
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    $lastStatus = $null
    while ((Get-Date) -lt $deadline) {
        $status = Get-ControllerStatus -ControllerUrl $ControllerUrl
        $lastStatus = $status
        if ($status.pending -eq 0 -and $status.assigned -eq 0) {
            return $status
        }
        Start-Sleep -Milliseconds 500
    }

    throw "controller did not become idle within $TimeoutSeconds seconds; last status: $($lastStatus | ConvertTo-Json -Compress)"
}

function Assert-OutputFields {
    param(
        [Parameter(Mandatory = $true)]
        [object]$Output
    )

    if ($Output.status -ne 'ok') {
        throw "unexpected status: $($Output.status)"
    }
    if ($Output.message -ne 'python hello fixture completed') {
        throw "unexpected message: $($Output.message)"
    }
    if ($Output.operation -ne 'scripts/hello.py') {
        throw "unexpected operation: $($Output.operation)"
    }
    if (($Output.observed_input_keys -join ',') -ne 'work_item') {
        throw "unexpected observed_input_keys: $($Output.observed_input_keys | ConvertTo-Json -Compress)"
    }
    if ($Output.support_input_exists -ne $true) {
        throw "expected support_input_exists to be true"
    }
}

if ($Help) {
    Write-Usage
    exit 0
}

$scriptRoot = Split-Path -Parent $PSCommandPath
$repoRoot = Resolve-AbsolutePath (Join-Path $scriptRoot '..')
$demoRootCandidate = Join-Path $repoRoot '..\go-etl-demo-project'
if (-not (Test-Path -LiteralPath $demoRootCandidate -PathType Container)) {
    throw "missing sibling demo project: $demoRootCandidate"
}
$demoRoot = Resolve-AbsolutePath $demoRootCandidate

Set-Location $repoRoot

$controllerConfig = Join-Path $repoRoot 'cmd\controller\demo-config.json'
$workerConfig = Join-Path $repoRoot 'cmd\worker\demo-config.json'
$projectJson = Join-Path $demoRoot 'project.json'
$workflowJson = Join-Path $demoRoot 'workflows\python-hello.json'
$submissionJson = Join-Path $demoRoot 'submissions\python-hello-local.json'
$entrypointPy = Join-Path $demoRoot 'scripts\hello.py'
$environmentJson = Join-Path $demoRoot 'environments\system-python.json'

Assert-LeafExists -Path $controllerConfig -Label 'controller config'
Assert-LeafExists -Path $workerConfig -Label 'worker config'
Assert-LeafExists -Path $projectJson -Label 'project fixture'
Assert-LeafExists -Path $workflowJson -Label 'workflow fixture'
Assert-LeafExists -Path $submissionJson -Label 'submission fixture'
Assert-LeafExists -Path $entrypointPy -Label 'python entrypoint'
Assert-LeafExists -Path $environmentJson -Label 'python environment'

Write-Host "Validating JSON fixtures..."
$jsonTool = Test-JsonSyntax -Path $projectJson
[void](Test-JsonSyntax -Path $workflowJson)
[void](Test-JsonSyntax -Path $submissionJson)
[void](Test-JsonSyntax -Path $environmentJson)
Write-Host "Validated JSON syntax with $jsonTool."

$python3Path = Assert-Python3Available

Write-Host "Compiling hello.py with python3..."
& $python3Path -m py_compile $entrypointPy

$controllerLogRoot = Join-Path $repoRoot '.run\python-workitem-smoke'
$controllerStdout = Join-Path $controllerLogRoot 'controller.stdout.log'
$controllerStderr = Join-Path $controllerLogRoot 'controller.stderr.log'
$controllerDatabaseDir = Join-Path $repoRoot '.run\controller'
$workerRuntimeDir = Join-Path $repoRoot 'cmd\worker\.run'

Remove-TreeIfPresent -Path $controllerLogRoot
Remove-TreeIfPresent -Path $controllerDatabaseDir
Remove-TreeIfPresent -Path $workerRuntimeDir
New-Item -ItemType Directory -Force -Path $controllerLogRoot | Out-Null

$go = Get-Command go -ErrorAction Stop
$controllerArgs = @('run', './cmd/controller', './cmd/controller/demo-config.json')
$controllerProcess = Start-Process -FilePath $go.Path -ArgumentList $controllerArgs -WorkingDirectory $repoRoot -PassThru -WindowStyle Hidden -RedirectStandardOutput $controllerStdout -RedirectStandardError $controllerStderr

try {
    Wait-ForHttpGet -Uri 'http://localhost:8080/healthz' -ExpectedStatusCode 204 -TimeoutSeconds 120
    Wait-ForHttpGet -Uri 'http://localhost:8080/status' -ExpectedStatusCode 200 -TimeoutSeconds 120

    Write-Host "Submitting python-hello workflow..."
    $submissionBody = Get-Content -Raw -LiteralPath $submissionJson
    $submitResponse = Invoke-WebRequest -Method Post -Uri 'http://localhost:8080/workflow' -ContentType 'application/json' -Body $submissionBody -TimeoutSec 30
    if ($submitResponse.StatusCode -ne 204) {
        throw "workflow submission returned HTTP $($submitResponse.StatusCode), want 204"
    }

    $finalStatus = Wait-ForControllerIdle -ControllerUrl 'http://localhost:8080' -TimeoutSeconds 180
    Write-Host ("Controller idle: pending={0}, assigned={1}" -f $finalStatus.pending, $finalStatus.assigned)

    $workerDataDir = Join-Path $repoRoot 'cmd\worker\.run\data'
    Assert-DirectoryExists -Path $workerDataDir -Label 'worker data dir'
    $expectedOutputPath = Join-Path $workerDataDir 'python-hello-hello.json'
    Assert-LeafExists -Path $expectedOutputPath -Label 'worker output'

    $output = Get-Content -Raw -LiteralPath $expectedOutputPath | ConvertFrom-Json
    Assert-OutputFields -Output $output

    $attemptRoot = Join-Path $repoRoot 'cmd\worker\.run\tmp\attempts'
    Assert-DirectoryExists -Path $attemptRoot -Label 'worker attempt root'
    $attemptDir = Get-ChildItem -LiteralPath $attemptRoot -Directory | Sort-Object LastWriteTime -Descending | Select-Object -First 1
    if (-not $attemptDir) {
        throw "no worker attempt directory found under $attemptRoot"
    }

    $attemptLogDir = Join-Path $attemptDir.FullName 'logs'
    Assert-DirectoryExists -Path $attemptLogDir -Label 'worker attempt logs'
    Assert-LeafExists -Path (Join-Path $attemptLogDir 'stdout.log') -Label 'stdout log'
    Assert-LeafExists -Path (Join-Path $attemptLogDir 'stderr.log') -Label 'stderr log'

    Write-Host "Output file: $expectedOutputPath"
    Write-Host "Attempt logs: $attemptLogDir"
    Write-Host "Controller log: $controllerStdout"
    Write-Host "Smoke path completed."
} finally {
    try {
        Invoke-WebRequest -Method Post -Uri 'http://localhost:8080/shutdown' -TimeoutSec 10 | Out-Null
    } catch {
        # If shutdown is already in progress or the controller never came up,
        # fall back to process cleanup below.
    }

    if ($controllerProcess -and -not $controllerProcess.HasExited) {
        try {
            Wait-Process -Id $controllerProcess.Id -Timeout 30
        } catch {
            Stop-Process -Id $controllerProcess.Id -Force
        }
    }
}
