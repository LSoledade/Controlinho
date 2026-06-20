@echo off
REM ============================================================
REM  pc-remote — auto-start installer (Task Scheduler, logon)
REM ============================================================
REM  Registers the current .exe to launch at user logon, hidden,
REM  so the server comes back on every boot. Re-runnable.
REM ============================================================
setlocal enableextensions

REM Resolve the directory of this script and the executable next to it.
set "SCRIPT_DIR=%~dp0"
set "SCRIPT_DIR=%SCRIPT_DIR:~0,-1%"
set "EXE=%SCRIPT_DIR%\pc-remote.exe"

if not exist "%EXE%" (
  echo [install] Executavel nao encontrado: %EXE%
  echo [install] Compile com: go build -o pc-remote.exe
  exit /b 1
)

REM Task name + a per-user task so no admin elevation is needed.
set "TASK=pc-remote"

REM Remove any previous registration (ignore errors if absent).
schtasks /query /tn "%TASK%" >nul 2>&1
if %errorlevel%==0 (
  schtasks /end /tn "%TASK%" >nul 2>&1
  schtasks /delete /tn "%TASK%" /f >nul 2>&1
)

REM Register: trigger at user logon, run as current user, hidden window.
schtasks /create /tn "%TASK%" /tr "\"%EXE%\"" /sc onlogon /rl limited /f
if %errorlevel% neq 0 (
  echo [install] Falha ao criar a tarefa agendada.
  exit /b 1
)

REM Open the firewall for both ports (needs admin; ignored if not elevated).
netsh advfirewall firewall delete rule name="pc-remote" >nul 2>&1
netsh advfirewall firewall add rule name="pc-remote" dir=in action=allow protocol=TCP localport=8080,8443 >nul 2>&1
if %errorlevel%==0 (
  echo [install] Regra de firewall criada para as portas 8080/8443.
) else (
  echo [install] AVISO: nao foi possivel criar a regra de firewall.
  echo [install]        Rode este .bat como administrador, ou libere as portas
  echo [install]        8080 e 8443 manualmente no Firewall do Windows.
)

REM Start it now so you don't have to log off/on.
schtasks /run /tn "%TASK%" >nul 2>&1

echo.
echo [install] Tarefa "%TASK%" criada com sucesso.
echo [install] O servidor iniciara automaticamente no logon.
echo [install] Executavel: %EXE%
echo.
echo [install] No celular: abra http://SEU-IP:8080 e siga o passo a passo
echo [install] para instalar o certificado e o app (PWA).
echo.
pause
endlocal
