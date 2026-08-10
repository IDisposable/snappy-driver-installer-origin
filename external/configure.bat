@echo off
TITLE Snappy Driver Installer Origin - Toolset Installer

rem this sets up the color text stuff
for /F "tokens=1,2 delims=#" %%a in ('"prompt #$H#$E# & echo on & for %%b in (1) do rem"') do (set "DEL=%%a")

rem Colors
set c_menu=03
set c_normal=07
set c_done=0A
set c_fail=0C
set c_do=0D
set c_skip=02



rem MSYS2
rem used by libwebp build process
rem https://www.msys2.org/
rem msys2-x86_64-20200903.exe
rem msys2-x86_64-20260611.exe


rem *** mingw / gcc
rem must use gcc with posix thread model for libtorrent to compile
rem install mingw/gcc
rem preferred
rem 7.3.0 64 posix https://sourceforge.net/projects/mingw-w64/files/Toolchains%20targetting%20Win64/Personal%20Builds/mingw-builds/7.3.0/threads-posix/sjlj/
rem 7.3.0 32 posix https://sourceforge.net/projects/mingw-w64/files/Toolchains%20targetting%20Win32/Personal%20Builds/mingw-builds/7.3.0/threads-posix/sjlj/
rem 8.1.0 64 posix https://sourceforge.net/projects/mingw-w64/files/Toolchains%20targetting%20Win64/Personal%20Builds/mingw-builds/8.1.0/threads-posix/sjlj/
rem 8.1.0 32 posix https://sourceforge.net/projects/mingw-w64/files/Toolchains%20targetting%20Win32/Personal%20Builds/mingw-builds/8.1.0/threads-posix/sjlj/
rem open each archive in 7z and extract contents to c:\mingw
rem result will be:
rem 64-bit -> c:\mingw\mingw64
rem 32-bit -> c:\mingw\mingw32
rem ensure boost, libtorrent, libwebp are rebuilt

rem pragma warning thread_local is broken
rem alledgedly fixed in gcc 16
rem https://sourceforge.net/p/mingw-w64/bugs/527/



rem BOOST
rem https://sourceforge.net/projects/boost/files/boost/
rem download, run install procedure below

rem b2 --help for full list of build options (see b2-help.txt)
rem -q quit on first error
rem --prefix=%BOOST_INSTALL_PATH%
rem --build-type=minimal
rem --layout=system (not with buildtype=complete)
rem toolset=gcc
rem variant=debug|release
rem link=static
rem threading=multi

rem Notes for Windows users

rem Boost.WinAPI has been updated to target Windows 7 by default, where possible. 
rem In previous releases Windows Vista was the default.

rem Support for Windows versions older than Windows 10 is deprecated and will be removed in Boost 1.87.

rem Boost.WinAPI is used internally as the Windows SDK abstraction layer in a 
rem number of Boost libraries, including Boost.Beast, Boost.Chrono, Boost.DateTime, 
rem Boost.Dll, Boost.Log, Boost.Process, Boost.Stacktrace, Boost.System, 
rem Boost.Thread and Boost.UUID. To select the target Windows version define 
rem BOOST_USE_WINAPI_VERSION to the numeric version similar to 
rem _WIN32_WINNT while compiling Boost and user's code. 
rem For example:
rem define=BOOST_USE_WINAPI_VERSION=0x0501 define=BOOST_USE_NTDDI_VERSION=0x0501
rem this enables boost to run on XP but it hides the boost stack trace stuff
rem which shows up as an error during boost build

rem If boost::filesystem simply will not compile with 0x0501, you can build the Boost libraries 
rem themselves under the default target setting (which safely uses internal fallback checks), and 
rem only define BOOST_USE_WINAPI_VERSION=0x0501 inside your application project files.
rem this works except libtorrent overrides it
rem the only way to get a build that runs on XP is to build without libtorrent
rem the workspace has 2 projects targetted to XP that eliminate libtorrent from the build
rem using #ifdef USE_TORRENT
rem default is 0x601 = win7


rem EXTRA_OPTIONS
rem Enforce using -std=c++11 when building boost
rem Enforce using -std=c++11 when building libtorrent (e.g. ensure both uses exactly the same options, period) through 
rem and when calling b2/bjam
rem ensure boost is built with these things
rem ensure this setting is on for code blocks project build options target compiler flags

rem libtorrent
rem https://github.com/arvidn/libtorrent/releases
rem libtorrent 64-bit bjam options   -mbig-obj    because it seems the object files get very big
rem -DTORRENT_ABI_VERSION=1
rem https://www.rasterbar.com/products/libtorrent/building.html

rem libtorrent v1.2.20 doesn't compile with boost v1.80.0
rem https://www.google.com/search?q=libtorrent-rasterbar+v1.2.20+doesn%27t+compile+with+boost+v1.80.0&sca_esv=b85cb7cd991b1669&biw=1149&bih=797&ei=2_tJaqu6CamP2roP1-XO6A0&ved=0ahUKEwjrk5O69rqVAxWph1YBHdeyE904FBDh1QMIEg&uact=5&oq=libtorrent-rasterbar+v1.2.20+doesn%27t+compile+with+boost+v1.80.0&gs_lp=Egxnd3Mtd2l6LXNlcnAiP2xpYnRvcnJlbnQtcmFzdGVyYmFyIHYxLjIuMjAgZG9lc24ndCBjb21waWxlIHdpdGggYm9vc3QgdjEuODAuMEjlGFAAWJkVcAB4AJABAJgBjgOgAa0LqgEHMC43LjAuMbgBA8gBAPgBAZgCB6ACuQrCAggQIRigARjDBMICBRAAGO8FwgIIEAAYgAQYogSYAwCSBwcwLjYuNC0xoAfhGbIHBzAuNi40LTG4B7kKwgcFMC41LjLIBxGACAE&sclient=gws-wiz-serp

rem 7zip
rem https://sourceforge.net/projects/sevenzip/files/7-Zip/
rem C\7zVersion.h
rem extract the C and CPP folders from the archive
rem add to Drivers project linker: uuid, ole32, oleaut32
rem try to build the 7-ZIP project first
rem errors about undefined reference means a file needs to be added to the 7-ZIP project source
rem the 7-Zip source code is set up to build a stand-alone executable with a main() function
rem but we need to build it into our executable. so...
rem CPP\7zip\UI\Console\MainAr.cpp   &  Main.cpp
rem all references to Main2 should be replaced with
rem 1.
rem int Main2(
rem   const WCHAR *command_line
rem );
rem 2.
rem MainAr.cpp has a main() function which is overriding the real one
rem this should be renamed to: int Extract7z(const WCHAR *str)
rem 3.
rem the entire contents of the try {} block near the top of Extract7z() (old main()) should be replaced with
rem res = Main2(str);
rem about a dozen lines -ish
rem 4.
rem need to add all the register procedures called by common.cpp
rem 7zRegister.cpp, Bcj2Register.cpp, BcjRegister.cpp, BranchRegister,cpp, ByteSwap.cpp, CopyRegister.cpp
rem Lzma2Register.cpp, LzmaRegister.cpp, PpmdRegister.cpp
rem 5.
rem there's a line in Main.cpp line 866 that retrieves the passed in command line instead of the
rem application command line
rem   NCommandLineParser::SplitCommandLine(command_line?command_line:GetCommandLineW(), commandStrings);
rem 6.
rem "CPP\7zip\UI\Console\ExtractCallbackConsole.cpp"
rem do a compare and look for lines relating to "_7z_total" & "_7z_setcomplited"

rem code blocks
rem https://sourceforge.net/projects/codeblocks/files/Binaries/
rem make sure to install the version *without* the built-in Compiler
rem eg codeblocks-25.03-setup.exe




rem *** Snappy Driver Installer Origin v2 ***
rem *** tool versions
rem Code Blocks v25.03			31-03-2025
rem msys2 v20220503				03-05-2022
rem gcc v8.1.0					24-05-2018
rem boost v1.79.0				09-04-2022
rem libwebp v1.6.0				09-07-2025
rem libtorrent v1.2.20			28-01-2025
rem 7-Zip v26.01				29-04-2026





rem Toolset
set TOOLSET=gcc

rem BOOST
set BOOST_VER=1_79_0
set BOOST_VER2=1.79.0
set BOOST_VER3=1_79

rem LIBTORRENT
set LIBTORRENT_VER2=1.2.20
set LIBTORRENT_VER=1_2_20


rem LIBWEBP
rem http://downloads.webmproject.org/releases/webp/index.html
set LIBWEBP_VER=1.6.0
rem "D:\Development\Snappy Driver Installer Origin\trunk\external\webp\mingw\msys\1.0\home\libwebp-*.tar.gz"
rem "D:\Development\Snappy Driver Installer Origin\trunk\external\webp\mingw\msys\1.0\makewebp.bat"
rem "D:\Development\Snappy Driver Installer Origin\trunk\external\webp\mingw\msys\1.0\home\makewebp.bat"

rem MING/GCC
rem requires posix variant of mingw
set GCC_VERSION=8.1.0
set GCC_VERSION2=81

set MINGW_PATH=C:\mingw

rem GCC 32-bit
set GCC_PATH=%MINGW_PATH%\mingw32
set GCC_PREFIX1=/i686-w64-mingw32
set GCC_PREFIX=\i686-w64-mingw32

rem GCC 64-bit
set GCC64_PATH=%MINGW_PATH%\mingw64
set GCC64_PREFIX1=/x86_64-w64-mingw32
set GCC64_PREFIX=\x86_64-w64-mingw32

rem GCC (common)
rem -w inhibit all warning messages
if %TOOLSET%==gcc set TOOLSET2=gcc
set EXTRA_OPTIONS="cxxflags=-std=c++11 -fexpensive-optimizations -fomit-frame-pointer -D IPV6_TCLASS=30 -w"
set LIBBOOST32="%GCC_PATH%%GCC_PREFIX%\lib\libboost_system_tr.a"
set LIBBOOST64="%GCC64_PATH%%GCC64_PREFIX%\lib\libboost_system_tr.a"
set LIBWEBP="%GCC_PATH%%GCC_PREFIX%\lib\libwebp.a"
set LIBTORREN32="%GCC_PATH%%GCC_PREFIX%\lib\libtorrent.a"
set LIBTORREN64="%GCC64_PATH%%GCC64_PREFIX%\lib\libtorrent.a"
rem code blocks links to these paths
set LIBTORRENT_INSTALL_PATH=C:\LIBTORRENT32
set LIBTORRENT64_INSTALL_PATH=C:\LIBTORRENT64
set LIBTORRENT_INSTALL_PATH_DEBUG=C:\LIBTORRENT32\DEBUG
set LIBTORRENT64_INSTALL_PATH_DEBUG=C:\LIBTORRENT64\DEBUG

rem MSYS
set MSYS_PATH=C:\msys64
set MSYS_BIN=%MSYS_PATH%\usr\bin
set ADR64=\adrs-mdl-64

rem BOOST
set BOOST_ROOT=%CD%\boost_%BOOST_VER%
set BOOST_BUILD_PATH=%BOOST_ROOT%
set BOOST_INSTALL_PATH=C:\BOOST32_%GCC_VERSION2%
set BOOST64_INSTALL_PATH=C:\BOOST64_%GCC_VERSION2%
set BOOST_INSTALL_PATH_DEBUG=C:\BOOST32_%GCC_VERSION2%\DEBUG
set BOOST64_INSTALL_PATH_DEBUG=C:\BOOST64_%GCC_VERSION2%\DEBUG

rem Configure paths
set LIBTORRENT_PATH=%CD%\libtorrent-libtorrent-%LIBTORRENT_VER%
set WEBP_PATH=%CD%\webp
set path=%BOOST_ROOT%;%MSYS_BIN%;%path%
set path=%GCC_PATH%\bin;%GCC64_PATH%\bin;%path%






:mainmenu
cls
echo.
color %c_menu%
call :ColorText 0F "  SDIO Tools Installation"&echo.
echo   ÉÍÍÍÍÍÍÍÍÍÍÍÍÍÍÍÍÍÍÍÍÍÍÍÍÍ»
echo   º MAIN MENU               º
echo   ÇÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄÄ¶
echo   º B - Build BOOST         º
echo   º T - Rebuild libttorrent º
echo   º W - Rebuild WebP        º
echo   º Q - Quit                º
echo   ÈÍÍÍÍÍÍÍÍÍÍÍÍÍÍÍÍÍÍÍÍÍÍÍÍÍ¼
echo.
set /p MENU=Enter command:

color %c_normal%
cls
if /I "%menu%"=="B" goto :buildboost
if /I "%menu%"=="T" goto :installtorrent
if /I "%menu%"=="W" goto :installwebp
if /I "%menu%"=="Q" exit
goto mainmenu










rem ***
rem *** BOOST
rem ***

rem BOOST_ROOT                          =  D:\Development\Snappy Driver Installer Origin\trunk\external\boost_%BOOST_VER%
rem BOOST_BUILD_PATH                    =  %BOOST_ROOT%
rem BOOST_INSTALL_PATH                  =  C:\BOOST32_%GCC_VERSION2%
rem BOOST64_INSTALL_PATH                =  C:\BOOST64_%GCC_VERSION2%

:buildboost
TITLE Snappy Driver Installer Origin - Boost
cd D:\Development\Snappy Driver Installer Origin\trunk\external
@echo BOOST_ROOT : %BOOST_ROOT%&echo.

rem bootstrap.bat wants to do msvc so i have to modify it to force gcc
rem goto :skip_unpack

echo Deleting boost
if exist "%BOOST_ROOT%" rd "%BOOST_ROOT%" /s /q
if exist %BOOST_INSTALL_PATH% rd %BOOST_INSTALL_PATH% /s /q
if exist %BOOST64_INSTALL_PATH% rd %BOOST64_INSTALL_PATH% /s /q
pause

if exist boost_%BOOST_VER%.tar del if exist boost_%BOOST_VER%.tar
if exist boost_%BOOST_VER%.tar.gz  "C:\Program Files\7-Zip\7z.exe" x boost_%BOOST_VER%.tar.gz
if exist boost_%BOOST_VER%.tar  "C:\Program Files\7-Zip\7z.exe" x boost_%BOOST_VER%.tar
if exist boost_%BOOST_VER%.tar  del boost_%BOOST_VER%.tar
pause

rem have to modify bootstrap.bat line 15 to add gcc [cxxflags="-std=c++11"]
call :ColorText %c_fail% "Now is the time to add gcc to line 15 of bootstrap.bat"&echo.
pause

:SKIP_UNPACK

rem Build bjam.exe
echo.
call :ColorText %c_do% "Boost Bootstrap"&echo.
pushd %BOOST_ROOT%
set oldpath=%path%
set path=%GCC_PATH%\bin;%path%
call bootstrap.bat gcc 
if exist b2.exe copy b2.exe bjam.exe /Y
set path=%oldpath%
popd
pause

rem Install BOOST (32-bit)
call :ColorText %c_do% "Installing BOOST 32-bit"&echo.
if %BOOST_VER2% LSS 1.65.0 (call :copyecho "libtorrent_patch\socket_types.hpp" "%BOOST_ROOT%\boost\asio\detail\socket_types.hpp" /Y)
pushd %BOOST_ROOT%
set oldpath=%path%
set path=%GCC_PATH%\bin;%BOOST_ROOT%;%path%
bjam.exe install -q --prefix=%BOOST_INSTALL_PATH% --build-type=minimal --layout=system toolset=gcc variant=release link=static threading=multi -std=c++11 -j%NUMBER_OF_PROCESSORS% address-model=32
rem define=BOOST_USE_WINAPI_VERSION=0x0501 define=BOOST_USE_NTDDI_VERSION=0x05010000
bjam.exe install -q --prefix=%BOOST_INSTALL_PATH_DEBUG% --build-type=minimal --layout=system toolset=gcc variant=debug link=static threading=multi -std=c++11 -j%NUMBER_OF_PROCESSORS% address-model=32
rem define=BOOST_USE_WINAPI_VERSION=0x0501 define=BOOST_USE_NTDDI_VERSION=0x05010000
set path=%oldpath%
popd
pause

rem Install BOOST (64-bit)
call :ColorText %c_do% "Installing BOOST 64-bit"&echo.
pushd %BOOST_ROOT%
set oldpath=%path%
set path=%GCC64_PATH%\bin;%BOOST_ROOT%;%path%
bjam.exe install -q --prefix=%BOOST64_INSTALL_PATH% --build-type=minimal --layout=system toolset=gcc variant=release link=static threading=multi -std=c++11 -j%NUMBER_OF_PROCESSORS% address-model=64
rem define=BOOST_USE_WINAPI_VERSION=0x0600 define=BOOST_USE_NTDDI_VERSION=0x0600
bjam.exe install -q --prefix=%BOOST64_INSTALL_PATH_DEBUG% --build-type=minimal --layout=system toolset=gcc variant=debug link=static threading=multi -std=c++11 -j%NUMBER_OF_PROCESSORS% address-model=64
rem define=BOOST_USE_WINAPI_VERSION=0x0600 define=BOOST_USE_NTDDI_VERSION=0x0600
set path=%oldpath%
popd
pause
goto :DONE

rem ./b2 --with-system --with-thread --with-date_time --with-regex --with-serialization stage





















rem ***
rem *** LIBWEBP
rem ***

rem "%GCC_PATH%%GCC_PREFIX%     =   C:\mingw\mingw32\i686-w64-mingw32
rem %GCC64_PATH%%GCC64_PREFIX%  =   C:\mingw\mingw64\x86_64-w64-mingw32

:installwebp
TITLE Snappy Driver Installer Origin - Libwebp
call :ColorText %c_do% "Deleting LibWebP output files"&echo.
if exist "%GCC_PATH%%GCC_PREFIX%\include\webp"           rd "%GCC_PATH%%GCC_PREFIX%\include\webp" /s /q
if exist "%GCC64_PATH%%GCC64_PREFIX%\include\webp"       rd "%GCC64_PATH%%GCC64_PREFIX%\include\webp" /s /q
if exist "%GCC_PATH%%GCC_PREFIX%\lib\libwebp.*"          del "%GCC_PATH%%GCC_PREFIX%\lib\libwebp.*"
if exist "%GCC64_PATH%%GCC64_PREFIX%\lib\libwebp.*"      del "%GCC64_PATH%%GCC64_PREFIX%\lib\libwebp.*"
if exist "%MSYS_PATH%\home\libwebp-%LIBWEBP_VER%"        rd "%MSYS_PATH%\home\libwebp-%LIBWEBP_VER%" /s /q
if exist "%MSYS_PATH%\makewebp.bat"                      del "%MSYS_PATH%\makewebp.bat"
if exist "%MSYS_PATH%\home\makewebp.bat"                 del "%MSYS_PATH%\home\makewebp.bat"
if exist "%MSYS_PATH%\home\unpack.bat"                   del "%MSYS_PATH%\home\unpack.bat"
if exist "%MSYS_PATH%\home\libwebp-%LIBWEBP_VER%.tar.gz" del "%MSYS_PATH%\home\libwebp-%LIBWEBP_VER%.tar.gz"
if exist "%MSYS_PATH%\home\libwebp-%LIBWEBP_VER%.tar"    del "%MSYS_PATH%\home\libwebp-%LIBWEBP_VER%.tar"
pause
call :ColorText %c_do% "Installing LibWebP"&echo.
xcopy webp\mingw\msys\1.0\home %MSYS_PATH%\home /E /I /Y
copy webp\mingw\msys\1.0\makewebp.bat %MSYS_PATH% /Y
echo %GCC_PATH% /mingw32> %MSYS_PATH%\etc\fstab
echo %GCC64_PATH% /mingw64>> %MSYS_PATH%\etc\fstab
echo MSYS_PATH : %MSYS_PATH%
pause

pushd %MSYS_PATH%
rem 'make' fails to create the output directories
if not exist c:\msys64\mingw32\i686-w64-mingw32\include\webp      mkdir c:\msys64\mingw32\i686-w64-mingw32\include\webp
if not exist c:\msys64\mingw32\i686-w64-mingw32\bin               mkdir c:\msys64\mingw32\i686-w64-mingw32\bin
if not exist c:\msys64\mingw32\i686-w64-mingw32\lib\pkgconfig     mkdir c:\msys64\mingw32\i686-w64-mingw32\lib\pkgconfig
if not exist c:\msys64\mingw32\i686-w64-mingw32\share\man\man1    mkdir c:\msys64\mingw32\i686-w64-mingw32\share\man\man1
if not exist c:\msys64\mingw64\x86_64-w64-mingw32\include\webp    mkdir c:\msys64\mingw64\x86_64-w64-mingw32\include\webp
if not exist c:\msys64\mingw64\x86_64-w64-mingw32\bin             mkdir c:\msys64\mingw64\x86_64-w64-mingw32\bin
if not exist c:\msys64\mingw64\x86_64-w64-mingw32\lib\pkgconfig   mkdir c:\msys64\mingw64\x86_64-w64-mingw32\lib\pkgconfig
if not exist c:\msys64\mingw64\x86_64-w64-mingw32\share\man\man1  mkdir c:\msys64\mingw64\x86_64-w64-mingw32\share\man\man1

call makewebp.bat %MSYS_BIN% /mingw32%GCC_PREFIX1% /mingw64%GCC64_PREFIX1%
pause
goto :DONE


















rem ***
rem *** LIBTORRENT
rem ***

rem GCC_PATH%%GCC_PREFIX      =   C:\mingw\mingw32\i686-w64-mingw32
rem GCC64_PATH%%GCC64_PREFIX  =   C:\mingw\mingw64\x86_64-w64-mingw32
rem LIBTORREN32               =   C:\mingw\ming32\i686-w64-mingw32\lib\libtorrent.a
rem LIBTORREN64               =   C:\mingw\mingw64\x86_64-w64-mingw32\lib\libtorrent.a
rem LIBTORRENT_PATH           =   D:\Development\Snappy Driver Installer Origin\trunk\external\libtorrent-libtorrent-%LIBTORRENT_VER%
rem LIBTORRENT_INSTALL_PATH   =   C:\LIBTORRENT32
rem LIBTORRENT64_INSTALL_PATH =   C:\LIBTORRENT64
rem LIBBOOST32                =   C:\mingw\mingw32\i686-w64-mingw32\lib\libboost_system_tr.a
rem LIBBOOST64                =   C:\mingw\mingw64\x86_64-w64-mingw32\lib\libboost_system_tr.a
rem BOOST_ROOT                =   D:\Development\Snappy Driver Installer Origin\trunk\external\boost_%BOOST_VER%
rem BOOST_BUILD_PATH          =   %BOOST_ROOT%
rem BOOST_INSTALL_PATH        =   D:\BOOST32_%GCC_VERSION2%
rem BOOST64_INSTALL_PATH      =   D:\BOOST64_%GCC_VERSION2%

:installtorrent
TITLE Snappy Driver Installer Origin - Libtorrent
call :ColorText %c_do% "Deleting libtorrent output files"&echo.
if exist %LIBTORRENT_INSTALL_PATH%                                     rd %LIBTORRENT_INSTALL_PATH% /s /q
if exist %LIBTORRENT64_INSTALL_PATH%                                   rd %LIBTORRENT64_INSTALL_PATH% /s /q
if exist "%LIBTORRENT_PATH%"                                           rd "%LIBTORRENT_PATH%" /s /q
if exist libtorrent-rasterbar-%LIBTORRENT_VER%                         rd libtorrent-rasterbar-%LIBTORRENT_VER%.tar /s /q
if exist libtorrent-rasterbar-%LIBTORRENT_VER2%                        rd libtorrent-rasterbar-%LIBTORRENT_VER2%.tar /s /q
pause

call :ColorText %c_do% "Unpacking libtorrent files"&echo.
cd D:\Development\Snappy Driver Installer Origin\trunk\external
if exist libtorrent-rasterbar-%LIBTORRENT_VER%.tar      del libtorrent-rasterbar-%LIBTORRENT_VER%.tar
if exist libtorrent-rasterbar-%LIBTORRENT_VER%.tar.gz  "C:\Program Files\7-Zip\7z.exe" x libtorrent-rasterbar-%LIBTORRENT_VER%.tar.gz
if exist libtorrent-rasterbar-%LIBTORRENT_VER%.tar     "C:\Program Files\7-Zip\7z.exe" x libtorrent-rasterbar-%LIBTORRENT_VER%.tar
if exist libtorrent-rasterbar-%LIBTORRENT_VER%.tar     del libtorrent-rasterbar-%LIBTORRENT_VER%.tar
if exist libtorrent-rasterbar-%LIBTORRENT_VER%         ren libtorrent-rasterbar-%LIBTORRENT_VER% libtorrent-libtorrent-%LIBTORRENT_VER%
if exist libtorrent-rasterbar-%LIBTORRENT_VER2%        ren libtorrent-rasterbar-%LIBTORRENT_VER2% libtorrent-libtorrent-%LIBTORRENT_VER%
pause

rem NOTE: 1.2.13+ first build fails with error can't find libtorrent-rasterbar.pc
rem       no known fix yet
rem       running it twice will work because the file is created after the error is thrown
rem NOTE: output file name is libtorrent-rasterbar.a

rem Build libtorrent.a (32-bit)
call :ColorText %c_do% "Building libtorrent 32-bit"&echo.
pushd "%LIBTORRENT_PATH%"
set path=C:\mingw\mingw32\bin;%path%
bjam install -q --prefix=%LIBTORRENT_INSTALL_PATH% --layout=system toolset=gcc variant=release link=static runtime-link=static boost-link=static -std=c++11 -j%NUMBER_OF_PROCESSORS% define=BOOST_USE_WINAPI_VERSION=0x0501 define=BOOST_USE_NTDDI_VERSION=0x0501
bjam install -q --prefix=%LIBTORRENT_INSTALL_PATH% --layout=system toolset=gcc variant=release link=static runtime-link=static boost-link=static -std=c++11 -j%NUMBER_OF_PROCESSORS% define=BOOST_USE_WINAPI_VERSION=0x0501 define=BOOST_USE_NTDDI_VERSION=0x0501
pause
bjam install -q --prefix=%LIBTORRENT_INSTALL_PATH_DEBUG% --layout=system toolset=gcc variant=debug link=static -std=c++11 -j%NUMBER_OF_PROCESSORS% 
pause
popd

rem Build libtorrent.a (64-bit)
call :ColorText %c_do% "Building libtorrent 64-bit"&echo.
set oldpath=%path%
set path=c:\mingw\mingw64\bin;%path%
pushd "%LIBTORRENT_PATH%"
bjam install -q --prefix=%LIBTORRENT64_INSTALL_PATH% --layout=system toolset=gcc variant=release link=static -std=c++11 cflags="-Wa,-mbig-obj" address-model=64 -j%NUMBER_OF_PROCESSORS% define=BOOST_USE_WINAPI_VERSION=0x0501 define=BOOST_USE_NTDDI_VERSION=0x0501
pause
bjam install -q --prefix=%LIBTORRENT64_INSTALL_PATH_DEBUG% --layout=system toolset=gcc variant=debug link=static -std=c++11 cflags="-Wa,-mbig-obj" address-model=64 -j%NUMBER_OF_PROCESSORS% define=BOOST_USE_WINAPI_VERSION=0x0501 define=BOOST_USE_NTDDI_VERSION=0x0501
set path=%oldpath%
pause
popd
goto :DONE









:ColorText
echo off
<nul set /p ".=%DEL%" > "%~2"
findstr /v /a:%1 /R "^$" "%~2" nul
del "%~2" > nul 2>&1
goto :eof


:copyecho
@echo off
echo Copying %1 %2 %3 %4 %5 %6 %7 %8 %9
copy %1 %2 %3 %4 %5 %6 %7 %8 %9
if errorlevel 1 call :ColorText  %c_fail% "Copy failed"&echo.&echo.
goto :eof


:DONE