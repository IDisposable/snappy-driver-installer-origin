@echo off
rem Builds one of this rewrite's commands with the native Windows Go
rem toolchain and runs it, for use directly from a Windows command
rem prompt (not WSL) - requires Go to be installed on Windows. See
rem run-windows.sh for the WSL/PE-interop equivalent used during
rem development, which cross-compiles instead.
rem
rem Usage: scripts\run-windows.cmd [target] [args...]
rem   target   one of: sdigo (default), sdi, hwdump, torrenttest
rem   args     forwarded to the built executable as-is
rem
rem hwdump and torrenttest are sdigo subcommands, not their own
rem binaries; this script still accepts them as a target name for
rem convenience, building cmd\sdigo and forwarding the subcommand as
rem the first argument.
rem
rem Examples:
rem   scripts\run-windows.cmd
rem   scripts\run-windows.cmd sdi -drp-dir=D:\drivers -index-dir=D:\indexes
rem   scripts\run-windows.cmd sdigo -torrent-file=D:\SDIO_Update.torrent
rem   scripts\run-windows.cmd hwdump

setlocal enabledelayedexpansion

cd /d "%~dp0.."

set "TARGET=sdigo"
set "FIRST=%~1"
if not "%FIRST%"=="" (
	set "FIRSTCHAR=%FIRST:~0,1%"
	if not "!FIRSTCHAR!"=="-" (
		set "TARGET=%FIRST%"
		shift
	)
)

set "BUILDTARGET=%TARGET%"
set "PREPENDARG="
if /i "%TARGET%"=="sdigo" goto validtarget
if /i "%TARGET%"=="sdi" goto validtarget
if /i "%TARGET%"=="hwdump" (
	set "BUILDTARGET=sdigo"
	set "PREPENDARG=hwdump"
	goto validtarget
)
if /i "%TARGET%"=="torrenttest" (
	set "BUILDTARGET=sdigo"
	set "PREPENDARG=torrenttest"
	goto validtarget
)
echo run-windows.cmd: unknown target "%TARGET%" (want sdigo, sdi, hwdump, or torrenttest) 1>&2
exit /b 1
:validtarget

set "ARGS="
if not "%PREPENDARG%"=="" set "ARGS=!ARGS! "%PREPENDARG%""
:collectargs
if "%~1"=="" goto donecollecting
set "ARGS=!ARGS! "%~1""
shift
goto collectargs
:donecollecting

set "BIN=%TEMP%\%BUILDTARGET%-run.exe"

echo Building cmd\%BUILDTARGET%... 1>&2
go build -o "%BIN%" ".\cmd\%BUILDTARGET%"
if errorlevel 1 exit /b 1

echo Running... 1>&2
"%BIN%" !ARGS!
set "EXITCODE=%ERRORLEVEL%"
del "%BIN%" >nul 2>&1
exit /b %EXITCODE%
