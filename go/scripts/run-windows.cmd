@echo off
rem Builds one of this rewrite's commands with the native Windows Go
rem toolchain and runs it, for use directly from a Windows command
rem prompt (not WSL) - requires Go to be installed on Windows. See
rem run-windows.sh for the WSL/PE-interop equivalent used during
rem development, which cross-compiles instead.
rem
rem Usage: scripts\run-windows.cmd [target] [args...]
rem   target   one of: sdi (default), sditui, hwdump, torrenttest
rem   args     forwarded to the built executable as-is
rem
rem Examples:
rem   scripts\run-windows.cmd
rem   scripts\run-windows.cmd sdi -drp-dir=D:\drivers -index-dir=D:\indexes
rem   scripts\run-windows.cmd sditui -torrent-file=D:\SDIO_Update.torrent

setlocal enabledelayedexpansion

cd /d "%~dp0.."

set "TARGET=sdi"
set "FIRST=%~1"
if not "%FIRST%"=="" (
	set "FIRSTCHAR=%FIRST:~0,1%"
	if not "!FIRSTCHAR!"=="-" (
		set "TARGET=%FIRST%"
		shift
	)
)

if /i "%TARGET%"=="sdi" goto validtarget
if /i "%TARGET%"=="sditui" goto validtarget
if /i "%TARGET%"=="hwdump" goto validtarget
if /i "%TARGET%"=="torrenttest" goto validtarget
echo run-windows.cmd: unknown target "%TARGET%" (want sdi, sditui, hwdump, or torrenttest) 1>&2
exit /b 1
:validtarget

set "ARGS="
:collectargs
if "%~1"=="" goto donecollecting
set "ARGS=!ARGS! "%~1""
shift
goto collectargs
:donecollecting

set "BIN=%TEMP%\%TARGET%-run.exe"

echo Building cmd\%TARGET%... 1>&2
go build -o "%BIN%" ".\cmd\%TARGET%"
if errorlevel 1 exit /b 1

echo Running... 1>&2
"%BIN%" !ARGS!
set "EXITCODE=%ERRORLEVEL%"
del "%BIN%" >nul 2>&1
exit /b %EXITCODE%
