; Cloud Manager Per-User Windows Installer (Zero Admin / Zero UAC Prompt)
#define MyAppName "Cloud Manager"
#define MyAppVersion "0.1.0"
#define MyAppPublisher "Mostakim2005"
#define MyAppURL "https://github.com/Mostakim2005/cloud-manager"
#define MyAppExeName "CloudManager.exe"

[Setup]
AppId={{D9C2E3F4-8B1A-4A5E-9F1C-5E7B3A9D2C10}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}
DefaultDirName={localappdata}\Programs\CloudManager
DefaultGroupName={#MyAppName}
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
OutputDir=..\build
OutputBaseFilename=CloudManager-Setup
Compression=lzma2/ultra64
SolidCompression=yes
WizardStyle=modern
CloseApplications=yes
RestartApplications=no

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"
Name: "startup"; Description: "Start silently in background tray when Windows signs in (~0% idle CPU)"; GroupDescription: "Background Integration:"

[Files]
Source: "..\build\CloudManager.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\build\cloud-manager.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\build\cloud-managerd.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\build\CloudManager.exe"; DestDir: "{%USERPROFILE}\.local\bin"; Flags: ignoreversion
Source: "..\build\cloud-manager.exe"; DestDir: "{%USERPROFILE}\.local\bin"; Flags: ignoreversion
Source: "..\build\cloud-managerd.exe"; DestDir: "{%USERPROFILE}\.local\bin"; Flags: ignoreversion

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Comment: "Open Cloud Manager Dashboard & System Tray"
Name: "{group}\Uninstall {#MyAppName}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon; Comment: "Open Cloud Manager Dashboard"

[Registry]
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "CloudManager"; ValueData: """{app}\{#MyAppExeName}"" --background"; Flags: uninsdeletevalue; Tasks: startup
Root: HKCU; Subkey: "Software\Classes\cloud-manager"; ValueType: string; ValueName: ""; ValueData: "URL:Cloud Manager Protocol"; Flags: uninsdeletekey
Root: HKCU; Subkey: "Software\Classes\cloud-manager"; ValueType: string; ValueName: "URL Protocol"; ValueData: ""
Root: HKCU; Subkey: "Software\Classes\cloud-manager\shell\open\command"; ValueType: string; ValueName: ""; ValueData: """{app}\cloud-manager.exe"" ""open-uri"" ""%1"""

[Run]
Filename: "{app}\{#MyAppExeName}"; Parameters: "--background"; Description: "Start Cloud Manager silently in background tray"; Flags: nowait runhidden

[Code]
procedure StopRunningDaemon();
var
  ResultCode: Integer;
  CliApp: String;
  CliLocal: String;
begin
  CliApp := ExpandConstant('{app}\cloud-manager.exe');
  if FileExists(CliApp) then
    Exec(CliApp, 'quit', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  CliLocal := ExpandConstant('{%USERPROFILE}\.local\bin\cloud-manager.exe');
  if FileExists(CliLocal) then
    Exec(CliLocal, 'quit', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  StopRunningDaemon();
  Result := '';
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usUninstall then
    StopRunningDaemon();
end;
