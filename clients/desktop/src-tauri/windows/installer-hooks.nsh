; Remove the install-location key the NSIS template writes at
; HKCU\Software\datasafe\DataSafeS3 Desktop (manufacturer is the second
; label of com.datasafe.desktop). The stock uninstaller deletes that key
; only when the "delete app data" checkbox is checked. App data directories
; stay on that checkbox; this hook does not touch them.
; Skip the delete during an update so the new installer can still read the
; previous install directory.

!macro NSIS_HOOK_POSTUNINSTALL
  ${If} $UpdateMode <> 1
    DeleteRegKey HKCU "Software\datasafe\DataSafeS3 Desktop"
    DeleteRegKey /ifempty HKCU "Software\datasafe"
    DeleteRegKey HKLM "Software\datasafe\DataSafeS3 Desktop"
    DeleteRegKey /ifempty HKLM "Software\datasafe"
  ${EndIf}
!macroend
