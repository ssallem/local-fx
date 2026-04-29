; setup.iss - Inno Setup script for LocalFx (Tab Explorer) Native Messaging Host.
;
; This installer is a thin wrapper around installer/windows/install.ps1 — it
; copies fx-host.exe + the install/uninstall PowerShell scripts to %LOCALAPPDATA%
; and then invokes install.ps1 to register the Chrome Native Messaging host
; (HKCU only, no admin required).
;
; All install/uninstall logic lives in install.ps1 / uninstall.ps1; do not
; reimplement registry or manifest writes here.
;
; Build with installer/windows/build-setup.ps1 (locates ISCC.exe automatically).

; AppId is hardcoded below (not a #define) because Inno Setup requires the
; leading "{" of a GUID-style AppId to be doubled ("{{") for escaping, which
; doesn't compose cleanly with #define expansion. Treat the GUID as the stable
; identity of this product — never change it, or upgrades will install side-by-side.
#define MyAppName      "LocalFx Native Host"
#define MyAppVersion   "0.3.0"
#define MyAppPublisher "LocalFx"
#define MyAppURL       "https://github.com/ssallem/local-fx"
; Hardcoded extension IDs (production first, dev second). Order is irrelevant
; to Chrome but production is the more likely target on end-user PCs.
#define ProdExtId      "hkopameeeinhkodnddimfmeogkjnpidf"
#define DevExtId       "cjaibkecpdcabflelcjciceofknnpmck"
#define BothExtIds     ProdExtId + "," + DevExtId

[Setup]
AppId={{B6A2C8E1-4F3D-4B5A-9C7E-1A8D2E5F0B3C}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
DefaultDirName={localappdata}\LocalFx
DefaultGroupName=LocalFx
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=
DisableDirPage=yes
DisableProgramGroupPage=yes
DisableReadyPage=no
OutputDir=..\..\extension\dist-prod
OutputBaseFilename=localfx-host-setup-v{#MyAppVersion}
Compression=lzma2/max
SolidCompression=yes
Uninstallable=yes
UninstallDisplayName={#MyAppName}
UninstallDisplayIcon={app}\fx-host.exe
WizardStyle=modern
MinVersion=10.0

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"
Name: "korean"; MessagesFile: "compiler:Languages\Korean.isl"

[CustomMessages]
english.RegisteringHost=Registering Chrome Native Messaging host...
korean.RegisteringHost=Chrome 네이티브 메시징 호스트 등록 중...

[Files]
; Native host binary (the actual program Chrome will spawn via stdio).
Source: "..\..\native-host\bin\fx-host.exe"; DestDir: "{app}"; Flags: ignoreversion
; Installer scripts that the [Run] / [UninstallRun] sections will invoke.
Source: "install.ps1"; DestDir: "{app}\installer"; Flags: ignoreversion
Source: "uninstall.ps1"; DestDir: "{app}\installer"; Flags: ignoreversion
Source: "com.local.fx.json.tmpl"; DestDir: "{app}\installer"; Flags: ignoreversion

; [Run] intentionally empty — install.ps1 is invoked from [Code]/RegisterNativeHost
; below so we can capture its exit code, surface a MsgBox on failure, and Abort
; the wizard. Using [Run] with `runhidden` swallows non-zero exits silently.

[UninstallRun]
; uninstall.ps1 honors $env:LocalFxKeepFiles=1 to skip its own directory delete
; (Inno Setup needs the {app} tree intact so it can track-delete the files it
; recorded in [Files]). The PS wrapper sets the env var for the child process
; only; -Yes is mandatory because uninstall.ps1 calls Read-Host without it,
; which would block forever in an unattended uninstall.
Filename: "powershell.exe"; \
  Parameters: "-NoProfile -ExecutionPolicy Bypass -Command ""$env:LocalFxKeepFiles='1'; & '{app}\installer\uninstall.ps1' -Yes"""; \
  Flags: runhidden waituntilterminated

[Code]
procedure RegisterNativeHost;
var
  LogDir:     string;
  LogPath:    string;
  PsCommand:  string;
  Params:     string;
  ResultCode: Integer;
  ExecOk:     Boolean;
begin
  LogDir  := ExpandConstant('{localappdata}\LocalFx');
  LogPath := LogDir + '\install.log';

  // Make sure the log directory exists before Start-Transcript opens the file.
  // install.ps1 will (re)create it too, but Start-Transcript runs first.
  if not DirExists(LogDir) then
    ForceDirectories(LogDir);

  // Build the -Command body. We use single-quoted PS strings throughout so
  // the embedded paths don't need to escape backslashes. The outer Inno Setup
  // string uses doubled double-quotes ("") to embed literal " into the
  // process command line.
  PsCommand :=
    'Start-Transcript -Path ''' + LogPath + ''' -Force | Out-Null; ' +
    'try { ' +
    '& ''' + ExpandConstant('{app}\installer\install.ps1') + ''' ' +
      '-Force ' +
      '-HostBinary ''' + ExpandConstant('{app}\fx-host.exe') + ''' ' +
      '-ExtensionId ''{#BothExtIds}''; ' +
    'exit $LASTEXITCODE ' +
    '} finally { Stop-Transcript | Out-Null }';

  Params := '-NoProfile -ExecutionPolicy Bypass -Command "' + PsCommand + '"';

  WizardForm.StatusLabel.Caption := ExpandConstant('{cm:RegisteringHost}');
  // Force the label to repaint before the synchronous Exec blocks the UI thread,
  // otherwise the new caption is invisible until Exec returns.
  WizardForm.Update;

  ExecOk := Exec('powershell.exe', Params, '', SW_HIDE,
                 ewWaitUntilTerminated, ResultCode);

  if (not ExecOk) or (ResultCode <> 0) then
  begin
    MsgBox(
      'Chrome 네이티브 메시징 호스트 등록에 실패했습니다 (코드 ' +
      IntToStr(ResultCode) + '). 자세한 내용은 ' + LogPath + ' 를 확인하세요.',
      mbCriticalError, MB_OK);
    Abort;
  end;
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then
    RegisterNativeHost;
end;
