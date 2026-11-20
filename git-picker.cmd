@echo off
setlocal

if "%~1"=="" (
    echo Usage: %~nx0 "folder"
    exit /b 1
)

pushd "%~1"
if errorlevel 1 (
    echo Could not open folder: "%~1"
    exit /b 1
)

"%~dp0git-picker.exe"
set "result=%errorlevel%"

popd
exit /b %result%
