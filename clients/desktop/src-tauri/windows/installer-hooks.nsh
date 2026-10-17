; Remove the install-location key the NSIS template writes at
; HKCU\Software\datasafe\DataSafeS3 Desktop (manufacturer is the second
; label of com.datasafe.desktop). The stock uninstaller deletes that key
; only when the "delete app data" checkbox is checked.
; Silent uninstall is uninstall.exe /S. That skips the stock checkbox, so
; %APPDATA%\DataSafeS3 (profiles, journal, logs) is left in place. There is
; no /S option that also deletes it. The checkbox itself only removes
; %APPDATA%\com.datasafe.desktop and %LOCALAPPDATA%\com.datasafe.desktop.
; Those are not the data directory. This hook also removes
; %LOCALAPPDATA%\com.datasafe.desktop, the WebView2 profile. It holds no
; user data. %APPDATA%\DataSafeS3 stays.
; Credential Manager entries (target suffix .datasafe-desktop) stay unless
; the user opts in. /S does not ask and does not remove them, so a reinstall
; can sign in again. Forget profile inside the app is the other way to
; remove one profile's secrets. Skip both deletes during an update.

Var DeleteDataSafeSecrets

!macro NSIS_HOOK_PREUNINSTALL
  StrCpy $DeleteDataSafeSecrets 0
  ${If} $UpdateMode <> 1
    MessageBox MB_YESNO|MB_DEFBUTTON2 "Remove saved sign-in secrets from Windows Credential Manager? / Удалить сохранённые секреты входа из диспетчера учётных данных?$\n$\nLeave this as No to sign in again after reinstalling. / Оставьте «Нет», чтобы войти снова после переустановки.$\n$\n%APPDATA%\DataSafeS3 is kept either way." /SD IDNO IDYES forget_datasafe_secrets
    Goto datasafe_secrets_decided
    forget_datasafe_secrets:
      StrCpy $DeleteDataSafeSecrets 1
    datasafe_secrets_decided:
  ${EndIf}
!macroend

!macro NSIS_HOOK_POSTUNINSTALL
  ${If} $UpdateMode <> 1
    DeleteRegKey HKCU "Software\datasafe\DataSafeS3 Desktop"
    DeleteRegKey /ifempty HKCU "Software\datasafe"
    DeleteRegKey HKLM "Software\datasafe\DataSafeS3 Desktop"
    DeleteRegKey /ifempty HKLM "Software\datasafe"
    RMDir /r "$LOCALAPPDATA\com.datasafe.desktop"
    ${If} $DeleteDataSafeSecrets = 1
      FileOpen $0 "$TEMP\datasafe-forget-secrets.ps1" w
      FileWrite $0 "cmdkey /list | ForEach-Object {$\r$\n"
      FileWrite $0 "  if ($$_ -match 'Target:\s*(.*datasafe-desktop)\s*$$') {$\r$\n"
      FileWrite $0 "    $$name = $$Matches[1].Trim()$\r$\n"
      FileWrite $0 "    if ($$name -ne '') { cmdkey /delete:$$name | Out-Null }$\r$\n"
      FileWrite $0 "  }$\r$\n"
      FileWrite $0 "}$\r$\n"
      FileClose $0
      ExecWait 'powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$TEMP\datasafe-forget-secrets.ps1"'
      Delete "$TEMP\datasafe-forget-secrets.ps1"
    ${EndIf}
  ${EndIf}
!macroend
