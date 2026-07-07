[CmdletBinding()]
param(
    [switch]$FakeHPCC,
    [switch]$Help
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Write-Usage {
    @'
Usage:
  pwsh -NoProfile -File scripts/cdl-yanroy-fixture-smoke.ps1
  pwsh -NoProfile -File scripts/cdl-yanroy-fixture-smoke.ps1 -FakeHPCC

The default mode starts a local controller, submits the sibling demo-project
CDL/Yan/Roy fixture workflow, and starts one local worker manually.

-FakeHPCC uses the existing local fake Slurm/sbatch boundary to start the
worker through the configured execution environment. Neither mode downloads
real CDL, reads real Yan/Roy data, contacts Google Drive, or needs credentials.
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
    $json = $Value | ConvertTo-Json -Depth 60
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
    $deadline = (Get-Date).AddSeconds(45)
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
    $deadline = (Get-Date).AddSeconds(120)
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

function Assert-FileText {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$Expected
    )
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "expected file missing: $Path"
    }
    $actual = [System.IO.File]::ReadAllText($Path).Replace("`r`n", "`n")
    if ($actual -ne $Expected) {
        throw "unexpected file content: $Path`n$actual"
    }
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$demoRoot = Join-Path (Split-Path -Parent $repoRoot) 'go-etl-demo-project'
if (-not (Test-Path -LiteralPath (Join-Path $demoRoot 'workflows\cdl-yanroy-fixture.json') -PathType Leaf)) {
    throw "sibling CDL/Yan/Roy fixture workflow missing under $demoRoot"
}
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw 'go is required'
}
$pythonCommand = Get-Command python3 -ErrorAction SilentlyContinue
if (-not $pythonCommand) {
    $pythonCommand = Get-Command python -ErrorAction SilentlyContinue
}
if (-not $pythonCommand) {
    throw 'python3 or python is required'
}
if ($FakeHPCC -and -not (Get-Command bash -ErrorAction SilentlyContinue)) {
    throw 'bash is required so fake sbatch can execute generated Slurm scripts'
}

$modeName = if ($FakeHPCC) { 'fake-hpcc' } else { 'local' }
$runRoot = Join-Path $repoRoot ".run\cdl-yanroy-fixture-$modeName"
$controllerURL = 'http://localhost:8080'
$workerDataRoot = Join-Path $runRoot 'worker-data'
$workerTmpRoot = Join-Path $runRoot 'worker-tmp'
$workerLogRoot = Join-Path $runRoot 'worker-logs'
$assetCacheRoot = Join-Path $runRoot 'asset-cache'
$publishedRoot = Join-Path $runRoot 'published-data'
$fixtureRoot = Join-Path $demoRoot 'data\fixtures\cdl-yanroy'
$controllerConfigPath = Join-Path $runRoot 'controller.json'
$workerConfigPath = Join-Path $runRoot 'worker.json'
$submissionPath = Join-Path $runRoot 'submission.json'

if (Test-Path -LiteralPath $runRoot) {
    Remove-Item -LiteralPath $runRoot -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $runRoot, $workerDataRoot, $workerTmpRoot, $workerLogRoot, $assetCacheRoot, $publishedRoot | Out-Null
Copy-Item -LiteralPath (Join-Path $repoRoot 'cmd\controller\defaults.json') -Destination (Join-Path $runRoot 'defaults.json') -Force

$controllerConfig = @{
    api_version = 'goet/v1alpha1'
    kind = 'Controller'
    variables = @(
        @{ name = @{ namespace = 'controller_config'; key = 'controller_url' }; type = 'string'; expression = $controllerURL },
        @{ name = @{ namespace = 'controller_config'; key = 'main_database_driver' }; type = 'string'; expression = 'sqlite' },
        @{ name = @{ namespace = 'controller_config'; key = 'main_database_connection_string' }; type = 'string'; expression = (Convert-ToForwardSlashPath (Join-Path $runRoot 'controller.sqlite')) },
        @{ name = @{ namespace = 'controller_config'; key = 'controller_root_dir' }; type = 'path'; expression = (Convert-ToForwardSlashPath $runRoot) }
    )
}

if ($FakeHPCC) {
    $runtimeRoot = Join-Path $runRoot 'runtime'
    $slurmRunRoot = Join-Path $runRoot 'slurm'
    $binRoot = Join-Path $runRoot 'bin'
    New-Item -ItemType Directory -Force -Path $runtimeRoot, $slurmRunRoot, $binRoot | Out-Null
    $workerConfigPath = Join-Path $runtimeRoot 'config\worker.json'
    $workerLogRoot = Join-Path $runtimeRoot 'logs'
    $workerScriptPath = Join-Path $runtimeRoot 'scripts\worker.slurm'
    $controllerConfig.execution_environment = @{
        name = 'cdl-yanroy-fixture-local-slurm'
        transports = @(@{ name = 'local'; type = 'local' })
        dialect = @{ type = 'bash' }
        scheduler = @{ type = 'slurm' }
        runtime = @{ type = 'worker'; settings = @{
            root = (Convert-ToForwardSlashPath $runtimeRoot)
            controller_url = $controllerURL
            data_dir = (Convert-ToForwardSlashPath $workerDataRoot)
            asset_cache_dir = (Convert-ToForwardSlashPath $assetCacheRoot)
            data_location_roots = @{
                fixture_data = (Convert-ToForwardSlashPath $fixtureRoot)
                published_data = (Convert-ToForwardSlashPath $publishedRoot)
            }
            python_executable = $pythonCommand.Source
        }}
    }
}
Write-JsonFile -Path $controllerConfigPath -Value $controllerConfig

$submission = @{
    project = @{ repository = 'local:demo'; ref = 'main'; path = 'project.json' }
    workflow = @{ repository = 'local:demo'; ref = 'main'; path = 'workflows/cdl-yanroy-fixture.json' }
    variables = @()
}
if ($FakeHPCC) {
    $submission.variables = @(
        @{ name = @{ namespace = 'worker_config'; key = 'scheduler' }; type = 'object'; expression = @{
            type = @{ type = 'string'; expression = 'slurm' }
            settings = @{ type = 'object'; expression = @{
                script_path = @{ type = 'path'; expression = (Convert-ToForwardSlashPath $workerScriptPath) }
                job_name = @{ type = 'string'; expression = 'goetl-cdl-yanroy-fixture' }
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
                config_path = @{ type = 'path'; expression = (Convert-ToForwardSlashPath $workerConfigPath) }
                log_dir = @{ type = 'path'; expression = (Convert-ToForwardSlashPath $workerLogRoot) }
            }}
        }},
        @{ name = @{ namespace = 'worker_config'; key = 'worker_min_count' }; type = 'int'; expression = 1 },
        @{ name = @{ namespace = 'worker_config'; key = 'worker_max_count' }; type = 'int'; expression = 1 },
        @{ name = @{ namespace = 'worker_config'; key = 'worker_count_per_start' }; type = 'int'; expression = 1 },
        @{ name = @{ namespace = 'worker_config'; key = 'worker_min_elapsed_time_between_starts' }; type = 'string'; expression = '0s' }
    )
}
Write-JsonFile -Path $submissionPath -Value $submission

if (-not $FakeHPCC) {
    $workerConfig = @{
        log_dir = (Convert-ToForwardSlashPath $workerLogRoot)
        tmp_dir = (Convert-ToForwardSlashPath $workerTmpRoot)
        data_dir = (Convert-ToForwardSlashPath $workerDataRoot)
        controller_url = $controllerURL
        python_executable = $pythonCommand.Source
        asset_cache_dir = (Convert-ToForwardSlashPath $assetCacheRoot)
        data_location_roots = @{
            fixture_data = (Convert-ToForwardSlashPath $fixtureRoot)
            published_data = (Convert-ToForwardSlashPath $publishedRoot)
        }
    }
    Write-JsonFile -Path $workerConfigPath -Value $workerConfig
}

$oldPath = $env:PATH
$oldRunRoot = $env:FAKE_SLURM_RUN_ROOT
$oldSlurmWorkdir = $env:FAKE_SLURM_WORKDIR
$oldForeground = $env:FAKE_SLURM_FOREGROUND
$controller = $null
try {
    if ($FakeHPCC) {
        $fakeSbatchScript = Join-Path $binRoot 'fake-sbatch.ps1'
        Write-TextFile -Path $fakeSbatchScript -Value @'
param([Parameter(Mandatory = $true)][string]$ScriptPath)
$ErrorActionPreference = 'Stop'
$runRoot = $env:FAKE_SLURM_RUN_ROOT
if (-not $runRoot) { $runRoot = '.run/fake-slurm' }
New-Item -ItemType Directory -Force -Path $runRoot | Out-Null
$counterFile = Join-Path $runRoot 'job-counter'
if (Test-Path -LiteralPath $counterFile -PathType Leaf) {
    $jobID = [int]([System.IO.File]::ReadAllText($counterFile).Trim()) + 1
} else {
    $jobID = 1000
}
[System.IO.File]::WriteAllText($counterFile, "$jobID`n", [System.Text.UTF8Encoding]::new($false))
$stdoutPath = Join-Path $runRoot "job-$jobID.out"
$stderrPath = Join-Path $runRoot "job-$jobID.err"
$submissionLog = Join-Path $runRoot 'submissions.log'
Add-Content -LiteralPath $submissionLog -Value "job_id=$jobID"
Add-Content -LiteralPath $submissionLog -Value "script=$ScriptPath"
Add-Content -LiteralPath $submissionLog -Value "stdout=$stdoutPath"
Add-Content -LiteralPath $submissionLog -Value "stderr=$stderrPath"
$scriptText = [System.IO.File]::ReadAllText($ScriptPath)
$match = [regex]::Match($scriptText, "'go'\s+'run'\s+'\./cmd/worker'\s+'([^']+)'")
if (-not $match.Success) {
    Write-Error "could not find generated worker command in $ScriptPath"
    exit 2
}
$workerConfig = $match.Groups[1].Value
Write-Output "Submitted batch job $jobID"
Push-Location $env:FAKE_SLURM_WORKDIR
try {
    & go run ./cmd/worker "$workerConfig" > $stdoutPath 2> $stderrPath
} finally {
    Pop-Location
}
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}
'@
        $sbatchCmd = Join-Path $binRoot 'sbatch.cmd'
        Write-TextFile -Path $sbatchCmd -Value "@echo off`r`npowershell -NoProfile -ExecutionPolicy Bypass -File `"$fakeSbatchScript`" `"%~1`"`r`n"
        if ($pythonCommand.Name -ne 'python3.exe' -and $pythonCommand.Name -ne 'python3') {
            $pythonShim = Join-Path $binRoot 'python3.cmd'
            Write-TextFile -Path $pythonShim -Value "@echo off`r`n`"$($pythonCommand.Source)`" %*`r`n"
        }
        $env:PATH = "$binRoot;$oldPath"
        $env:FAKE_SLURM_RUN_ROOT = Convert-ToForwardSlashPath $slurmRunRoot
        $env:FAKE_SLURM_WORKDIR = $repoRoot
        $env:FAKE_SLURM_FOREGROUND = '1'
    }

    $controllerOut = Join-Path $runRoot 'controller.out.log'
    $controllerErr = Join-Path $runRoot 'controller.err.log'
    $controllerConfigArg = '"' + (Convert-ToForwardSlashPath $controllerConfigPath) + '"'
    $controller = Start-Process -FilePath 'go' -ArgumentList @('run', './cmd/controller', '--config', $controllerConfigArg) -WorkingDirectory $repoRoot -RedirectStandardOutput $controllerOut -RedirectStandardError $controllerErr -WindowStyle Hidden -PassThru
    Wait-ForController -ControllerURL $controllerURL

    $ack = Invoke-RestMethod -Method Post -Uri "$controllerURL/workflow" -ContentType 'application/json' -InFile $submissionPath
    $submissionID = [string]$ack.submission_id
    if (-not $submissionID) {
        throw 'controller acknowledgement did not include submission_id'
    }

    if (-not $FakeHPCC) {
        $workerOut = Join-Path $runRoot 'worker.out.log'
        $workerErr = Join-Path $runRoot 'worker.err.log'
        $workerConfigArg = '"' + (Convert-ToForwardSlashPath $workerConfigPath) + '"'
        $worker = Start-Process -FilePath 'go' -ArgumentList @('run', './cmd/worker', $workerConfigArg) -WorkingDirectory $repoRoot -RedirectStandardOutput $workerOut -RedirectStandardError $workerErr -WindowStyle Hidden -PassThru
        if (-not $worker.WaitForExit(90000)) {
            $worker.Kill()
            throw 'local worker timed out'
        }
        $worker.Refresh()
        if ($null -ne $worker.ExitCode -and $worker.ExitCode -ne 0) {
            throw "local worker exited with code $($worker.ExitCode)"
        }
    }

    $status = Wait-ForSubmission -ControllerURL $controllerURL -SubmissionID $submissionID
    $manifestPath = Join-Path $workerDataRoot 'cdl-yanroy-fixture-fixture_tile_001.json'
    if (-not (Test-Path -LiteralPath $manifestPath -PathType Leaf)) {
        throw "artifact manifest output missing: $manifestPath"
    }
    $manifest = Get-Content -Raw -LiteralPath $manifestPath | ConvertFrom-Json
    if ($manifest.artifacts.Count -ne 2) {
        throw "artifact manifest should contain 2 artifacts: $manifestPath"
    }
    if ($manifest.published_assets.Count -ne 2) {
        throw "artifact manifest should contain 2 published assets: $manifestPath"
    }

    $compositionPath = Join-Path $publishedRoot 'field_cdl_composition\year=2023\tile=fixture_tile_001\field_cdl_composition.csv'
    $dominantPath = Join-Path $publishedRoot 'field_dominant_crop\year=2023\tile=fixture_tile_001\field_dominant_crop.csv'
    $expectedComposition = "field_id,field_tile_id,year,crop_code,crop_type,field_pixel_count,crop_pixel_count,crop_fraction,is_dominant_crop,dominant_crop_code,dominant_crop_type,dominant_crop_fraction,assignment_policy`n1,fixture_tile_001,2023,5,corn,5,4,0.8,true,5,corn,0.8,dominant_share_v1`n1,fixture_tile_001,2023,1,soybeans,5,1,0.2,false,5,corn,0.8,dominant_share_v1`n2,fixture_tile_001,2023,1,soybeans,5,4,0.8,true,1,soybeans,0.8,dominant_share_v1`n2,fixture_tile_001,2023,2,wheat,5,1,0.2,false,1,soybeans,0.8,dominant_share_v1`n3,fixture_tile_001,2023,2,wheat,5,4,0.8,true,2,wheat,0.8,dominant_share_v1`n3,fixture_tile_001,2023,5,corn,5,1,0.2,false,2,wheat,0.8,dominant_share_v1`n"
    $expectedDominant = "field_id,field_tile_id,year,dominant_crop_code,dominant_crop_type,dominant_crop_fraction,field_pixel_count,assignment_status,assignment_policy`n1,fixture_tile_001,2023,5,corn,0.8,5,assigned,dominant_share_v1`n2,fixture_tile_001,2023,1,soybeans,0.8,5,assigned,dominant_share_v1`n3,fixture_tile_001,2023,2,wheat,0.8,5,assigned,dominant_share_v1`n"
    Assert-FileText -Path $compositionPath -Expected $expectedComposition
    Assert-FileText -Path $dominantPath -Expected $expectedDominant

    [pscustomobject]@{
        mode = $modeName
        submission_id = $submissionID
        status = $status.status
        manifest = $manifestPath
        composition = $compositionPath
        dominant = $dominantPath
        controller_stdout = $controllerOut
        controller_stderr = $controllerErr
        worker_logs = $workerLogRoot
    } | ConvertTo-Json -Depth 4 | Write-Host
} finally {
    try {
        Invoke-RestMethod -Method Post -Uri "$controllerURL/shutdown" | Out-Null
    } catch {
    }
    if ($controller -and -not $controller.HasExited) {
        try {
            $controller.Kill()
        } catch {
        }
    }
    $env:PATH = $oldPath
    $env:FAKE_SLURM_RUN_ROOT = $oldRunRoot
    $env:FAKE_SLURM_WORKDIR = $oldSlurmWorkdir
    $env:FAKE_SLURM_FOREGROUND = $oldForeground
}
