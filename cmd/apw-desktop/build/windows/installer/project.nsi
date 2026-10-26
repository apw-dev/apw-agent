Unicode true

!include "wails_tools.nsh"
!include "MUI2.nsh"

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\APW-Setup-${INFO_PRODUCTVERSION}.exe"
InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
InstallDirRegKey HKCU "Software\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}" "InstallDir"
RequestExecutionLevel user
ShowInstDetails show
ShowUninstDetails show

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN "$INSTDIR\${PRODUCT_EXECUTABLE}"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

Section "APW Desktop & Core CLI (required)" SecCore
    SectionIn RO
    !insertmacro wails.setShellContext
    !insertmacro wails.checkArchitecture

    SetOutPath "$INSTDIR"
    !insertmacro wails.files

    WriteRegStr HKCU "Software\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}" "InstallDir" "$INSTDIR"

    CreateDirectory "$SMPROGRAMS\${INFO_PRODUCTNAME}"
    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}" "" "$INSTDIR\${PRODUCT_EXECUTABLE}" 0
    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}\Uninstall ${INFO_PRODUCTNAME}.lnk" "$INSTDIR\uninstall.exe"

    !insertmacro wails.writeUninstaller
SectionEnd

Section /o "Add to User PATH (optional)" SecPath
    ; Never read or write PATH with NSIS string functions: they stop at 1024 characters and
    ; return "" for a longer PATH, which would replace the whole user PATH. apw.exe edits the
    ; registry value in full, keeps its type, and changes nothing when it cannot read it.
    nsExec::ExecToLog '"$INSTDIR\bin\apw.exe" path add "$INSTDIR\bin"'
    Pop $0
    StrCmp $0 "0" path_done
        MessageBox MB_ICONEXCLAMATION|MB_OK "APW could not add $INSTDIR\bin to your PATH (exit $0). Your PATH was not changed; add the folder yourself if you need the CLI in a terminal."
path_done:
SectionEnd

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
    !insertmacro MUI_DESCRIPTION_TEXT ${SecCore} "Installs APW.exe (native Windows Desktop shell), bin\apw.exe (CLI), and apwd.exe (background daemon) with a Start Menu shortcut and uninstaller."
    !insertmacro MUI_DESCRIPTION_TEXT ${SecPath} "Optionally adds the APW CLI folder ($INSTDIR\bin) to your user PATH so apw and apwd can be run from any terminal."
!insertmacro MUI_FUNCTION_DESCRIPTION_END

Section "Uninstall"
    !insertmacro wails.setShellContext

    ; Remove only this folder from the user PATH (a no-op when it was never added).
    nsExec::ExecToLog '"$INSTDIR\bin\apw.exe" path remove "$INSTDIR\bin"'
    Pop $0

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}"
    Delete "$INSTDIR\${PRODUCT_EXECUTABLE}"
    Delete "$INSTDIR\apwd.exe"
    Delete "$INSTDIR\apw.example.json"
    Delete "$INSTDIR\bin\apw.exe"
    Delete "$INSTDIR\bin\apwd.exe"
    RMDir "$INSTDIR\bin"

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}\${INFO_PRODUCTNAME}.lnk"
    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}\Uninstall ${INFO_PRODUCTNAME}.lnk"
    RMDir "$SMPROGRAMS\${INFO_PRODUCTNAME}"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.deleteUninstaller
    DeleteRegKey HKCU "Software\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
    RMDir "$INSTDIR"
SectionEnd
