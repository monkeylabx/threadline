param(
    [Parameter(Mandatory = $true)]
    [string]$ArtifactRoot,

    [ValidateRange(3, 10)]
    [int]$Repetitions = 3
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$stackOverflowExit = -1073741571
$artifactRoot = [System.IO.Path]::GetFullPath($ArtifactRoot)
$binDirectory = Join-Path $artifactRoot "bin"
$buildLog = Join-Path $artifactRoot "cargo-build.txt"
$runLog = Join-Path $artifactRoot "runs.txt"

New-Item -ItemType Directory -Path $binDirectory -Force | Out-Null

& cargo test -p threadline-client-core --test sqlcipher_backend --locked --no-run 2>&1 |
    Tee-Object -FilePath $buildLog
if ($LASTEXITCODE -ne 0) {
    throw "cargo test --no-run failed with exit code $LASTEXITCODE"
}

$targetDirectory = if ($env:CARGO_TARGET_DIR) {
    [System.IO.Path]::GetFullPath($env:CARGO_TARGET_DIR)
} else {
    Join-Path (Get-Location) "target"
}
$dependencyDirectory = Join-Path $targetDirectory "debug\deps"
$testExecutables = @(
    Get-ChildItem -Path $dependencyDirectory -Filter "sqlcipher_backend-*.exe" -File |
        Sort-Object -Property LastWriteTimeUtc -Descending
)
if ($testExecutables.Count -ne 1) {
    throw "Expected exactly one sqlcipher_backend test executable; found $($testExecutables.Count)"
}

$sourceExecutable = $testExecutables[0]
$capturedExecutable = Join-Path $binDirectory $sourceExecutable.Name
Copy-Item -LiteralPath $sourceExecutable.FullName -Destination $capturedExecutable

$sourcePdb = [System.IO.Path]::ChangeExtension($sourceExecutable.FullName, ".pdb")
if (Test-Path -LiteralPath $sourcePdb) {
    Copy-Item -LiteralPath $sourcePdb -Destination $binDirectory
}

@(
    Join-Path $targetDirectory "debug"
    $dependencyDirectory
) | ForEach-Object {
    if (Test-Path -LiteralPath $_) {
        Get-ChildItem -Path $_ -Filter "*.dll" -File | ForEach-Object {
            Copy-Item -LiteralPath $_.FullName -Destination $binDirectory -Force
        }
    }
}

@'
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$executables = @(Get-ChildItem -Path "$PSScriptRoot\bin" -Filter "sqlcipher_backend-*.exe" -File)
if ($executables.Count -ne 1) {
    throw "Expected exactly one sqlcipher_backend test executable; found $($executables.Count)"
}
$env:PATH = "$PSScriptRoot\bin;$env:PATH"
& $executables[0].FullName --nocapture
if ($LASTEXITCODE -eq -1073741571) { exit 1 }
if ($LASTEXITCODE -ne 0) { exit 2 }
exit 0
'@ | Set-Content -LiteralPath (Join-Path $artifactRoot "reproduce.ps1") -Encoding utf8

$env:PATH = "$binDirectory;$env:PATH"
$testNames = @(
    & $capturedExecutable --color never --list |
        ForEach-Object {
            if ($_ -match '^(.+): test$') {
                $Matches[1]
            }
        }
)
if ($testNames.Count -eq 0) {
    throw "The SQLCipher test executable did not list any tests"
}

$results = @()
foreach ($testName in $testNames) {
    for ($attempt = 1; $attempt -le $Repetitions; $attempt++) {
        "=== $testName run $attempt of $Repetitions ===" | Tee-Object -FilePath $runLog -Append
        $output = & $capturedExecutable $testName --exact --nocapture --test-threads=1 2>&1
        $exitCode = $LASTEXITCODE
        $output | Out-File -LiteralPath $runLog -Encoding utf8 -Append
        "exit_code=$exitCode" | Tee-Object -FilePath $runLog -Append
        $classification = if ($exitCode -eq $stackOverflowExit) {
            "STATUS_STACK_OVERFLOW"
        } elseif ($exitCode -eq 0) {
            "PASS"
        } else {
            "OTHER_FAILURE"
        }
        $results += [pscustomobject]@{
            test = $testName
            attempt = $attempt
            exit_code = $exitCode
            classification = $classification
        }
    }
}

$hash = (Get-FileHash -LiteralPath $capturedExecutable -Algorithm SHA256).Hash.ToLowerInvariant()
$manifest = [pscustomobject]@{
    source_revision = $env:GITHUB_SHA
    runner_image = $env:ImageVersion
    runner_os = $env:ImageOS
    rustc = ((& rustc --version --verbose) -join "`n")
    cargo = (& cargo --version)
    command = "cargo test -p threadline-client-core --test sqlcipher_backend --locked --no-run"
    executable = "bin/$($sourceExecutable.Name)"
    sha256 = $hash
    repetitions_per_test = $Repetitions
    expected_exit_code = $stackOverflowExit
    results = $results
}
$manifest | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $artifactRoot "manifest.json") -Encoding utf8

$resultSummary = ($results | ForEach-Object { "$($_.test)=$($_.classification)" }) -join ", "
$summary = @(
    "## Windows SQLCipher diagnostic"
    ""
    "- Executable: ``$($sourceExecutable.Name)``"
    "- SHA-256: ``$hash``"
    "- Tests: ``$(($testNames -join ', '))``"
    "- Results: ``$resultSummary``"
) -join [Environment]::NewLine
$summary | Out-File -LiteralPath $env:GITHUB_STEP_SUMMARY -Encoding utf8 -Append

$stackOverflowTests = @(
    $results |
        Where-Object { $_.classification -eq "STATUS_STACK_OVERFLOW" } |
        Select-Object -ExpandProperty test -Unique
)
$otherFailures = @($results | Where-Object { $_.classification -eq "OTHER_FAILURE" })
if ($stackOverflowTests.Count -gt 0) {
    Write-Error "Reproduced STATUS_STACK_OVERFLOW in: $($stackOverflowTests -join ', ')"
    exit 1
}
if ($otherFailures.Count -eq 0) {
    Write-Host "Every test passed $Repetitions times from the same executable."
    exit 0
}
Write-Error "The result was not a deterministic pass or stack overflow: $(($results | ConvertTo-Json -Compress))"
exit 2
