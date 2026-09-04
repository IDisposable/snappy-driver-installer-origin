@echo off
rem Builds cmd/sdigo with the native Windows Go toolchain and runs it,
rem for use directly from a Windows command prompt (not WSL) -
rem requires Go to be installed on Windows. See run-windows.sh for the
rem WSL/PE-interop equivalent used during development, which
rem cross-compiles instead.
rem
rem Usage: scripts\run-windows.cmd [args...]
rem   args   forwarded to sdigo as-is
rem
rem Examples:
rem   scripts\run-windows.cmd
rem   scripts\run-windows.cmd -nogui -drp-dir=D:\drivers -index-dir=D:\indexes
rem   scripts\run-windows.cmd hwdump

setlocal enabledelayedexpansion

cd /d "%~dp0.."

set "ARGS="
:collectargs
if "%~1"=="" goto donecollecting
set "ARGS=!ARGS! "%~1""
shift
goto collectargs
:donecollecting

set "BIN=%TEMP%\sdigo-run.exe"

echo Building cmd\sdigo... 1>&2
go build -o "%BIN%" ".\cmd\sdigo"
if errorlevel 1 exit /b 1

echo Running... 1>&2
"%BIN%" !ARGS!
set "EXITCODE=%ERRORLEVEL%"
del "%BIN%" >nul 2>&1
exit /b %EXITCODE%
