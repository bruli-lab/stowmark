#ifndef AppVersion
  #define AppVersion "0.0.0-devel"
#endif

#ifndef SourceExe
  #define SourceExe "stowmark.exe"
#endif

#ifndef OutputDir
  #define OutputDir "."
#endif

[Setup]
AppId=Stowmark
AppName=Stowmark
AppVersion={#AppVersion}
AppPublisher=bruli-lab
AppPublisherURL=https://stowmark.dev
AppSupportURL=https://github.com/bruli-lab/stowmark/issues
AppUpdatesURL=https://github.com/bruli-lab/stowmark/releases
DefaultDirName={autopf}\Stowmark
DisableProgramGroupPage=yes
LicenseFile=..\..\LICENSE
OutputDir={#OutputDir}
OutputBaseFilename=stowmark_{#AppVersion}_windows_amd64_setup
Compression=lzma2
SolidCompression=yes
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
PrivilegesRequired=admin
ChangesEnvironment=yes
UninstallDisplayName=Stowmark
UninstallDisplayIcon={app}\stowmark.exe
WizardStyle=modern

[Tasks]
Name: "addtopath"; Description: "Add Stowmark to the system PATH"; GroupDescription: "Additional options:"; Flags: checkedonce

[Files]
Source: "{#SourceExe}"; DestDir: "{app}"; Flags: ignoreversion

[Code]
const
  EnvironmentKey = 'SYSTEM\CurrentControlSet\Control\Session Manager\Environment';

function RemovePathEntry(const Paths, Entry: String): String;
var
  Remaining: String;
  Current: String;
  Separator: Integer;
begin
  Result := '';
  Remaining := Paths;

  while Remaining <> '' do
  begin
    Separator := Pos(';', Remaining);
    if Separator = 0 then
    begin
      Current := Remaining;
      Remaining := '';
    end
    else
    begin
      Current := Copy(Remaining, 1, Separator - 1);
      Delete(Remaining, 1, Separator);
    end;

    Current := Trim(Current);
    if (Current <> '') and (CompareText(Current, Entry) <> 0) then
    begin
      if Result <> '' then
        Result := Result + ';';
      Result := Result + Current;
    end;
  end;
end;

procedure AddInstallDirectoryToPath;
var
  Paths: String;
  InstallDirectory: String;
begin
  InstallDirectory := ExpandConstant('{app}');
  if not RegQueryStringValue(HKLM64, EnvironmentKey, 'Path', Paths) then
    Paths := '';

  Paths := RemovePathEntry(Paths, InstallDirectory);
  if Paths <> '' then
    Paths := Paths + ';';

  RegWriteExpandStringValue(
    HKLM64,
    EnvironmentKey,
    'Path',
    Paths + InstallDirectory
  );
end;

procedure RemoveInstallDirectoryFromPath;
var
  Paths: String;
begin
  if RegQueryStringValue(HKLM64, EnvironmentKey, 'Path', Paths) then
    RegWriteExpandStringValue(
      HKLM64,
      EnvironmentKey,
      'Path',
      RemovePathEntry(Paths, ExpandConstant('{app}'))
    );
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if (CurStep = ssPostInstall) and WizardIsTaskSelected('addtopath') then
    AddInstallDirectoryToPath;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usPostUninstall then
    RemoveInstallDirectoryFromPath;
end;
