@echo off
REM ============================================
REM  Windows build script for mcp-chat
REM  Usage: build.bat [version]
REM
REM  Output: standalone Windows .exe binaries with embedded web + mcp_servers.json.
REM  cfg.yml is external (placed alongside the binary at runtime).
REM ============================================

setlocal enabledelayedexpansion

REM --- garble 路径 ---
set "GARBLE=%USERPROFILE%\go\bin\garble.exe"
if not exist "%GARBLE%" (
    echo ERROR: garble not found at %GARBLE%
    echo Install with: go install mvdan.cc/garble@latest
    exit /b 1
)

REM --- version: use arg or today's date (YYYYMMDD) ---
set "VERSION=%1"
if "%VERSION%"=="" (
    for /f %%I in ('powershell -NoProfile -Command "Get-Date -Format yyyyMMdd"') do set "VERSION=%%I"
)

set "APP_NAME=mcp-chat"
set "BUILD_DIR=build"

if not exist "%BUILD_DIR%" mkdir "%BUILD_DIR%"

echo ============================================
echo  Building %APP_NAME% v%VERSION%
echo  Go:
go version
echo ============================================
echo.

REM --- build_one <arch> ---
call :build_one amd64

echo ============================================
echo  Done. Output: %BUILD_DIR%\
echo ============================================
dir "%BUILD_DIR%"
exit /b 0

REM ============================================
REM  build_one
REM    %1 = GOARCH
REM ============================================
:build_one
set "ARCH=%~1"
set "OUT=%BUILD_DIR%\%APP_NAME%_%VERSION%_windows_%ARCH%.exe"

echo --- [windows/%ARCH%] ---
echo   CGO_ENABLED=0 GOOS=windows GOARCH=%ARCH% garble -literals -tiny build -ldflags="-s -w -X main.version=%VERSION%" -o %OUT%

set "CGO_ENABLED=0"
set "GOOS=windows"
set "GOARCH=%ARCH%"
"%GARBLE%" -literals -tiny build -ldflags="-s -w -X main.version=%VERSION%" -o "%OUT%" .

if errorlevel 1 (
    echo   [ERROR] Build failed for windows/%ARCH%
    exit /b 1
)

echo   -^> %OUT%
echo.
exit /b 0