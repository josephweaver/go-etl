[CmdletBinding()]
param(
    [switch]$Help
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Write-Usage {
    @'
Usage:
  pwsh -NoProfile -File scripts/fake-hpcc-data-assets-smoke.ps1

What it does:
  - creates tiny local fixture data under .run/fake-hpcc-data-assets
  - creates temporary source files under ../go-etl-demo-project/.goetl-smoke
  - starts the controller with local transport, fake sbatch, and WorkerRuntime
  - submits a source-reference Python workflow
  - verifies artifact-manifest evidence, promoted artifact file, and published file

Prerequisites:
  - go
  - PowerShell 7+
  - bash available on PATH for scripts/fake-hpcc/sbatch
  - Compress-Archive
'@ | Write-Host
}

if ($Help) {
    Write-Usage
    return
}

function Convert-ToForwardSlashPath {
    param([Parameter(Mandatory = $true)][string]$Path)
    return [System.IO.Path]::GetFullPath($Path).Replace('\', '/')
}

function Write-JsonFile {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)]$Value
    )
    $json = $Value | ConvertTo-Json -Depth 40
    [System.IO.File]::WriteAllText($Path, $json + [Environment]::NewLine, [System.Text.UTF8Encoding]::new($false))
}

function Write-TextFile {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$Value
    )
    [System.IO.File]::WriteAllText($Path, $Value, [System.Text.UTF8Encoding]::new($false))
}

function Wait-ForController {
    param([string]$ControllerURL)
    $deadline = (Get-Date).AddSeconds(30)
    while ((Get-Date) -lt $deadline) {
        try {
            Invoke-RestMethod -Method Get -Uri "$ControllerURL/status" | Out-Null
            return
        } catch {
            Start-Sleep -Milliseconds 500
        }
    }
    throw "controller did not become reachable at $ControllerURL"
}

function Wait-ForSubmission {
    param(
        [string]$ControllerURL,
        [string]$SubmissionID
    )
    $deadline = (Get-Date).AddSeconds(90)
    while ((Get-Date) -lt $deadline) {
        $status = Invoke-RestMethod -Method Get -Uri "$ControllerURL/submissions/$SubmissionID/status"
        if ($status.status -eq 'completed') {
            return $status
        }
        if ($status.status -eq 'failed') {
            throw "submission $SubmissionID failed"
        }
        Start-Sleep -Seconds 1
    }
    throw "timed out waiting for submission $SubmissionID"
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$demoRoot = Join-Path (Split-Path -Parent $repoRoot) 'go-etl-demo-project'
if (-not (Test-Path -LiteralPath (Join-Path $demoRoot 'project.json') -PathType Leaf)) {
    throw "sibling demo project missing: $demoRoot"
}
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw 'go is required'
}
if (-not (Get-Command bash -ErrorAction SilentlyContinue)) {
    throw 'bash is required so fake sbatch can execute generated Slurm scripts'
}

$runRoot = Join-Path $repoRoot '.run\fake-hpcc-data-assets'
$runtimeRoot = Join-Path $runRoot 'runtime'
$workerDataRoot = Join-Path $runRoot 'worker-data'
$fixtureRoot = Join-Path $runRoot 'fixture-data'
$publishedRoot = Join-Path $runRoot 'published-data'
$slurmRunRoot = Join-Path $runRoot 'slurm'
$binRoot = Join-Path $runRoot 'bin'
$sourceRoot = Join-Path $demoRoot '.goetl-smoke\fake-hpcc-data-assets'
$sourceScriptDir = Join-Path $sourceRoot 'scripts'

if (Test-Path -LiteralPath $runRoot) {
    Remove-Item -LiteralPath $runRoot -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $runtimeRoot, $workerDataRoot, $fixtureRoot, $publishedRoot, $slurmRunRoot, $binRoot, $sourceScriptDir | Out-Null
Copy-Item -LiteralPath (Join-Path $repoRoot 'cmd\controller\defaults.json') -Destination (Join-Path $runRoot 'defaults.json') -Force

Write-TextFile -Path (Join-Path $fixtureRoot 'input.txt') -Value "smoke input$([Environment]::NewLine)"
$archiveSource = Join-Path $runRoot 'archive-source'
New-Item -ItemType Directory -Force -Path $archiveSource | Out-Null
Write-TextFile -Path (Join-Path $archiveSource 'selected-note.txt') -Value "archive note$([Environment]::NewLine)"
Compress-Archive -Path (Join-Path $archiveSource 'selected-note.txt') -DestinationPath (Join-Path $fixtureRoot 'archive.zip') -Force

$pythonPath = Join-Path $sourceScriptDir 'fake_hpcc_data_assets.py'
Write-TextFile -Path $pythonPath -Value @'
import argparse
import json
import os

parser = argparse.ArgumentParser()
parser.add_argument("--input", required=True)
parser.add_argument("--archive", required=True)
parser.add_argument("--out", required=True)
args = parser.parse_args()

with open(args.input, "r", encoding="utf-8") as handle:
    input_text = handle.read().strip()
with open(args.archive, "r", encoding="utf-8") as handle:
    archive_text = handle.read().strip()

os.makedirs(os.path.dirname(args.out), exist_ok=True)
with open(args.out, "w", encoding="utf-8") as handle:
    handle.write("source,content\n")
    handle.write(f"input,{input_text}\n")
    handle.write(f"archive,{archive_text}\n")

with open(os.environ["GOET_OUTPUT_JSON"], "w", encoding="utf-8") as handle:
    json.dump({
        "result": "ok",
        "artifacts": [{
            "name": "summary",
            "kind": "file",
            "format": "csv",
            "path": "reports/summary.csv"
        }]
    }, handle)
'@

$workflowPath = Join-Path $sourceRoot 'workflow.json'
$controllerConfigPath = Join-Path $runRoot 'controller.json'
$submissionPath = Join-Path $runRoot 'submission.json'
$controllerURL = 'http://localhost:8080'
$runtimeRootFwd = Convert-ToForwardSlashPath $runtimeRoot
$workerConfigPathFwd = Convert-ToForwardSlashPath (Join-Path $runtimeRoot 'config\worker.json')
$workerLogRootFwd = Convert-ToForwardSlashPath (Join-Path $runtimeRoot 'logs')
$workerScriptPathFwd = Convert-ToForwardSlashPath (Join-Path $runtimeRoot 'scripts\worker.slurm')
$workerDataRootFwd = Convert-ToForwardSlashPath $workerDataRoot
$fixtureRootFwd = Convert-ToForwardSlashPath $fixtureRoot
$publishedRootFwd = Convert-ToForwardSlashPath $publishedRoot
$assetCacheRootFwd = Convert-ToForwardSlashPath (Join-Path $runtimeRoot 'cache\assets')

$workerVariables = @(
    @{ name = @{ namespace = 'worker_config'; key = 'scheduler' }; type = 'object'; expression = @{
        type = @{ type = 'string'; expression = 'slurm' }
        settings = @{ type = 'object'; expression = @{
            script_path = @{ type = 'path'; expression = $workerScriptPathFwd }
            job_name = @{ type = 'string'; expression = 'goetl-worker' }
        }}
    }},
    @{ name = @{ namespace = 'worker_config'; key = 'runtime' }; type = 'object'; expression = @{
        type = @{ type = 'string'; expression = 'worker' }
        settings = @{ type = 'object'; expression = @{
            executable = @{ type = 'string'; expression = 'go' }
            args = @{ type = 'list'; expression = @(
                @{ type = 'string'; expression = 'run' },
                @{ type = 'string'; expression = './cmd/worker' }
            )}
            config_path = @{ type = 'path'; expression = $workerConfigPathFwd }
            log_dir = @{ type = 'path'; expression = $workerLogRootFwd }
        }}
    }},
    @{ name = @{ namespace = 'worker_config'; key = 'worker_min_count' }; type = 'int'; expression = 1 },
    @{ name = @{ namespace = 'worker_config'; key = 'worker_max_count' }; type = 'int'; expression = 1 },
    @{ name = @{ namespace = 'worker_config'; key = 'worker_count_per_start' }; type = 'int'; expression = 1 },
    @{ name = @{ namespace = 'worker_config'; key = 'worker_min_elapsed_time_between_starts' }; type = 'string'; expression = '0s' }
)

$workflow = @{
    workflow = @{
        ID = 'fake-hpcc-data-assets-smoke'
        Variables = @(
            @{ name = @{ namespace = 'workflow'; key = 'smoke_items' }; type = 'list'; expression = @(
                @{ type = 'object'; expression = @{ id = @{ type = 'string'; expression = 'smoke' } } }
            ) }
        )
        Steps = @(
            @{ ID = 'fake-hpcc-data-assets'; FanOut = @{ WorkItem = @{
                FanOutExpression = '${smoke_items[*]}'
                IDTokenAccessor = '.id'
                OutputAccessor = '.id'
                Type = 'python_script'
                OutputPrefix = 'fake-hpcc-data-assets'
                OutputExtension = '.json'
                Parameters = @{
                    python_entrypoint = @{ type = 'path'; value = '.goetl-smoke/fake-hpcc-data-assets/scripts/fake_hpcc_data_assets.py' }
                    python_args = @{ type = 'list'; value = @('--input', '${data.input_data.local_path}', '--archive', '${data.archived_note.local_path}', '--out', '${artifact_dir}/reports/summary.csv') }
                    data_assets = @{ type = 'data_assets'; value = @(
                        @{
                            binding_name = 'input_data'
                            provider_name = 'fixture_input'
                            kind = 'text'
                            format = 'txt'
                            provider = 'local_file'
                            location = @{ type = 'local_file'; location_name = 'fixture_data'; path = 'input.txt' }
                            materialization = @{ strategy = 'reference' }
                        },
                        @{
                            binding_name = 'archived_note'
                            provider_name = 'fixture_archive'
                            kind = 'text_archive'
                            format = 'zip'
                            provider = 'local_file'
                            location = @{ type = 'local_file'; location_name = 'fixture_data'; path = 'archive.zip' }
                            cache = @{ strategy = 'worker_cache'; cache_key = 'fake-hpcc-data-assets/archive.zip' }
                            archive = @{ type = 'zip'; select = @(@{ member = 'selected-note.txt'; as = 'note.txt'; required = $true }); expose = 'selected_path' }
                            materialization = @{ strategy = 'worker_cache' }
                        }
                    ) }
                    publish = @{ type = 'publish_targets'; value = @{
                        publish_summary = @{
                            from_artifact = 'summary'
                            location = @{ type = 'registered_location'; location_name = 'published_data'; path = 'reports/summary.csv' }
                            overwrite_policy = 'fail_if_exists'
                        }
                    } }
                }
            }}}
        )
    }
    source_manifest = @{ files = @(
        @{ role = 'python_entrypoint'; path = '.goetl-smoke/fake-hpcc-data-assets/scripts/fake_hpcc_data_assets.py'; content_type = 'text/x-python' }
    ) }
    variables = $workerVariables
}
Write-JsonFile -Path $workflowPath -Value $workflow

$controllerConfig = @{
    api_version = 'goet/v1alpha1'
    kind = 'Controller'
    variables = @(
        @{ name = @{ namespace = 'controller_config'; key = 'controller_url' }; type = 'string'; expression = $controllerURL },
        @{ name = @{ namespace = 'controller_config'; key = 'main_database_driver' }; type = 'string'; expression = 'sqlite' },
        @{ name = @{ namespace = 'controller_config'; key = 'main_database_connection_string' }; type = 'string'; expression = (Convert-ToForwardSlashPath (Join-Path $runRoot 'controller.sqlite')) }
    )
    execution_environment = @{
        name = 'fake-hpcc-data-assets-local-slurm'
        transports = @(@{ name = 'local'; type = 'local' })
        dialect = @{ type = 'bash' }
        scheduler = @{ type = 'slurm' }
        runtime = @{ type = 'worker'; settings = @{
            root = $runtimeRootFwd
            controller_url = $controllerURL
            data_dir = $workerDataRootFwd
            asset_cache_dir = $assetCacheRootFwd
            data_location_roots = @{
                fixture_data = $fixtureRootFwd
                published_data = $publishedRootFwd
            }
        }}
    }
}
Write-JsonFile -Path $controllerConfigPath -Value $controllerConfig

$submission = @{
    project = @{ repository = 'local:demo'; ref = 'main'; path = 'project.json' }
    workflow = @{ repository = 'local:demo'; ref = 'main'; path = '.goetl-smoke/fake-hpcc-data-assets/workflow.json' }
    variables = @()
}
Write-JsonFile -Path $submissionPath -Value $submission

$fakeSbatch = Join-Path $repoRoot 'scripts\fake-hpcc'
$sbatchCmd = Join-Path $binRoot 'sbatch.cmd'
Write-TextFile -Path $sbatchCmd -Value "@echo off`r`nbash `"$((Join-Path $fakeSbatch 'sbatch').Replace('\', '/'))`" %*`r`n"

$oldPath = $env:PATH
$oldRunRoot = $env:FAKE_SLURM_RUN_ROOT
$oldForeground = $env:FAKE_SLURM_FOREGROUND
$controller = $null
try {
    $env:PATH = "$binRoot;$fakeSbatch;$oldPath"
    $env:FAKE_SLURM_RUN_ROOT = Convert-ToForwardSlashPath $slurmRunRoot
    $env:FAKE_SLURM_FOREGROUND = '1'

    $controllerOut = Join-Path $runRoot 'controller.out.log'
    $controllerErr = Join-Path $runRoot 'controller.err.log'
    $controller = Start-Process -FilePath 'go' -ArgumentList @('run', './cmd/controller', '--config', (Convert-ToForwardSlashPath $controllerConfigPath)) -WorkingDirectory $repoRoot -RedirectStandardOutput $controllerOut -RedirectStandardError $controllerErr -WindowStyle Hidden -PassThru
    Wait-ForController -ControllerURL $controllerURL

    $ack = Invoke-RestMethod -Method Post -Uri "$controllerURL/workflow" -ContentType 'application/json' -InFile $submissionPath
    $submissionID = [string]$ack.submission_id
    if (-not $submissionID) {
        throw "controller acknowledgement did not include submission_id"
    }
    $status = Wait-ForSubmission -ControllerURL $controllerURL -SubmissionID $submissionID

    $manifestPath = Join-Path $workerDataRoot 'fake-hpcc-data-assets-smoke.json'
    if (-not (Test-Path -LiteralPath $manifestPath -PathType Leaf)) {
        throw "artifact manifest output missing: $manifestPath"
    }
    $manifest = Get-Content -Raw -LiteralPath $manifestPath | ConvertFrom-Json
    if (-not $manifest.artifacts -or $manifest.artifacts.Count -lt 1) {
        throw "artifact manifest missing artifacts: $manifestPath"
    }
    if (-not $manifest.published_assets -or $manifest.published_assets.Count -lt 1) {
        throw "artifact manifest missing published assets: $manifestPath"
    }

    $promotedArtifact = Join-Path $workerDataRoot ($manifest.artifacts[0].path -replace '/', [IO.Path]::DirectorySeparatorChar)
    if (-not (Test-Path -LiteralPath $promotedArtifact -PathType Leaf)) {
        throw "promoted artifact missing: $promotedArtifact"
    }
    $publishedArtifact = Join-Path $publishedRoot 'reports\summary.csv'
    if (-not (Test-Path -LiteralPath $publishedArtifact -PathType Leaf)) {
        throw "published artifact missing: $publishedArtifact"
    }

    [pscustomobject]@{
        submission_id = $submissionID
        status = $status.status
        manifest = $manifestPath
        promoted_artifact = $promotedArtifact
        published_artifact = $publishedArtifact
        controller_stdout = $controllerOut
        controller_stderr = $controllerErr
        worker_logs = (Join-Path $runtimeRoot 'logs')
        slurm_logs = $slurmRunRoot
    } | ConvertTo-Json -Depth 4 | Write-Host
} finally {
    try {
        Invoke-RestMethod -Method Post -Uri "$controllerURL/shutdown" | Out-Null
    } catch {
    }
    if ($controller -and -not $controller.HasExited) {
        $controller.Kill()
    }
    $env:PATH = $oldPath
    $env:FAKE_SLURM_RUN_ROOT = $oldRunRoot
    $env:FAKE_SLURM_FOREGROUND = $oldForeground
}
