@echo off
rem ============================================================
rem  yx-android one-click build
rem  Step 1: gomobile compiles the Go engine into yx.aar
rem  Step 2: Gradle builds the signed APK, copied to repo root
rem
rem  Toolchain defaults to the D:\yx-android layout; override via
rem  env vars: JAVA_HOME / ANDROID_HOME / ANDROID_NDK_HOME
rem ============================================================
setlocal
set "ROOT=%~dp0"
if "%ROOT:~-1%"=="\" set "ROOT=%ROOT:~0,-1%"

if not defined JAVA_HOME set "JAVA_HOME=D:\yx-android\jdk-17"
if not defined ANDROID_HOME set "ANDROID_HOME=D:\yx-android\android-sdk"
if not defined ANDROID_NDK_HOME set "ANDROID_NDK_HOME=%ANDROID_HOME%\ndk\26.3.11579264"
set "PATH=%JAVA_HOME%\bin;%PATH%"
rem gobind-generated Java is UTF-8; force javac to read it as UTF-8
set "JAVA_TOOL_OPTIONS=-Dfile.encoding=UTF-8"

rem make go's bin dir available (gomobile/gobind live there)
for /f "delims=" %%i in ('go env GOPATH') do set "PATH=%%i\bin;%PATH%"

rem Gradle: use the one on PATH, fall back to the local install
set "GRADLE_CMD=gradle.bat"
where gradle.bat >nul 2>nul || set "GRADLE_CMD=D:\yx-android\gradle-8.10.2\bin\gradle.bat"

rem machine-specific local.properties (gitignored)
> "%ROOT%\android\local.properties" echo sdk.dir=%ANDROID_HOME:\=\\%

echo == [1/2] gomobile bind ==
if not exist "%ROOT%\android\app\libs" mkdir "%ROOT%\android\app\libs"
cd /d "%ROOT%"
gomobile bind -target=android/arm64 -androidapi 26 -o android\app\libs\yx.aar .\androidlib
if errorlevel 1 goto :err

echo == [2/2] gradle assembleRelease ==
cd /d "%ROOT%\android"
call %GRADLE_CMD% assembleRelease --console=plain
if errorlevel 1 goto :err

copy /y app\build\outputs\apk\release\app-release.apk "%ROOT%\yx-android.apk" >nul
echo.
echo Build OK: %ROOT%\yx-android.apk
exit /b 0

:err
echo.
echo Build FAILED, check the log above
exit /b 1
