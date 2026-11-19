# Build the standalone git-picker binary.
#
#   .\build.ps1
#
# The result is git-picker.exe in this folder: a single file with no runtime
# dependencies, which is what makes it start in tens of milliseconds.

$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot

# -s -w drop the symbol table and the debug info. The program gets no
# smaller in any way that matters, but the file does.
go build -ldflags "-s -w" -o git-picker.exe .

Write-Host ("Built {0} ({1:N1} KB)" -f (Join-Path $PSScriptRoot 'git-picker.exe'), ((Get-Item (Join-Path $PSScriptRoot 'git-picker.exe')).Length / 1KB))
