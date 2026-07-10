Unicode true
ManifestDPIAware true

!define PRODUCT_NAME "uniTerm"
!define BINARY "uniTerm.exe"
!ifndef VERSION
  !define VERSION "dev"
!endif

Name "${PRODUCT_NAME}"
OutFile "..\..\bin\uniTerm-amd64-installer.exe"
InstallDir "$PROGRAMFILES64\${PRODUCT_NAME}"
RequestExecutionLevel admin
SetCompressor /SOLID lzma
SetCompressorDictSize 128

!include "MUI2.nsh"
!include "LogicLib.nsh"
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN "$INSTDIR\${BINARY}"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

Function .onInit
  Call CheckAndCloseProcess
FunctionEnd

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English"

Function CheckAndCloseProcess
  check_process:
  ; Use findstr to filter tasklist output: only match if process name appears in output
  nsExec::ExecToStack 'cmd /c tasklist /FI "IMAGENAME eq ${BINARY}" /NH 2>nul | findstr /I "${BINARY}"'
  Pop $0
  Pop $1
  ; findstr returns 0 only if the process name is found in output
  ${If} $0 != "0"
    Return
  ${EndIf}

  ; Process is running — ask user
  MessageBox MB_YESNO|MB_ICONQUESTION "${PRODUCT_NAME} 正在运行。安装前需要先关闭它。是否强制关闭进程？" /SD IDYES IDNO no_kill
  nsExec::ExecToStack 'cmd /c taskkill /F /IM "${BINARY}"'
  Pop $0
  Sleep 1500
  Goto check_process

  no_kill:
  MessageBox MB_OK|MB_ICONEXCLAMATION "请手动关闭 ${PRODUCT_NAME} 后再继续安装。"
  Abort
FunctionEnd

Section "Install"
  SetOutPath "$INSTDIR"
  File "/oname=${BINARY}" "${ARG_WAILS_AMD64_BINARY}"
  CreateShortCut "$DESKTOP\${PRODUCT_NAME}.lnk" "$INSTDIR\${BINARY}"
  CreateDirectory "$SMPROGRAMS\${PRODUCT_NAME}"
  CreateShortCut "$SMPROGRAMS\${PRODUCT_NAME}\${PRODUCT_NAME}.lnk" "$INSTDIR\${BINARY}"
  CreateShortCut "$SMPROGRAMS\${PRODUCT_NAME}\Uninstall.lnk" "$INSTDIR\uninstall.exe"
  WriteUninstaller "$INSTDIR\uninstall.exe"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${PRODUCT_NAME}" "DisplayName" "${PRODUCT_NAME}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${PRODUCT_NAME}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${PRODUCT_NAME}" "DisplayIcon" '"$INSTDIR\${BINARY}"'
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${PRODUCT_NAME}" "Publisher" "uniTerm"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${PRODUCT_NAME}" "DisplayVersion" "${VERSION}"
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${PRODUCT_NAME}" "NoModify" 1
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${PRODUCT_NAME}" "NoRepair" 1
SectionEnd

Section "Uninstall"
  Delete "$INSTDIR\${BINARY}"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
  Delete "$DESKTOP\${PRODUCT_NAME}.lnk"
  Delete "$SMPROGRAMS\${PRODUCT_NAME}\${PRODUCT_NAME}.lnk"
  Delete "$SMPROGRAMS\${PRODUCT_NAME}\Uninstall.lnk"
  RMDir "$SMPROGRAMS\${PRODUCT_NAME}"
  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${PRODUCT_NAME}"
SectionEnd
