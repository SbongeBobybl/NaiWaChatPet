#define MyAppName "NaiWaChatPet"
[Setup]
AppId={{D21E3D70-BC1D-4AFE-A890-6C147AF78026}
AppName=奶蛙桌宠
AppVersion=2.0.1
AppVerName=奶蛙桌宠 V2.0.1
AppPublisher=NaiWaChatPet
DefaultDirName={localappdata}\Programs\NaiWaChatPet
DefaultGroupName=奶蛙桌宠
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0.19041
OutputDir=..\Release
OutputBaseFilename=NaiWaChatPet-Setup-v2.0.1-x64
SetupIconFile=..\Source\NaiWaChatPet\icons\NaiWaChatPet.ico
UninstallDisplayIcon={app}\NaiWaChatPet.exe
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
DisableProgramGroupPage=yes
LicenseFile=..\Source\DeskBox-1.5.8\LICENSE
CloseApplications=yes
CloseApplicationsFilter=NaiWaChatPet.exe,DeskBox.exe
[Tasks]
Name: "desktopicon"; Description: "创建奶蛙桌面快捷方式"; Flags: unchecked
[Files]
Source: "..\Release\NaiWaChatPet-v2.0.1\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs
[Icons]
Name: "{group}\奶蛙桌宠"; Filename: "{app}\NaiWaChatPet.exe"; WorkingDir: "{app}"
Name: "{autodesktop}\奶蛙桌宠"; Filename: "{app}\NaiWaChatPet.exe"; WorkingDir: "{app}"; Tasks: desktopicon
[Run]
Filename: "{app}\NaiWaChatPet.exe"; Parameters: "-no-shortcut"; Description: "启动奶蛙桌宠"; Flags: nowait postinstall skipifsilent
