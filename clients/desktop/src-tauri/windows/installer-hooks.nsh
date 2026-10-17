; Remove the install-location key the NSIS template writes at
; HKCU\Software\datasafe\DataSafeS3 Desktop (manufacturer is the second
; label of com.datasafe.desktop). The stock uninstaller deletes that key
; only when the "delete app data" checkbox is checked.
; Silent uninstall is uninstall.exe /S. That skips the checkbox, so
; %APPDATA%\DataSafeS3 (profiles, journal, logs) is left in place. There is
; no /S option that also deletes it. The checkbox itself only removes
; %APPDATA%\com.datasafe.desktop and %LOCALAPPDATA%\com.datasafe.desktop.
; Those are not the data directory. This hook also removes
; %LOCALAPPDATA%\com.datasafe.desktop, the WebView2 profile. It holds no
; user data, and it is where saved codes were stored. %APPDATA%\DataSafeS3
; stays. Skip the delete during an update so the new installer can still
; read the previous install directory.

!macro NSIS_HOOK_POSTUNINSTALL
  ${If} $UpdateMode <> 1
    DeleteRegKey HKCU "Software\datasafe\DataSafeS3 Desktop"
    DeleteRegKey /ifempty HKCU "Software\datasafe"
    DeleteRegKey HKLM "Software\datasafe\DataSafeS3 Desktop"
    DeleteRegKey /ifempty HKLM "Software\datasafe"
    RMDir /r "$LOCALAPPDATA\com.datasafe.desktop"
  ${EndIf}
!macroend
