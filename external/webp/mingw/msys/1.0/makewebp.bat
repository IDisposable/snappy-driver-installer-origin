rem NOTES:
rem need to update the following files when changing libwebp version
rem "trunk\external\webp\mingw\msys\1.0\home\libwebp-*.tar.gz"
rem "trunk\external\webp\mingw\msys\1.0\makewebp.bat"
rem "trunk\external\webp\mingw\msys\1.0\home\makewebp.bat"

rem %PREFIX% is the output path of the build
rem add each of these paths to code blocks search path for each project build config

echo Using %TOOLSET%

set LIBWEBP_VER=1.6.0
set HOME=%CD%\home

cls
@echo *** libwebp 32-bit build
@echo.

set MSYSTEM=MINGW32
set PREFIX=c:/mingw/mingw32/i686-w64-mingw32
set PATH=C:\mingw\mingw32\bin;%PATH%
@echo MSYSTEM: %MSYSTEM%
@echo PREFIX : %PREFIX%
@echo PATH   : %PATH%
pause

rem need a clean source path
cd C:\msys64\home
if exist libwebp-%LIBWEBP_VER%  rd libwebp-%LIBWEBP_VER% /s /q	
if exist libwebp-%LIBWEBP_VER%.tar del libwebp-%LIBWEBP_VER%.tar /q
"C:\Program Files\7-Zip\7z.exe" x libwebp-%LIBWEBP_VER%.tar.gz
"C:\Program Files\7-Zip\7z.exe" x libwebp-%LIBWEBP_VER%.tar
pause
%1\sh --login -i %~dp0home\makewebp.bat

cls
@echo *** libwebp 64-bit build
@echo.

set MSYSTEM=MINGW64
set PREFIX=c:/mingw/mingw64/x86_64-w64-mingw32
set PATH=C:\mingw\mingw64\bin;%PATH%
@echo MSYSTEM: %MSYSTEM%
@echo PREFIX : %PREFIX%
@echo PATH   : %PATH%
pause

rem need a clean source path
cd C:\msys64\home
if exist libwebp-%LIBWEBP_VER% rd libwebp-%LIBWEBP_VER% /s /q
if exist libwebp-%LIBWEBP_VER%.tar del libwebp-%LIBWEBP_VER%.tar /q
"C:\Program Files\7-Zip\7z.exe" x libwebp-%LIBWEBP_VER%.tar.gz
"C:\Program Files\7-Zip\7z.exe" x libwebp-%LIBWEBP_VER%.tar
pause

%1\sh --login -i %~dp0home\makewebp.bat
