[CmdletBinding()]
param(
    [switch]$Help
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Write-Usage {
    @'
Usage:
  powershell -NoProfile -File scripts/dependency-aware-workflow-smoke.ps1
  powershell -NoProfile -File scripts/dependency-aware-workflow-smoke.ps1 -Help

What it does:
  - validates the sibling ../go-etl-demo-project fixture
  - creates temporary dependency-aware workflow fixtures in that sibling repo
  - starts the controller from cmd/controller/demo-config.json
  - submits a two-stage sequential workflow
  - proves stage 1 is not assignable until stage 0 completes
  - submits a contiguous parallel_with workflow
  - proves sibling parallel-stage work is assignable together
  - submits an invalid non-contiguous parallel_with workflow
  - proves invalid submission is rejected without queue mutation
  - verifies dependency transition observations through goet logs
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
        [string]$Path,

        [Parameter(Mandatory = $true)]
        [string]$AllowedRoot
    )

    if (-not (Test-Path -LiteralPath $Path)) {
        return
    }

    $fullPath = [System.IO.Path]::GetFullPath($Path)
    $fullRoot = [System.IO.Path]::GetFullPath($AllowedRoot)
    if (-not $fullPath.StartsWith($fullRoot, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "refusing to remove path outside allowed root: $fullPath"
    }

    $deadline = (Get-Date).AddSeconds(30)
    while ($true) {
        try {
            Remove-Item -LiteralPath $fullPath -Recurse -Force
            return
        } catch {
            if ((Get-Date) -ge $deadline) {
                throw
            }
            Start-Sleep -Milliseconds 500
        }
    }
}

function Write-JsonFile {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,

        [Parameter(Mandatory = $true)]
        [string]$Json
    )

    $parent = Split-Path -Parent $Path
    New-Item -ItemType Directory -Force -Path $parent | Out-Null
    [System.IO.File]::WriteAllText($Path, $Json.Trim() + "`n", [System.Text.UTF8Encoding]::new($false))
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
    $bodyPath = $null
    try {
        $curlArgs = @('-s', '-X', $Method.ToUpperInvariant(), '-o', $responsePath, '-w', '%{http_code}')
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

        return [pscustomobject]@{
            StatusCode = $statusCode
            Body       = $bodyText
        }
    } finally {
        if ($bodyPath -and (Test-Path -LiteralPath $bodyPath)) {
            Remove-Item -LiteralPath $bodyPath -Force
        }
        if (Test-Path -LiteralPath $responsePath) {
            Remove-Item -LiteralPath $responsePath -Force
        }
    }
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

function Invoke-GoetJson {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$Arguments
    )

    $output = & $script:GoPath run ./cmd/demo-client @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "go run ./cmd/demo-client $($Arguments -join ' ') failed"
    }
    return ($output | Out-String) | ConvertFrom-Json
}

function Get-SubmissionStatus {
    param(
        [Parameter(Mandatory = $true)]
        [string]$SubmissionID
    )

    return Invoke-GoetJson -Arguments @('status', $SubmissionID, '--controller-url', $script:ControllerBaseUrl, '--json')
}

function Get-SubmissionLogs {
    param(
        [Parameter(Mandatory = $true)]
        [string]$SubmissionID
    )

    return Invoke-GoetJson -Arguments @('logs', $SubmissionID, '--controller-url', $script:ControllerBaseUrl, '--json')
}

function Submit-WorkflowRun {
    param(
        [Parameter(Mandatory = $true)]
        [string]$WorkflowPath
    )

    $body = [pscustomobject]@{
        project = [pscustomobject]@{
            repository = 'local:demo'
            ref        = 'main'
            path       = 'project.json'
        }
        workflow = [pscustomobject]@{
            repository = 'local:demo'
            ref        = 'main'
            path       = $WorkflowPath
        }
        variables = @()
    } | ConvertTo-Json -Depth 20

    $response = Invoke-CurlRequest -Method Post -Uri ($script:ControllerBaseUrl + '/workflow') -Body $body
    if ([int]$response.StatusCode -ne 202) {
        throw "workflow submission returned HTTP $([int]$response.StatusCode), want 202; body: $($response.Body)"
    }
    return $response.Body | ConvertFrom-Json
}

function Submit-InvalidWorkflowRun {
    param(
        [Parameter(Mandatory = $true)]
        [string]$WorkflowPath
    )

    $body = [pscustomobject]@{
        project = [pscustomobject]@{
            repository = 'local:demo'
            ref        = 'main'
            path       = 'project.json'
        }
        workflow = [pscustomobject]@{
            repository = 'local:demo'
            ref        = 'main'
            path       = $WorkflowPath
        }
        variables = @()
    } | ConvertTo-Json -Depth 20

    return Invoke-CurlRequest -Method Post -Uri ($script:ControllerBaseUrl + '/workflow') -Body $body
}

function Get-NextWork {
    $response = Invoke-CurlRequest -Method Get -Uri ($script:ControllerBaseUrl + '/work/next')
    if ([int]$response.StatusCode -ne 200) {
        throw "next work returned HTTP $([int]$response.StatusCode), want 200; body: $($response.Body)"
    }
    return $response.Body | ConvertFrom-Json
}

function Complete-Work {
    param(
        [Parameter(Mandatory = $true)]
        [object]$WorkItem,

        [Parameter(Mandatory = $true)]
        [string]$OutputJson
    )

    $body = [pscustomobject]@{
        id             = $WorkItem.id
        attempt_id     = $WorkItem.attempt_id
        output_json    = $OutputJson
        pre_state_json = '{}'
        post_state_json = '{}'
        completed_at   = '2026-07-06T12:00:00Z'
    } | ConvertTo-Json -Depth 20

    $response = Invoke-CurlRequest -Method Post -Uri ($script:ControllerBaseUrl + '/work/complete') -Body $body
    if ([int]$response.StatusCode -ne 204) {
        throw "complete work $($WorkItem.id) returned HTTP $([int]$response.StatusCode), want 204; body: $($response.Body)"
    }
}

function Get-ControllerStatus {
    $response = Invoke-CurlRequest -Method Get -Uri ($script:ControllerBaseUrl + '/status')
    if ([int]$response.StatusCode -ne 200) {
        throw "controller status returned HTTP $([int]$response.StatusCode), want 200; body: $($response.Body)"
    }
    return $response.Body | ConvertFrom-Json
}

function Assert-DependencyStage {
    param(
        [Parameter(Mandatory = $true)]
        [object]$Status,

        [Parameter(Mandatory = $true)]
        [int]$StageIndex,

        [Parameter(Mandatory = $true)]
        [string]$State,

        [int]$AssignablePending = -1,

        [int]$BlockedFuture = -1,

        [int]$Active = -1
    )

    if (-not $Status.dependency) {
        throw "status for $($Status.submission_id) did not include dependency summary"
    }

    $stage = @($Status.dependency.stages | Where-Object { $_.stage_index -eq $StageIndex }) | Select-Object -First 1
    if (-not $stage) {
        throw "dependency stage $StageIndex missing"
    }
    if ($stage.state -ne $State) {
        throw "dependency stage $StageIndex state = $($stage.state), want $State"
    }
    if ($AssignablePending -ge 0 -and $stage.counts.assignable_pending -ne $AssignablePending) {
        throw "dependency stage $StageIndex assignable_pending = $($stage.counts.assignable_pending), want $AssignablePending"
    }
    if ($BlockedFuture -ge 0 -and $stage.counts.blocked_future -ne $BlockedFuture) {
        throw "dependency stage $StageIndex blocked_future = $($stage.counts.blocked_future), want $BlockedFuture"
    }
    if ($Active -ge 0 -and $stage.counts.active -ne $Active) {
        throw "dependency stage $StageIndex active = $($stage.counts.active), want $Active"
    }
}

function Assert-LogMessage {
    param(
        [Parameter(Mandatory = $true)]
        [object]$Logs,

        [Parameter(Mandatory = $true)]
        [string]$Contains
    )

    $match = @($Logs.entries | Where-Object { $_.message -like ('*' + $Contains + '*') }) | Select-Object -First 1
    if (-not $match) {
        throw "submission logs did not contain message fragment: $Contains"
    }
}

if ($Help) {
    Write-Usage
    exit 0
}

$scriptRoot = Split-Path -Parent $PSCommandPath
$repoRoot = Resolve-AbsolutePath (Join-Path $scriptRoot '..')
$demoRootCandidate = Join-Path $repoRoot '..\go-etl-demo-project'
Assert-DirectoryExists -Path $demoRootCandidate -Label 'sibling demo project'
$demoRoot = Resolve-AbsolutePath $demoRootCandidate

Set-Location $repoRoot

Assert-LeafExists -Path (Join-Path $repoRoot 'cmd\controller\demo-config.json') -Label 'controller config'
Assert-LeafExists -Path (Join-Path $demoRoot 'project.json') -Label 'demo project'

$script:GoPath = (Get-Command go -ErrorAction Stop).Path
$script:ControllerBaseUrl = 'http://localhost:8080'
$runID = (Get-Date -Format 'yyyyMMdd-HHmmss') + '-pid' + $PID
$smokeRelativeRoot = ".run/dependency-aware-workflow-smoke/$runID"
$smokeRoot = Join-Path $demoRoot ($smokeRelativeRoot -replace '/', '\')
$controllerLogRoot = Join-Path $repoRoot ('.run\dependency-aware-workflow-smoke\' + $runID)
$controllerStdout = Join-Path $controllerLogRoot 'controller.stdout.log'
$controllerStderr = Join-Path $controllerLogRoot 'controller.stderr.log'
$controllerConfigRelative = ".run\dependency-aware-workflow-smoke\$runID\controller-config.json"
$controllerConfig = Join-Path $repoRoot $controllerConfigRelative
$controllerDatabaseRelative = ".run/dependency-aware-workflow-smoke/$runID/workflow-execution.sqlite"

try {
    [void](Invoke-CurlRequest -Method Post -Uri ($script:ControllerBaseUrl + '/shutdown'))
    Wait-ForControllerPortClosed -Port 8080 -TimeoutSeconds 15
} catch {
    # Ignore stale or absent controllers before the smoke starts.
}

Remove-TreeIfPresent -Path $controllerLogRoot -AllowedRoot $repoRoot
Remove-TreeIfPresent -Path $smokeRoot -AllowedRoot $demoRoot
New-Item -ItemType Directory -Force -Path $controllerLogRoot | Out-Null

Write-JsonFile -Path $controllerConfig -Json @"
{
  "api_version": "goet/v1alpha1",
  "kind": "Controller",
  "variables": [
    {
      "name": {"namespace": "controller_config", "key": "controller_url"},
      "type": "string",
      "expression": "$script:ControllerBaseUrl"
    },
    {
      "name": {"namespace": "controller_config", "key": "main_database_driver"},
      "type": "string",
      "expression": "sqlite"
    },
    {
      "name": {"namespace": "controller_config", "key": "main_database_connection_string"},
      "type": "string",
      "expression": "$controllerDatabaseRelative"
    }
  ]
}
"@
Copy-Item -LiteralPath (Join-Path $repoRoot 'cmd\controller\defaults.json') -Destination (Join-Path $controllerLogRoot 'defaults.json') -Force

$sequentialWorkflowPath = "$smokeRelativeRoot/sequential.json"
$parallelWorkflowPath = "$smokeRelativeRoot/parallel-valid.json"
$invalidWorkflowPath = "$smokeRelativeRoot/parallel-invalid.json"

Write-JsonFile -Path (Join-Path $demoRoot ($sequentialWorkflowPath -replace '/', '\')) -Json @'
{
  "workflow": {
    "ID": "dependency-smoke-sequential",
    "Variables": [
      {
        "name": {"namespace": "workflow", "key": "years"},
        "type": "list",
        "expression": [{"type": "int", "expression": 2024}]
      }
    ],
    "Steps": [
      {
        "ID": "prepare",
        "FanOut": {
          "WorkItem": {
            "FanOutExpression": "${years[*]}",
            "Type": "write_demo_output",
            "OutputPrefix": "dependency-smoke-prepare",
            "OutputExtension": ".txt"
          }
        }
      },
      {
        "ID": "summarize",
        "FanOut": {
          "WorkItem": {
            "FanOutExpression": "${workflow.step[*]}",
            "TokenAccessor": ".next_year",
            "Type": "write_demo_output",
            "OutputPrefix": "dependency-smoke-summarize",
            "OutputExtension": ".txt"
          }
        }
      }
    ]
  },
  "variables": []
}
'@

Write-JsonFile -Path (Join-Path $demoRoot ($parallelWorkflowPath -replace '/', '\')) -Json @'
{
  "workflow": {
    "ID": "dependency-smoke-parallel",
    "Variables": [
      {
        "name": {"namespace": "workflow", "key": "years"},
        "type": "list",
        "expression": [{"type": "int", "expression": 2024}]
      }
    ],
    "Steps": [
      {
        "ID": "parallel-left",
        "parallel_with": "group-a",
        "FanOut": {
          "WorkItem": {
            "FanOutExpression": "${years[*]}",
            "Type": "write_demo_output",
            "OutputPrefix": "dependency-smoke-left",
            "OutputExtension": ".txt"
          }
        }
      },
      {
        "ID": "parallel-right",
        "parallel_with": "group-a",
        "FanOut": {
          "WorkItem": {
            "FanOutExpression": "${years[*]}",
            "Type": "write_demo_output",
            "OutputPrefix": "dependency-smoke-right",
            "OutputExtension": ".txt"
          }
        }
      }
    ]
  },
  "variables": []
}
'@

Write-JsonFile -Path (Join-Path $demoRoot ($invalidWorkflowPath -replace '/', '\')) -Json @'
{
  "workflow": {
    "ID": "dependency-smoke-invalid-parallel",
    "Variables": [
      {
        "name": {"namespace": "workflow", "key": "years"},
        "type": "list",
        "expression": [{"type": "int", "expression": 2024}]
      }
    ],
    "Steps": [
      {
        "ID": "parallel-a",
        "parallel_with": "group-a",
        "FanOut": {
          "WorkItem": {
            "FanOutExpression": "${years[*]}",
            "Type": "write_demo_output",
            "OutputPrefix": "dependency-smoke-a",
            "OutputExtension": ".txt"
          }
        }
      },
      {
        "ID": "middle",
        "FanOut": {
          "WorkItem": {
            "FanOutExpression": "${years[*]}",
            "Type": "write_demo_output",
            "OutputPrefix": "dependency-smoke-middle",
            "OutputExtension": ".txt"
          }
        }
      },
      {
        "ID": "parallel-a-again",
        "parallel_with": "group-a",
        "FanOut": {
          "WorkItem": {
            "FanOutExpression": "${years[*]}",
            "Type": "write_demo_output",
            "OutputPrefix": "dependency-smoke-a-again",
            "OutputExtension": ".txt"
          }
        }
      }
    ]
  },
  "variables": []
}
'@

$controllerArgs = @('run', './cmd/controller', '--config', $controllerConfigRelative)
$controllerProcess = Start-Process -FilePath $script:GoPath -ArgumentList $controllerArgs -WorkingDirectory $repoRoot -PassThru -WindowStyle Hidden -RedirectStandardOutput $controllerStdout -RedirectStandardError $controllerStderr

try {
    Wait-ForHttpGet -Uri ($script:ControllerBaseUrl + '/healthz') -ExpectedStatusCode 204 -TimeoutSeconds 300
    Wait-ForHttpGet -Uri ($script:ControllerBaseUrl + '/status') -ExpectedStatusCode 200 -TimeoutSeconds 300

    Write-Host "Submitting sequential dependency workflow..."
    $sequentialAck = Submit-WorkflowRun -WorkflowPath $sequentialWorkflowPath
    if ($sequentialAck.initial_work_item_count -ne 1) {
        throw "sequential initial_work_item_count = $($sequentialAck.initial_work_item_count), want 1"
    }
    $sequentialStatus = Get-SubmissionStatus -SubmissionID $sequentialAck.submission_id
    Assert-DependencyStage -Status $sequentialStatus -StageIndex 0 -State 'ready' -AssignablePending 1
    Assert-DependencyStage -Status $sequentialStatus -StageIndex 1 -State 'blocked' -BlockedFuture 1

    $stage0Work = Get-NextWork
    if ($stage0Work.step_definition_id -ne 'prepare') {
        throw "first sequential assignment step = $($stage0Work.step_definition_id), want prepare"
    }
    Complete-Work -WorkItem $stage0Work -OutputJson '{"next_year":2025}'

    $sequentialStatus = Get-SubmissionStatus -SubmissionID $sequentialAck.submission_id
    Assert-DependencyStage -Status $sequentialStatus -StageIndex 0 -State 'completed'
    Assert-DependencyStage -Status $sequentialStatus -StageIndex 1 -State 'ready' -AssignablePending 1

    $stage1Work = Get-NextWork
    if ($stage1Work.step_definition_id -ne 'summarize' -or $stage1Work.id -notlike '*2025*') {
        throw "activated sequential assignment = $($stage1Work.id) / $($stage1Work.step_definition_id), want summarize item from workflow.step output"
    }
    Complete-Work -WorkItem $stage1Work -OutputJson '{"done":true}'

    $sequentialStatus = Get-SubmissionStatus -SubmissionID $sequentialAck.submission_id
    if ($sequentialStatus.status -ne 'completed') {
        throw "sequential status = $($sequentialStatus.status), want completed"
    }
    $sequentialLogs = Get-SubmissionLogs -SubmissionID $sequentialAck.submission_id
    Assert-LogMessage -Logs $sequentialLogs -Contains 'normalized workflow into 2 stages'
    Assert-LogMessage -Logs $sequentialLogs -Contains 'queued stage 0'
    Assert-LogMessage -Logs $sequentialLogs -Contains 'completed stage 0'
    Assert-LogMessage -Logs $sequentialLogs -Contains 'activated stage 1'

    Write-Host "Submitting valid contiguous parallel_with workflow..."
    $parallelAck = Submit-WorkflowRun -WorkflowPath $parallelWorkflowPath
    if ($parallelAck.initial_work_item_count -ne 2) {
        throw "parallel initial_work_item_count = $($parallelAck.initial_work_item_count), want 2"
    }
    $parallelStatus = Get-SubmissionStatus -SubmissionID $parallelAck.submission_id
    Assert-DependencyStage -Status $parallelStatus -StageIndex 0 -State 'ready' -AssignablePending 2

    $parallelLeft = Get-NextWork
    $parallelRight = Get-NextWork
    $parallelSteps = @($parallelLeft.step_definition_id, $parallelRight.step_definition_id) | Sort-Object
    if (($parallelSteps -join ',') -ne 'parallel-left,parallel-right') {
        throw "parallel assignments = $($parallelSteps -join ','), want parallel-left,parallel-right"
    }
    if ($parallelLeft.workflow_instance_id -ne $parallelRight.workflow_instance_id) {
        throw "parallel assignments came from different workflow instances"
    }
    Complete-Work -WorkItem $parallelLeft -OutputJson '{"left":true}'
    Complete-Work -WorkItem $parallelRight -OutputJson '{"right":true}'

    $parallelStatus = Get-SubmissionStatus -SubmissionID $parallelAck.submission_id
    if ($parallelStatus.status -ne 'completed') {
        throw "parallel status = $($parallelStatus.status), want completed"
    }

    Write-Host "Submitting invalid non-contiguous parallel_with workflow..."
    $beforeInvalid = Get-ControllerStatus
    $invalidResponse = Submit-InvalidWorkflowRun -WorkflowPath $invalidWorkflowPath
    if ([int]$invalidResponse.StatusCode -eq 202) {
        throw "invalid non-contiguous parallel_with workflow was accepted"
    }
    $afterInvalid = Get-ControllerStatus
    if ($afterInvalid.pending -ne $beforeInvalid.pending -or $afterInvalid.assigned -ne $beforeInvalid.assigned -or $afterInvalid.failed -ne $beforeInvalid.failed) {
        throw "invalid submission mutated queue counts; before=$($beforeInvalid | ConvertTo-Json -Compress) after=$($afterInvalid | ConvertTo-Json -Compress)"
    }

    Write-Host "Sequential submission: $($sequentialAck.submission_id)"
    Write-Host "Parallel submission: $($parallelAck.submission_id)"
    Write-Host "Controller log: $controllerStdout"
    Write-Host "Dependency-aware workflow smoke completed."
} finally {
    try {
        [void](Invoke-CurlRequest -Method Post -Uri ($script:ControllerBaseUrl + '/shutdown'))
    } catch {
        # Fall back to process cleanup below.
    }

    if ($controllerProcess -and -not $controllerProcess.HasExited) {
        try {
            Wait-Process -Id $controllerProcess.Id -Timeout 30
        } catch {
            Stop-Process -Id $controllerProcess.Id -Force
        }
    }

    try {
        Remove-TreeIfPresent -Path $smokeRoot -AllowedRoot $demoRoot
    } catch {
        Write-Warning "failed to remove temporary demo fixture root ${smokeRoot}: $_"
    }
}
