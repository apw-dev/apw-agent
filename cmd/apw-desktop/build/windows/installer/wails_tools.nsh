!ifndef INFO_PROJECTNAME
    !define INFO_PROJECTNAME "APW"
!endif
!ifndef INFO_COMPANYNAME
    !define INFO_COMPANYNAME "APW"
!endif
!ifndef INFO_PRODUCTNAME
    !define INFO_PRODUCTNAME "APW"
!endif
!ifndef INFO_PRODUCTVERSION
    !define INFO_PRODUCTVERSION "0.1.0"
!endif
!ifndef INFO_COPYRIGHT
    !define INFO_COPYRIGHT "MIT License"
!endif
!ifndef PRODUCT_EXECUTABLE
    !define PRODUCT_EXECUTABLE "APW.exe"
!endif
!ifndef UNINST_KEY_NAME
    !define UNINST_KEY_NAME "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}"
!endif
!define UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${UNINST_KEY_NAME}"

!macro wails.setShellContext
    SetShellVarContext current
!macroend

!macro wails.checkArchitecture
!macroend

!macro wails.installwebview2
!macroend

!macro wails.files
    SetOutPath "$INSTDIR"
    File "/oname=${PRODUCT_EXECUTABLE}" "..\..\bin\APW.exe"
    File /nonfatal "..\..\bin\apwd.exe"
    File /nonfatal "..\..\bin\apw.example.json"
    SetOutPath "$INSTDIR\bin"
    File /nonfatal "..\..\bin\cli\apw.exe"
    File /nonfatal "..\..\bin\apwd.exe"
    SetOutPath "$INSTDIR"
!macroend

!macro wails.writeUninstaller
    WriteUninstaller "$INSTDIR\uninstall.exe"
    WriteRegStr HKCU "${UNINST_KEY}" "Publisher" "${INFO_COMPANYNAME}"
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayName" "${INFO_PRODUCTNAME}"
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayVersion" "${INFO_PRODUCTVERSION}"
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    WriteRegStr HKCU "${UNINST_KEY}" "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
    WriteRegStr HKCU "${UNINST_KEY}" "QuietUninstallString" "$\"$INSTDIR\uninstall.exe$\" /S"
!macroend

!macro wails.deleteUninstaller
    Delete "$INSTDIR\uninstall.exe"
    DeleteRegKey HKCU "${UNINST_KEY}"
!macroend
