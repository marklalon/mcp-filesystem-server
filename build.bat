@echo off
setlocal

cd /d "%~dp0"

set "GOEXE=go"
where go >nul 2>&1
if errorlevel 1 (
    if exist "%ProgramFiles%\Go\bin\go.exe" (
        set "GOEXE=%ProgramFiles%\Go\bin\go.exe"
    ) else (
        echo [ERROR] Go was not found.
        echo Please install Go and make sure it is available in PATH.
        exit /b 1
    )
)

set "GOOS=windows"
set "GOARCH=amd64"
set "CGO_ENABLED=0"
set "OUTPUT=bin\x64\mcp-filesystem-server.exe"

if not exist "bin\x64" mkdir "bin\x64" >nul

echo Go version:
"%GOEXE%" version
echo.
echo Building %OUTPUT% ...

"%GOEXE%" mod download
if errorlevel 1 goto :failed

"%GOEXE%" build -trimpath -ldflags="-s -w" -o "%OUTPUT%" .
if errorlevel 1 goto :failed

echo.
echo Build succeeded:
echo %CD%\%OUTPUT%
exit /b 0

:failed
echo.
echo Build failed.
exit /b 1
