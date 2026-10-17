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
; Credential Manager entries stay unless the user answers Yes. Yes runs
; datasafe-agent.exe --purge-secrets while the binary is still installed.
; The agent deletes targets that end in .datasafe-desktop. It does not
; parse cmdkey output. /S does not ask and does not remove them. Forget
; profile inside the app removes one profile's secrets. Skip both deletes
; during an update.

!macro NSIS_HOOK_PREUNINSTALL
  ${If} $UpdateMode <> 1
    MessageBox MB_YESNO|MB_DEFBUTTON2 "Remove saved sign-in secrets from Windows Credential Manager? / Удалить сохранённые секреты входа из диспетчера учётных данных?$\n$\nLeave this as No to sign in again after reinstalling. / Оставьте «Нет», чтобы войти снова после переустановки.$\n$\n%APPDATA%\DataSafeS3 is kept either way." /SD IDNO IDYES forget_datasafe_secrets
    Goto datasafe_secrets_decided
    forget_datasafe_secrets:
      IfFileExists "$INSTDIR\binaries\datasafe-agent.exe" 0 datasafe_secrets_decided
      ClearErrors
      ExecWait '"$INSTDIR\binaries\datasafe-agent.exe" --purge-secrets' $R9
      ${If} ${Errors}
        MessageBox MB_OK|MB_ICONEXCLAMATION "Saved sign-in secrets could not be removed. / Не удалось удалить сохранённые секреты входа."
      ${ElseIf} $R9 <> 0
        MessageBox MB_OK|MB_ICONEXCLAMATION "Saved sign-in secrets could not be removed. / Не удалось удалить сохранённые секреты входа."
      ${EndIf}
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
  ${EndIf}
!macroend
