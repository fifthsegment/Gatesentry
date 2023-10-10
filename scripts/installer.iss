[Setup]
AppName=Gatesentry
AppVersion=1.0
DefaultDirName={pf}\Gatesentry
DefaultGroupName=Gatesentry
UninstallDisplayIcon={app}\gatesentry-windows.exe
OutputBaseFilename=GatesentrySetup
Compression=lzma
SolidCompression=yes
PrivilegesRequired=admin

[Files]
Source: "gatesentry-windows.exe"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\Gatesentry"; Filename: "{app}\gatesentry-windows.exe"
Name: "{commondesktop}\Gatesentry"; Filename: "{app}\gatesentry-windows.exe"; Tasks: desktopicon
Name: "{group}\Start Gatesentry Service"; Filename: "{cmd}"; Parameters: "/C sc start GateSentry"; WorkingDir: "{app}"
Name: "{group}\Stop Gatesentry Service"; Filename: "{cmd}"; Parameters: "/C sc stop GateSentry"; WorkingDir: "{app}"

[Tasks]
Name: desktopicon; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"

[Run]
Filename: "{app}\gatesentry-windows.exe"; Parameters: "-service install"; Description: "Install Gatesentry Service"; Flags: nowait postinstall skipifsilent runascurrentuser
Filename: "{app}\gatesentry-windows.exe"; Description: "Launch Gatesentry"; Flags: nowait postinstall skipifsilent
Filename: "{cmd}"; Parameters: "/C sc start GateSentry"; Description: "Start Gatesentry Service"; Flags: postinstall skipifsilent

[Code]
var
  LastStep: TSetupStep;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  LastStep := CurStep;
end;

procedure DeinitializeSetup();
begin
  { Check if setup has reached the post-install step }
  if LastStep = ssPostInstall then
  begin
    MsgBox('The installation was successfully completed. To manage Gatesentry, visit http://localhost:10786 in any web browser.', mbInformation, MB_OK);
  end
end;
