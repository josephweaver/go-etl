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
  - verifies the worker output file and controller logs by submission_id
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

function Invoke-CurlRequest {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Method,

        [Parameter(Mandatory = $true)]
        [string]$Uri,

        [string]$Body
    )

    $responsePath = [System.IO.Path]::GetTempFileName()
    $headerPath = [System.IO.Path]::GetTempFileName()
    $bodyPath = $null
    try {
        $curlArgs = @('-sS', '-D', $headerPath, '-X', $Method.ToUpperInvariant(), '-o', $responsePath, '-w', '%{http_code}')
        if ($null -ne $Body) {
            $bodyPath = [System.IO.Path]::GetTempFileName()
            [System.IO.File]::WriteAllText($bodyPath, $Body, [System.Text.UTF8Encoding]::new($false))
            $curlArgs += @('-H', 'Content-Type: application/json', '--data-binary', ('@' + $bodyPath))
        }

        $statusText = & curl.exe @curlArgs $Uri
        $statusCode = 0
        if (-not [int]::TryParse(($statusText | Out-String).Trim(), [ref]$statusCode)) {
            throw "curl returned non-numeric status code: $statusText"
        }

        $bodyText = ''
        if (Test-Path -LiteralPath $responsePath -PathType Leaf) {
            $bodyText = Get-Content -Raw -LiteralPath $responsePath
        }

        $headers = @{}
        if (Test-Path -LiteralPath $headerPath -PathType Leaf) {
            foreach ($line in Get-Content -LiteralPath $headerPath) {
                if ($line -match '^\s*$' -or $line -like 'HTTP/*') {
                    continue
                }
                $parts = $line -split ':\s*', 2
                if ($parts.Count -eq 2) {
                    $headers[$parts[0].ToLowerInvariant()] = $parts[1].Trim()
                }
            }
        }

        return [pscustomobject]@{
            StatusCode = $statusCode
            Body       = $bodyText
            Headers    = $headers
        }
    } finally {
        if ($bodyPath -and (Test-Path -LiteralPath $bodyPath)) {
            Remove-Item -LiteralPath $bodyPath -Force
        }
        if (Test-Path -LiteralPath $responsePath) {
            Remove-Item -LiteralPath $responsePath -Force
        }
        if (Test-Path -LiteralPath $headerPath) {
            Remove-Item -LiteralPath $headerPath -Force
        }
    }
}

function Parse-SubmissionId {
    param(
        [Parameter(Mandatory = $true)]
        [pscustomobject]$Response
    )

    if (-not $Response) {
        return $null
    }

    if ($Response.Body) {
        try {
            $submission = $Response.Body | ConvertFrom-Json
            if ($submission.submission_id) {
                return [string]$submission.submission_id
            }
        } catch {
            # Continue to header-based extraction.
        }
    }

    $location = $Response.Headers['location']
    if ($location -and $location -match '/submissions/([^/?#]+)') {
        return [string]$Matches[1]
    }

    return $null
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

function Resolve-PythonExecutable {
    $python3 = Get-Command python3 -ErrorAction SilentlyContinue
    if ($python3) {
        return $python3.Path
    }

    $python = Get-Command python -ErrorAction SilentlyContinue
    if ($python) {
        return $python.Path
    }

    $candidates = @(
        'C:\ProgramData\anaconda3\python.exe',
        'C:\Python314\python.exe',
        'C:\Program Files\Python314\python.exe',
        'C:\Program Files\Python313\python.exe',
        'C:\Program Files\Python312\python.exe',
        'C:\Program Files\Python311\python.exe',
        'C:\Program Files\Python310\python.exe'
    )

    if ($env:LocalAppData) {
        $candidates += @(
            (Join-Path $env:LocalAppData 'Programs\Python\Python314\python.exe'),
            (Join-Path $env:LocalAppData 'Programs\Python\Python313\python.exe'),
            (Join-Path $env:LocalAppData 'Programs\Python\Python312\python.exe'),
            (Join-Path $env:LocalAppData 'Programs\Python\Python311\python.exe'),
            (Join-Path $env:LocalAppData 'Programs\Python\Python310\python.exe')
        )
    }

    foreach ($candidate in $candidates) {
        if ($candidate -and (Test-Path -LiteralPath $candidate -PathType Leaf)) {
            return $candidate
        }
    }

    throw "python3 or python is required for the smoke path, or a known Windows python.exe install path must exist"
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
            $response = Invoke-CurlRequest -Method Get -Uri $Uri
            if ([int]$response.StatusCode -eq $ExpectedStatusCode) {
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

    $response = Invoke-CurlRequest -Method Get -Uri ($ControllerUrl.TrimEnd('/') + '/status')
    return $response.Body | ConvertFrom-Json
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

function Wait-ForControllerPortClosed {
    param(
        [Parameter(Mandatory = $true)]
        [int]$Port,

        [Parameter(Mandatory = $true)]
        [int]$TimeoutSeconds
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        $connection = Get-NetTCPConnection -LocalPort $Port -ErrorAction SilentlyContinue | Select-Object -First 1
        if (-not $connection) {
            return
        }
        Start-Sleep -Milliseconds 500
    }

    throw "timed out waiting for port $Port to close"
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

$pythonExecutable = Resolve-PythonExecutable

Write-Host "Compiling hello.py..."
& $pythonExecutable -m py_compile $entrypointPy

$controllerLogRoot = Join-Path $repoRoot ('.run\python-workitem-smoke\' + (Get-Date -Format 'yyyyMMdd-HHmmss') + '-pid' + $PID)
$controllerStdout = Join-Path $controllerLogRoot 'controller.stdout.log'
$controllerStderr = Join-Path $controllerLogRoot 'controller.stderr.log'
$controllerDatabaseDir = Join-Path $repoRoot '.run\controller'
$controllerDatabaseLock = Join-Path $controllerDatabaseDir 'workflow-execution.sqlite.controller.lock'
$workerRuntimeDir = Join-Path $repoRoot 'cmd\worker\.run'
$workerLogDir = Join-Path $workerRuntimeDir 'logs'
$workerTmpDir = Join-Path $workerRuntimeDir 'tmp'
$workerDataDir = Join-Path $workerRuntimeDir 'data'
$workerSmokeConfig = Join-Path $repoRoot '.run\python-workitem-smoke-worker-config.json'
$python3ShimDir = Join-Path $repoRoot '.run\python3-shim'
$python3ShimPath = Join-Path $python3ShimDir 'python3.cmd'
$pythonSmokeWrapperPath = Join-Path ([System.IO.Path]::GetTempPath()) ("goet-python-smoke-wrapper-{0}.cmd" -f $PID)
$controllerBaseUrl = 'http://localhost:8080'

try {
    [void](Invoke-CurlRequest -Method Post -Uri ($controllerBaseUrl + '/shutdown'))
    Wait-ForControllerPortClosed -Port 8080 -TimeoutSeconds 15
} catch {
    # Ignore stale or absent controllers. Cleanup below will continue with the
    # current on-disk state.
}

Remove-TreeIfPresent -Path $controllerDatabaseDir
if (Test-Path -LiteralPath $controllerDatabaseLock -PathType Leaf) {
    Remove-Item -LiteralPath $controllerDatabaseLock -Force
}
if (Test-Path -LiteralPath $controllerDatabaseDir) {
    throw "failed to remove stale controller state dir: $controllerDatabaseDir"
}
Remove-TreeIfPresent -Path $workerRuntimeDir
New-Item -ItemType Directory -Force -Path $controllerLogRoot | Out-Null
foreach ($dir in @($workerRuntimeDir, $workerLogDir, $workerTmpDir, $workerDataDir)) {
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
}
New-Item -ItemType Directory -Force -Path $python3ShimDir | Out-Null
Set-Content -LiteralPath $python3ShimPath -Value ("@echo off`r`n""{0}"" %*`r`n" -f $pythonExecutable) -Encoding ascii
Set-Content -LiteralPath $pythonSmokeWrapperPath -Value ("@echo off`r`necho goet smoke stdout`r`necho goet smoke stderr 1>&2`r`n""{0}"" %*`r`nexit /b %ERRORLEVEL%`r`n" -f $pythonExecutable) -Encoding ascii
$env:PATH = $python3ShimDir + ';' + $env:PATH

$workerSmokeConfigBody = [pscustomobject]@{
    log_dir           = $workerLogDir
    tmp_dir           = $workerTmpDir
    data_dir          = $workerDataDir
    controller_url    = $controllerBaseUrl
    python_executable = $pythonSmokeWrapperPath
} | ConvertTo-Json
[System.IO.File]::WriteAllText($workerSmokeConfig, $workerSmokeConfigBody, [System.Text.UTF8Encoding]::new($false))

$go = Get-Command go -ErrorAction Stop
$controllerArgs = @('run', './cmd/controller', '--config', './cmd/controller/demo-config.json')
$controllerProcess = Start-Process -FilePath $go.Path -ArgumentList $controllerArgs -WorkingDirectory $repoRoot -PassThru -WindowStyle Hidden -RedirectStandardOutput $controllerStdout -RedirectStandardError $controllerStderr

try {
    Wait-ForHttpGet -Uri ($controllerBaseUrl + '/healthz') -ExpectedStatusCode 204 -TimeoutSeconds 300
    Wait-ForHttpGet -Uri ($controllerBaseUrl + '/status') -ExpectedStatusCode 200 -TimeoutSeconds 300

    Write-Host "Submitting python-hello workflow..."
    $submission = Get-Content -Raw -LiteralPath $submissionJson | ConvertFrom-Json
    $override = [pscustomobject]@{
        name = [pscustomobject]@{
            namespace = 'worker_config'
            key = 'worker_max_count'
        }
        type = 'int'
        expression = 0
    }
    $submission.variables = @($submission.variables) + @($override)
    $submissionBody = $submission | ConvertTo-Json -Depth 20
    $submitResponse = Invoke-CurlRequest -Method Post -Uri ($controllerBaseUrl + '/workflow') -Body $submissionBody
    if ([int]$submitResponse.StatusCode -notin @(200, 201, 202)) {
        throw "workflow submission returned HTTP $([int]$submitResponse.StatusCode), want 200/201/202; body: $($submitResponse.Body)"
    }

    $submissionId = Parse-SubmissionId -Response $submitResponse
    if (-not $submissionId) {
        throw "submission response did not include a submission_id or submission location"
    }

    Write-Host "Submission ID: $submissionId"

    Write-Host "Starting worker with absolute config path..."
    $workerOutput = & $go.Path run ./cmd/worker $workerSmokeConfig

    $finalStatus = Wait-ForControllerIdle -ControllerUrl $controllerBaseUrl -TimeoutSeconds 300
    Write-Host ("Controller idle: pending={0}, assigned={1}" -f $finalStatus.pending, $finalStatus.assigned)

    Assert-DirectoryExists -Path $workerDataDir -Label 'worker data dir'
    $expectedOutputPath = Join-Path $workerDataDir 'python-hello-hello.json'
    Assert-LeafExists -Path $expectedOutputPath -Label 'worker output'

    $output = Get-Content -Raw -LiteralPath $expectedOutputPath | ConvertFrom-Json
    Assert-OutputFields -Output $output

    Write-Host "Verifying controller logs for submission..."
    $logsPayload = & $go.Path run ./cmd/demo-client logs $submissionId --controller-url $controllerBaseUrl --json
    if ($LASTEXITCODE -ne 0) {
        throw "goet logs failed for submission_id=$submissionId"
    }
    $logs = $logsPayload | ConvertFrom-Json
    if (($logs | Get-Member -Name entries) -eq $null -or $logs.entries.Count -eq 0) {
        throw "goet logs returned no entries for submission_id=$submissionId"
    }

    $stdoutOrStderrEntries = @($logs.entries | Where-Object { $_.stream -eq 'stdout' -or $_.stream -eq 'stderr' })
    if ($stdoutOrStderrEntries.Count -eq 0) {
        throw "goet logs for submission_id=$submissionId did not include stdout/stderr entries"
    }

    Write-Host "Output file: $expectedOutputPath"
    Write-Host "Submission logs entries: $($logs.entries.Count)"
    Write-Host "Controller log: $controllerStdout"
    if ($workerOutput) {
        Write-Host "Worker output:"
        $workerOutput | Write-Host
    }
    Write-Host "Smoke path completed."
} finally {
    try {
        [void](Invoke-CurlRequest -Method Post -Uri ($controllerBaseUrl + '/shutdown'))
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
