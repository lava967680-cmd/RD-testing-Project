; ============================================================
; ControlHub Windows Agent — Inno Setup Installer Script
; Produces: ControlHub-Agent-Setup.exe  (~8 MB, single file)
;
; Requirements to BUILD this installer:
;   - Inno Setup 6.x  (https://jrsoftware.org/isinfo.php)
;   - The compiled agent binary: controlhub-agent.exe
;   - ControlHub logo: assets\logo.bmp (55x55 px)
;
; Clients only need to DOUBLE-CLICK the output .exe — nothing else.
; ============================================================

#define AppName      "ControlHub Agent"
#define AppVersion   "1.0.0"
#define AppPublisher "ControlHub Technologies"
#define AppURL       "https://controlhub.io"
#define AppExeName   "controlhub-agent.exe"
#define ServiceName  "ControlHubAgent"

[Setup]
; Basic info
AppId={{A7F3C2D1-8B4E-4F2A-9C6D-1E5F8A3B7C2D}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisherURL={#AppURL}
AppSupportURL={#AppURL}/support
AppUpdatesURL={#AppURL}/updates
VersionInfoVersion={#AppVersion}
VersionInfoCompany={#AppPublisher}
VersionInfoDescription=ControlHub Authorized Device Management Agent

; Installation settings
DefaultDirName={commonpf64}\ControlHub\Agent
DefaultGroupName=ControlHub
DisableProgramGroupPage=yes
OutputDir=dist
OutputBaseFilename=ControlHub-Agent-Setup
SetupIconFile=assets\logo.ico
UninstallDisplayIcon={app}\{#AppExeName}
UninstallDisplayName={#AppName}

; Appearance
WizardStyle=modern
WizardSmallImageFile=assets\logo.bmp
WizardImageFile=assets\banner.bmp
DisableWelcomePage=no

; Privileges — requires admin to install as Windows Service
PrivilegesRequired=admin
PrivilegesRequiredOverridesAllowed=

; Compression
Compression=lzma2/ultra64
SolidCompression=yes
LZMAUseSeparateProcess=yes

; Single merged exe output (no separate files needed)
ArchitecturesInstallIn64BitMode=x64

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
; Optional: auto-start with Windows (enabled by default)
Name: "autostart"; Description: "Start ControlHub Agent automatically with Windows (recommended)"; GroupDescription: "Additional tasks:"; Flags: checkedonce

[Files]
; Main agent binary (compiled by Rust — see build pipeline)
Source: "build\controlhub-agent.exe"; DestDir: "{app}"; Flags: ignoreversion

; Configuration file (enrollment token entered by user)
Source: "assets\agent-config.toml"; DestDir: "{app}"; Flags: onlyifdoesntexist

; Visual C++ redistributable if needed (optional)
; Source: "redist\vc_redist.x64.exe"; DestDir: "{tmp}"; Flags: deleteafterinstall

[Icons]
; Add to Start Menu
Name: "{group}\ControlHub Agent"; Filename: "{app}\{#AppExeName}"
Name: "{group}\Uninstall ControlHub Agent"; Filename: "{uninstallexe}"

[Registry]
; Store installation path for the agent to find its config
Root: HKLM; Subkey: "SOFTWARE\ControlHub\Agent"; ValueType: string; ValueName: "InstallPath"; ValueData: "{app}"; Flags: uninsdeletekey
Root: HKLM; Subkey: "SOFTWARE\ControlHub\Agent"; ValueType: string; ValueName: "Version"; ValueData: "{#AppVersion}"

[Run]
; Install as Windows Service (runs in background, survives restarts)
Filename: "sc.exe"; Parameters: "create ""{#ServiceName}"" binPath= ""{app}\{#AppExeName}"" start= auto DisplayName= ""ControlHub Agent"""; Flags: runhidden waituntilterminated; StatusMsg: "Installing ControlHub Agent service..."

; Set service description
Filename: "sc.exe"; Parameters: "description ""{#ServiceName}"" ""Authorized device monitoring and management by ControlHub"""; Flags: runhidden waituntilterminated

; Start the service immediately after install
Filename: "sc.exe"; Parameters: "start ""{#ServiceName}"""; Flags: runhidden waituntilterminated; StatusMsg: "Starting ControlHub Agent..."

[UninstallRun]
; Stop and remove the service on uninstall
Filename: "sc.exe"; Parameters: "stop ""{#ServiceName}"""; Flags: runhidden waituntilterminated
Filename: "sc.exe"; Parameters: "delete ""{#ServiceName}"""; Flags: runhidden waituntilterminated

[Code]
// ── Custom installer pages ────────────────────────────────────────────────

var
  EnrollmentPage: TInputQueryWizardPage;

procedure InitializeWizard();
begin
  // Page 1: Ask for the enrollment token (provided by the IT admin)
  EnrollmentPage := CreateInputQueryPage(
    wpSelectDir,
    'Connect to Your Organization',
    'Enter the enrollment token provided by your IT administrator.',
    ''
  );
  EnrollmentPage.Add('Enrollment Token (CH-ENROLL-...):', False);
  EnrollmentPage.Add('Server URL (leave blank for cloud):', False);

  // Pre-fill the server URL with the default cloud endpoint
  EnrollmentPage.Values[1] := 'https://api.controlhub.io';
end;

function NextButtonClick(CurPageID: Integer): Boolean;
var
  Token: String;
begin
  Result := True;

  // Validate the enrollment token format before proceeding
  if CurPageID = EnrollmentPage.ID then
  begin
    Token := Trim(EnrollmentPage.Values[0]);

    if Token = '' then
    begin
      MsgBox(
        'Please enter your enrollment token.' + #13#10 +
        'You can find it in the ControlHub Admin Console under Devices → Enroll New Device.',
        mbError, MB_OK
      );
      Result := False;
      Exit;
    end;

    if (Length(Token) < 10) or (Copy(Token, 1, 10) <> 'CH-ENROLL-') then
    begin
      MsgBox(
        'The enrollment token must start with CH-ENROLL-' + #13#10 +
        'Please check the token and try again.',
        mbError, MB_OK
      );
      Result := False;
      Exit;
    end;
  end;
end;

procedure CurStepChanged(CurStep: TSetupStep);
var
  ConfigPath: String;
  Token, ServerURL: String;
  ConfigContent: TArrayOfString;
begin
  // After files are copied, write the config with the enrollment token
  if CurStep = ssPostInstall then
  begin
    Token     := Trim(EnrollmentPage.Values[0]);
    ServerURL := Trim(EnrollmentPage.Values[1]);

    if ServerURL = '' then
      ServerURL := 'https://api.controlhub.io';

    ConfigPath := ExpandConstant('{app}\agent-config.toml');

    SetArrayLength(ConfigContent, 8);
    ConfigContent[0] := '# ControlHub Agent Configuration';
    ConfigContent[1] := '# Auto-generated during installation — do not edit manually.';
    ConfigContent[2] := '';
    ConfigContent[3] := '[agent]';
    ConfigContent[4] := 'enrollment_token = "' + Token + '"';
    ConfigContent[5] := 'server_url       = "' + ServerURL + '"';
    ConfigContent[6] := 'version          = "' + ExpandConstant('{#AppVersion}') + '"';
    ConfigContent[7] := 'log_level        = "info"';

    SaveStringsToFile(ConfigPath, ConfigContent, False);
  end;
end;

function UpdateReadyMemo(Space, NewLine, MemoUserInfoInfo, MemoDirInfo,
  MemoTypeInfo, MemoComponentsInfo, MemoGroupInfo, MemoTasksInfo: String): String;
begin
  Result :=
    'ControlHub Agent will be installed with these settings:' + NewLine + NewLine +
    MemoDirInfo + NewLine +
    'Enrollment Token: ' + Copy(Trim(EnrollmentPage.Values[0]), 1, 20) + '...' + NewLine +
    'Server URL: ' + Trim(EnrollmentPage.Values[1]) + NewLine + NewLine +
    'The agent will run as a Windows Service in the background.' + NewLine +
    'A green tray icon will appear when the agent is connected.';
end;
