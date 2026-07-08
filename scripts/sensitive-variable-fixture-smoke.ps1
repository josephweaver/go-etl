[CmdletBinding()]
param(
    [switch]$Help
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Write-Usage {
    @'
Usage:
  pwsh -NoProfile -File scripts/sensitive-variable-fixture-smoke.ps1
  powershell -NoProfile -ExecutionPolicy Bypass -File scripts/sensitive-variable-fixture-smoke.ps1
  pwsh -NoProfile -File scripts/sensitive-variable-fixture-smoke.ps1 -Help

What it does:
  - creates a temporary credentialed Python fixture in ../go-etl-demo-project
  - starts the controller without GOET_FIXTURE_TOKEN
  - submits a workflow containing protected_ref worker_env:GOET_FIXTURE_TOKEN
  - starts one worker with GOET_FIXTURE_TOKEN set only for that worker process
  - verifies the fixture used the secret without persisting or reporting it raw
  - verifies stdout/stderr and controller logs contain the redaction label
  - scans the controller SQLite file for the raw fixture secret
'@ | Write-Host
}

function Resolve-AbsolutePath {
    param([Parameter(Mandatory = $true)][string]$Path)
    return (Resolve-Path -LiteralPath $Path).Path
}

function Assert-LeafExists {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$Label
    )
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "$Label missing: $Path"
    }
}

function Assert-DirectoryExists {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$Label
    )
    if (-not (Test-Path -LiteralPath $Path -PathType Container)) {
        throw "$Label missing: $Path"
    }
}

function Remove-TreeIfPresent {
    param([Parameter(Mandatory = $true)][string]$Path)
    if (Test-Path -LiteralPath $Path) {
        Remove-Item -LiteralPath $Path -Recurse -Force
    }
}

function Invoke-CurlRequest {
    param(
        [Parameter(Mandatory = $true)][string]$Method,
        [Parameter(Mandatory = $true)][string]$Uri,
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

        $statusText = & curl.exe @curlArgs $Uri 2>$null
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
    param([Parameter(Mandatory = $true)][pscustomobject]$Response)

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
        [Parameter(Mandatory = $true)][string]$Uri,
        [Parameter(Mandatory = $true)][int]$ExpectedStatusCode,
        [Parameter(Mandatory = $true)][int]$TimeoutSeconds
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

function Get-ControllerStatusResponse {
    param([Parameter(Mandatory = $true)][string]$ControllerUrl)
    return Invoke-CurlRequest -Method Get -Uri ($ControllerUrl.TrimEnd('/') + '/status')
}

function Wait-ForControllerIdle {
    param(
        [Parameter(Mandatory = $true)][string]$ControllerUrl,
        [Parameter(Mandatory = $true)][int]$TimeoutSeconds
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    $lastStatus = $null
    while ((Get-Date) -lt $deadline) {
        $response = Get-ControllerStatusResponse -ControllerUrl $ControllerUrl
        $status = $response.Body | ConvertFrom-Json
        $lastStatus = $status
        if ($status.pending -eq 0 -and $status.assigned -eq 0) {
            return $response
        }
        Start-Sleep -Milliseconds 500
    }

    throw "controller did not become idle within $TimeoutSeconds seconds; last status: $($lastStatus | ConvertTo-Json -Compress)"
}

function Wait-ForControllerPortClosed {
    param(
        [Parameter(Mandatory = $true)][int]$Port,
        [Parameter(Mandatory = $true)][int]$TimeoutSeconds
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

function Assert-TextDoesNotContain {
    param(
        [Parameter(Mandatory = $true)][string]$Text,
        [Parameter(Mandatory = $true)][string]$Needle,
        [Parameter(Mandatory = $true)][string]$Label
    )
    if ($Text.Contains($Needle)) {
        throw "$Label leaked raw fixture secret"
    }
}

function Assert-TextContains {
    param(
        [Parameter(Mandatory = $true)][string]$Text,
        [Parameter(Mandatory = $true)][string]$Needle,
        [Parameter(Mandatory = $true)][string]$Label
    )
    if (-not $Text.Contains($Needle)) {
        throw "$Label missing expected text: $Needle"
    }
}

function Assert-FileBytesDoNotContain {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$Needle,
        [Parameter(Mandatory = $true)][string]$Label
    )
    $stream = [System.IO.File]::Open($Path, [System.IO.FileMode]::Open, [System.IO.FileAccess]::Read, [System.IO.FileShare]::ReadWrite)
    try {
        $memory = New-Object System.IO.MemoryStream
        try {
            $stream.CopyTo($memory)
            $bytes = $memory.ToArray()
        } finally {
            $memory.Dispose()
        }
    } finally {
        $stream.Dispose()
    }
    $needleBytes = [System.Text.Encoding]::UTF8.GetBytes($Needle)
    if ($needleBytes.Length -eq 0 -or $bytes.Length -lt $needleBytes.Length) {
        return
    }

    for ($i = 0; $i -le ($bytes.Length - $needleBytes.Length); $i++) {
        $matched = $true
        for ($j = 0; $j -lt $needleBytes.Length; $j++) {
            if ($bytes[$i + $j] -ne $needleBytes[$j]) {
                $matched = $false
                break
            }
        }
        if ($matched) {
            throw "$Label leaked raw fixture secret"
        }
    }
}

function Assert-FileBytesContain {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$Needle,
        [Parameter(Mandatory = $true)][string]$Label
    )
    $stream = [System.IO.File]::Open($Path, [System.IO.FileMode]::Open, [System.IO.FileAccess]::Read, [System.IO.FileShare]::ReadWrite)
    try {
        $memory = New-Object System.IO.MemoryStream
        try {
            $stream.CopyTo($memory)
            $text = [System.Text.Encoding]::UTF8.GetString($memory.ToArray())
        } finally {
            $memory.Dispose()
        }
    } finally {
        $stream.Dispose()
    }
    Assert-TextContains -Text $text -Needle $Needle -Label $Label
}

function Test-JsonSyntax {
    param([Parameter(Mandatory = $true)][string]$Path)

    $python3 = Get-Command python3 -ErrorAction SilentlyContinue
    if ($python3) {
        & $python3.Path -m json.tool $Path | Out-Null
        return
    }

    $python = Get-Command python -ErrorAction SilentlyContinue
    if ($python) {
        & $python.Path -m json.tool $Path | Out-Null
        return
    }

    Get-Content -Raw -LiteralPath $Path | ConvertFrom-Json | Out-Null
}

function Write-FixtureFiles {
    param(
        [Parameter(Mandatory = $true)][string]$DemoRoot,
        [Parameter(Mandatory = $true)][string]$ExpectedSHA256
    )

    $scriptPath = Join-Path $DemoRoot 'scripts\sensitive_variable_fixture.py'
    $workflowPath = Join-Path $DemoRoot 'workflows\sensitive-variable-fixture.json'
    $submissionPath = Join-Path $DemoRoot 'submissions\sensitive-variable-fixture-local.json'

    $pythonSource = @'
#!/usr/bin/env python3

import hashlib
import json
import os
import sys


def parameter_value(input_document, name):
    return input_document["work_item"]["parameters"][name]["value"]


def main() -> int:
    output_path = os.environ.get("GOET_OUTPUT_JSON", "").strip()
    if not output_path:
        print("GOET_OUTPUT_JSON is required", file=sys.stderr)
        return 1

    input_path = os.environ.get("GOET_INPUT_JSON", "").strip()
    with open(input_path, "r", encoding="utf-8") as handle:
        input_document = json.load(handle)

    secret = os.environ.get("FIXTURE_TOKEN", "")
    expected_sha256 = parameter_value(input_document, "expected_fixture_token_sha256")
    observed_sha256 = hashlib.sha256(secret.encode("utf-8")).hexdigest() if secret else ""

    print("fixture stdout secret=" + secret)
    print("fixture stderr secret=" + secret, file=sys.stderr)

    secret_reached_argv = bool(secret) and any(secret in arg for arg in sys.argv)
    secret_reached_input = bool(secret) and secret in json.dumps(input_document, sort_keys=True)

    result = {
        "status": "ok",
        "fixture_token_available": bool(secret),
        "fixture_token_sha256_matches": observed_sha256 == expected_sha256,
        "fixture_token_length": len(secret),
        "secret_reached_argv": secret_reached_argv,
        "secret_reached_input_json": secret_reached_input,
        "boundary_proof": "worker_env protected ref resolved by worker fixture only"
    }

    with open(output_path, "w", encoding="utf-8") as handle:
        json.dump(result, handle, indent=2, sort_keys=True)
        handle.write("\n")

    if not result["fixture_token_available"] or not result["fixture_token_sha256_matches"]:
        return 2
    if secret_reached_argv or secret_reached_input:
        return 3
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
'@

    $workflow = [ordered]@{
        workflow = [ordered]@{
            ID = 'sensitive-variable-fixture'
            Variables = @(
                [ordered]@{
                    name = [ordered]@{ namespace = 'workflow'; key = 'fixture_runs' }
                    type = 'list'
                    expression = @(
                        [ordered]@{
                            type = 'object'
                            expression = [ordered]@{
                                id = [ordered]@{ type = 'string'; expression = 'check' }
                            }
                        }
                    )
                }
            )
            Steps = @(
                [ordered]@{
                    ID = 'credential-boundary'
                    FanOut = [ordered]@{
                        WorkItem = [ordered]@{
                            FanOutExpression = '${fixture_runs[*]}'
                            IDTokenAccessor = '.id'
                            OutputAccessor = '.id'
                            Type = 'python_script'
                            OutputPrefix = 'sensitive-variable-fixture'
                            OutputExtension = '.json'
                            Parameters = [ordered]@{
                                python_entrypoint = [ordered]@{ type = 'path'; value = 'scripts/sensitive_variable_fixture.py' }
                                python_environment = [ordered]@{ type = 'path'; value = 'environments/system-python.json' }
                                expected_fixture_token_sha256 = [ordered]@{ type = 'string'; value = $ExpectedSHA256 }
                                fixture_token = [ordered]@{
                                    type = 'string'
                                    protected_ref = [ordered]@{
                                        provider = 'worker_env'
                                        key = 'GOET_FIXTURE_TOKEN'
                                    }
                                    materialize = [ordered]@{
                                        mode = 'env'
                                        target = 'FIXTURE_TOKEN'
                                    }
                                }
                            }
                        }
                    }
                }
            )
        }
        source_manifest = [ordered]@{
            files = @(
                [ordered]@{ role = 'python_entrypoint'; path = 'scripts/sensitive_variable_fixture.py'; content_type = 'text/x-python' },
                [ordered]@{ role = 'python_environment'; path = 'environments/system-python.json'; content_type = 'application/json' }
            )
        }
        variables = @(
            [ordered]@{
                name = [ordered]@{ namespace = 'worker_config'; key = 'worker_target_environment' }
                type = 'string'
                expression = 'local'
            },
            [ordered]@{
                name = [ordered]@{ namespace = 'worker_config'; key = 'worker_start_executable' }
                type = 'string'
                expression = 'go'
            },
            [ordered]@{
                name = [ordered]@{ namespace = 'worker_config'; key = 'worker_start_args' }
                type = 'list'
                expression = @(
                    [ordered]@{ type = 'string'; expression = 'run' },
                    [ordered]@{ type = 'string'; expression = './cmd/worker' },
                    [ordered]@{ type = 'string'; expression = './cmd/worker/demo-config.json' }
                )
            },
            [ordered]@{
                name = [ordered]@{ namespace = 'worker_config'; key = 'worker_min_count' }
                type = 'int'
                expression = 1
            },
            [ordered]@{
                name = [ordered]@{ namespace = 'worker_config'; key = 'worker_max_count' }
                type = 'int'
                expression = 1
            },
            [ordered]@{
                name = [ordered]@{ namespace = 'worker_config'; key = 'worker_count_per_start' }
                type = 'int'
                expression = 1
            },
            [ordered]@{
                name = [ordered]@{ namespace = 'worker_config'; key = 'worker_min_elapsed_time_between_starts' }
                type = 'string'
                expression = '0s'
            }
        )
    }

    $submission = [ordered]@{
        project = [ordered]@{ repository = 'local:demo'; ref = 'main'; path = 'project.json' }
        workflow = [ordered]@{ repository = 'local:demo'; ref = 'main'; path = 'workflows/sensitive-variable-fixture.json' }
        variables = @()
    }

    [System.IO.File]::WriteAllText($scriptPath, $pythonSource, [System.Text.UTF8Encoding]::new($false))
    [System.IO.File]::WriteAllText($workflowPath, ($workflow | ConvertTo-Json -Depth 50), [System.Text.UTF8Encoding]::new($false))
    [System.IO.File]::WriteAllText($submissionPath, ($submission | ConvertTo-Json -Depth 20), [System.Text.UTF8Encoding]::new($false))

    return [pscustomobject]@{
        ScriptPath = $scriptPath
        WorkflowPath = $workflowPath
        SubmissionPath = $submissionPath
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

$fixtureSecret = 'goet-fixture-secret-008-do-not-persist'
$redactionLabel = '${worker_env.GOET_FIXTURE_TOKEN}'
$sha256 = [System.Security.Cryptography.SHA256]::Create()
try {
    $secretHashBytes = $sha256.ComputeHash([System.Text.Encoding]::UTF8.GetBytes($fixtureSecret))
} finally {
    $sha256.Dispose()
}
$expectedSHA256 = [System.BitConverter]::ToString($secretHashBytes).Replace('-', '').ToLowerInvariant()

$controllerConfig = Join-Path $repoRoot 'cmd\controller\demo-config.json'
$workerConfig = Join-Path $repoRoot 'cmd\worker\demo-config.json'
$environmentJson = Join-Path $demoRoot 'environments\system-python.json'

Assert-LeafExists -Path $controllerConfig -Label 'controller config'
Assert-LeafExists -Path $workerConfig -Label 'worker config'
Assert-LeafExists -Path $environmentJson -Label 'python environment'

$fixtureFiles = Write-FixtureFiles -DemoRoot $demoRoot -ExpectedSHA256 $expectedSHA256
Test-JsonSyntax -Path $fixtureFiles.WorkflowPath
Test-JsonSyntax -Path $fixtureFiles.SubmissionPath

$pythonExecutable = Resolve-PythonExecutable
& $pythonExecutable -m py_compile $fixtureFiles.ScriptPath

$controllerLogRoot = Join-Path $repoRoot ('.run\sensitive-variable-fixture-smoke\' + (Get-Date -Format 'yyyyMMdd-HHmmss') + '-pid' + $PID)
$controllerStdout = Join-Path $controllerLogRoot 'controller.stdout.log'
$controllerStderr = Join-Path $controllerLogRoot 'controller.stderr.log'
$controllerDatabaseDir = Join-Path $repoRoot '.run\controller'
$controllerDatabasePath = Join-Path $controllerDatabaseDir 'workflow-execution.sqlite'
$controllerDatabaseLock = Join-Path $controllerDatabaseDir 'workflow-execution.sqlite.controller.lock'
$workerRuntimeDir = Join-Path $repoRoot 'cmd\worker\.run'
$workerLogDir = Join-Path $workerRuntimeDir 'logs'
$workerTmpDir = Join-Path $workerRuntimeDir 'tmp'
$workerDataDir = Join-Path $workerRuntimeDir 'data'
$workerSmokeConfig = Join-Path $repoRoot '.run\sensitive-variable-fixture-worker-config.json'
$python3ShimDir = Join-Path $repoRoot '.run\python3-shim'
$python3ShimPath = Join-Path $python3ShimDir 'python3.cmd'
$controllerBaseUrl = 'http://localhost:8080'
$previousFixtureToken = $env:GOET_FIXTURE_TOKEN

try {
    try {
        [void](Invoke-CurlRequest -Method Post -Uri ($controllerBaseUrl + '/shutdown'))
        Wait-ForControllerPortClosed -Port 8080 -TimeoutSeconds 15
    } catch {
        # Ignore stale or absent controllers.
    }

    Remove-TreeIfPresent -Path $controllerDatabaseDir
    if (Test-Path -LiteralPath $controllerDatabaseLock -PathType Leaf) {
        Remove-Item -LiteralPath $controllerDatabaseLock -Force
    }
    Remove-TreeIfPresent -Path $workerRuntimeDir
    New-Item -ItemType Directory -Force -Path $controllerLogRoot | Out-Null
    foreach ($dir in @($workerRuntimeDir, $workerLogDir, $workerTmpDir, $workerDataDir, $python3ShimDir)) {
        New-Item -ItemType Directory -Force -Path $dir | Out-Null
    }

    Set-Content -LiteralPath $python3ShimPath -Value ("@echo off`r`n""{0}"" %*`r`n" -f $pythonExecutable) -Encoding ascii
    $env:PATH = $python3ShimDir + ';' + $env:PATH

    $workerSmokeConfigBody = [pscustomobject]@{
        log_dir           = $workerLogDir
        tmp_dir           = $workerTmpDir
        data_dir          = $workerDataDir
        controller_url    = $controllerBaseUrl
        python_executable = $pythonExecutable
    } | ConvertTo-Json
    [System.IO.File]::WriteAllText($workerSmokeConfig, $workerSmokeConfigBody, [System.Text.UTF8Encoding]::new($false))

    $env:GOET_FIXTURE_TOKEN = $null
    $go = Get-Command go -ErrorAction Stop
    $controllerArgs = @('run', './cmd/controller', '--config', './cmd/controller/demo-config.json')
    $controllerProcess = Start-Process -FilePath $go.Path -ArgumentList $controllerArgs -WorkingDirectory $repoRoot -PassThru -WindowStyle Hidden -RedirectStandardOutput $controllerStdout -RedirectStandardError $controllerStderr

    try {
        Wait-ForHttpGet -Uri ($controllerBaseUrl + '/healthz') -ExpectedStatusCode 204 -TimeoutSeconds 300
        Wait-ForHttpGet -Uri ($controllerBaseUrl + '/status') -ExpectedStatusCode 200 -TimeoutSeconds 300

        $preSubmitStatus = Get-ControllerStatusResponse -ControllerUrl $controllerBaseUrl
        Assert-TextDoesNotContain -Text $preSubmitStatus.Body -Needle $fixtureSecret -Label 'controller status before submit'

        Write-Host "Submitting sensitive-variable fixture workflow..."
        $submission = Get-Content -Raw -LiteralPath $fixtureFiles.SubmissionPath | ConvertFrom-Json
        $override = [pscustomobject]@{
            name = [pscustomobject]@{ namespace = 'worker_config'; key = 'worker_max_count' }
            type = 'int'
            expression = 0
        }
        $submission.variables = @($submission.variables) + @($override)
        $submissionBody = $submission | ConvertTo-Json -Depth 20
        Assert-TextDoesNotContain -Text $submissionBody -Needle $fixtureSecret -Label 'submission payload'

        $submitResponse = Invoke-CurlRequest -Method Post -Uri ($controllerBaseUrl + '/workflow') -Body $submissionBody
        if ([int]$submitResponse.StatusCode -notin @(200, 201, 202)) {
            throw "workflow submission returned HTTP $([int]$submitResponse.StatusCode), want 200/201/202; body: $($submitResponse.Body)"
        }

        $submissionId = Parse-SubmissionId -Response $submitResponse
        if (-not $submissionId) {
            throw "submission response did not include a submission_id or submission location"
        }
        Write-Host "Submission ID: $submissionId"

        $postSubmitStatus = Get-ControllerStatusResponse -ControllerUrl $controllerBaseUrl
        Assert-TextDoesNotContain -Text $postSubmitStatus.Body -Needle $fixtureSecret -Label 'controller status after submit'

        Write-Host "Starting worker with GOET_FIXTURE_TOKEN set only in worker process environment..."
        $env:GOET_FIXTURE_TOKEN = $fixtureSecret
        $workerOutput = & $go.Path run ./cmd/worker $workerSmokeConfig
        $workerExit = $LASTEXITCODE
        $env:GOET_FIXTURE_TOKEN = $null
        if ($workerExit -ne 0) {
            throw "worker exited with code $workerExit"
        }
        if ($workerOutput) {
            Assert-TextDoesNotContain -Text ($workerOutput | Out-String) -Needle $fixtureSecret -Label 'worker process output'
        }

        $finalStatusResponse = Wait-ForControllerIdle -ControllerUrl $controllerBaseUrl -TimeoutSeconds 300
        Assert-TextDoesNotContain -Text $finalStatusResponse.Body -Needle $fixtureSecret -Label 'controller final status'
        $finalStatus = $finalStatusResponse.Body | ConvertFrom-Json
        if ($finalStatus.failed -ne 0 -or $finalStatus.pending -ne 0 -or $finalStatus.assigned -ne 0) {
            throw "unexpected controller final status: $($finalStatusResponse.Body)"
        }

        $submissionStatusPayload = & $go.Path run ./cmd/demo-client status $submissionId --controller-url $controllerBaseUrl --json
        if ($LASTEXITCODE -ne 0) {
            throw "goet status failed for submission_id=$submissionId"
        }
        $submissionStatusText = $submissionStatusPayload | Out-String
        Assert-TextDoesNotContain -Text $submissionStatusText -Needle $fixtureSecret -Label 'submission status'
        $submissionStatus = $submissionStatusText | ConvertFrom-Json
        if ($submissionStatus.status -ne 'completed' -or $submissionStatus.completed -ne 1 -or $submissionStatus.failed -ne 0) {
            throw "unexpected submission status: $submissionStatusText"
        }

        $expectedOutputPath = Join-Path $workerDataDir 'sensitive-variable-fixture-check.json'
        Assert-LeafExists -Path $expectedOutputPath -Label 'worker output'
        $outputText = Get-Content -Raw -LiteralPath $expectedOutputPath
        Assert-TextDoesNotContain -Text $outputText -Needle $fixtureSecret -Label 'worker output file'
        $output = $outputText | ConvertFrom-Json
        if ($output.fixture_token_available -ne $true -or $output.fixture_token_sha256_matches -ne $true) {
            throw "fixture did not prove worker-side secret access: $outputText"
        }
        if ($output.secret_reached_argv -ne $false -or $output.secret_reached_input_json -ne $false) {
            throw "fixture reported secret in argv or GOET_INPUT_JSON: $outputText"
        }

        $attemptDir = Get-ChildItem -LiteralPath (Join-Path $workerTmpDir 'attempts') -Directory | Sort-Object LastWriteTime -Descending | Select-Object -First 1
        if (-not $attemptDir) {
            throw "worker attempt directory missing"
        }
        $stdoutText = Get-Content -Raw -LiteralPath (Join-Path $attemptDir.FullName 'logs\stdout.log')
        $stderrText = Get-Content -Raw -LiteralPath (Join-Path $attemptDir.FullName 'logs\stderr.log')
        Assert-TextDoesNotContain -Text $stdoutText -Needle $fixtureSecret -Label 'captured stdout'
        Assert-TextDoesNotContain -Text $stderrText -Needle $fixtureSecret -Label 'captured stderr'
        Assert-TextContains -Text $stdoutText -Needle $redactionLabel -Label 'captured stdout'
        Assert-TextContains -Text $stderrText -Needle $redactionLabel -Label 'captured stderr'

        $logsPayload = & $go.Path run ./cmd/demo-client logs $submissionId --controller-url $controllerBaseUrl --json
        if ($LASTEXITCODE -ne 0) {
            throw "goet logs failed for submission_id=$submissionId"
        }
        $logsText = $logsPayload | Out-String
        Assert-TextDoesNotContain -Text $logsText -Needle $fixtureSecret -Label 'controller log endpoint'
        Assert-TextContains -Text $logsText -Needle $redactionLabel -Label 'controller log endpoint'

        Assert-LeafExists -Path $controllerDatabasePath -Label 'controller database'
        Assert-FileBytesDoNotContain -Path $controllerDatabasePath -Needle $fixtureSecret -Label 'controller SQLite persistence'
        Assert-FileBytesContain -Path $controllerDatabasePath -Needle $redactionLabel -Label 'controller SQLite persistence'

        Write-Host "Output file: $expectedOutputPath"
        Write-Host "Attempt logs: $($attemptDir.FullName)"
        Write-Host "Controller database: $controllerDatabasePath"
        Write-Host "Smoke path completed."
    } finally {
        if ($controllerProcess -and -not $controllerProcess.HasExited) {
            try {
                [void](Invoke-CurlRequest -Method Post -Uri ($controllerBaseUrl + '/shutdown'))
            } catch {
                # If shutdown is already in progress or the controller never came up,
                # fall back to process cleanup below.
            }
        }

        if ($controllerProcess -and -not $controllerProcess.HasExited) {
            try {
                Wait-Process -Id $controllerProcess.Id -Timeout 30
            } catch {
                Stop-Process -Id $controllerProcess.Id -Force
            }
        }
    }
} finally {
    $env:GOET_FIXTURE_TOKEN = $previousFixtureToken
    foreach ($path in @($fixtureFiles.ScriptPath, $fixtureFiles.WorkflowPath, $fixtureFiles.SubmissionPath)) {
        if ($path -and (Test-Path -LiteralPath $path -PathType Leaf)) {
            Remove-Item -LiteralPath $path -Force
        }
    }
}
