@echo off
REM ============================================================
REM  pc-remote — auto-start installer
REM ------------------------------------------------------------
REM  All the logic now lives in the binary itself. This .bat is
REM  just a double-click entry point for:  pc-remote.exe -install
REM  (registers a logon task + opens the firewall, then starts).
REM  To remove it later:  pc-remote.exe -uninstall
REM ============================================================
set "EXE=%~dp0pc-remote.exe"

if not exist "%EXE%" (
  echo [install] Executavel nao encontrado: %EXE%
  echo [install] Compile com: go build -ldflags "-s -w -H windowsgui" -o pc-remote.exe .
  pause
  exit /b 1
)

"%EXE%" -install
echo.
pause
