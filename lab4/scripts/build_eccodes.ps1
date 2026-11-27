# Buduje libaec i ecCodes z CMake do lab4\.eccodes (Windows; wymaga Visual Studio Build Tools + cmake)
$ErrorActionPreference = "Stop"
$EccodesVersion = if ($env:ECCODES_VERSION) { $env:ECCODES_VERSION } else { "2.50.1" }
$LibaecVersion = if ($env:LIBAEC_VERSION) { $env:LIBAEC_VERSION } else { "1.1.4" }

$Root = Split-Path -Parent $PSScriptRoot
$Prefix = Join-Path $Root ".eccodes"
$Work = Join-Path $Root ".build"
New-Item -ItemType Directory -Force $Work, $Prefix | Out-Null
Set-Location $Work

function Get-Source($Url, $Archive, $Dir) {
    if (-not (Test-Path $Archive)) { Invoke-WebRequest $Url -OutFile $Archive }
    if (Test-Path $Dir) { Remove-Item -Recurse -Force $Dir }
    New-Item -ItemType Directory $Dir | Out-Null
    tar -xzf $Archive -C $Dir --strip-components=1
}

# --- libaec ---
Get-Source "https://github.com/MathisRosenhauer/libaec/archive/refs/tags/v$LibaecVersion.tar.gz" "libaec.tar.gz" "libaec-src"
cmake -S libaec-src -B libaec-build -DCMAKE_INSTALL_PREFIX="$Prefix" -DBUILD_TESTING=OFF
cmake --build libaec-build --config Release
cmake --install libaec-build --config Release

# --- ecCodes ---
Get-Source "https://github.com/ecmwf/eccodes/archive/refs/tags/$EccodesVersion.tar.gz" "eccodes.tar.gz" "eccodes-src"
cmake -S eccodes-src -B eccodes-build `
    -DCMAKE_INSTALL_PREFIX="$Prefix" -DCMAKE_PREFIX_PATH="$Prefix" `
    -DENABLE_FORTRAN=OFF -DENABLE_NETCDF=OFF -DENABLE_PYTHON=OFF `
    -DENABLE_MEMFS=OFF -DENABLE_EXAMPLES=OFF -DENABLE_TESTS=OFF -DENABLE_AEC=ON
cmake --build eccodes-build --config Release
cmake --install eccodes-build --config Release

Write-Host "OK: ecCodes zainstalowany w $Prefix"
