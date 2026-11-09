; Simplified Chinese for the Windows installer and uninstaller, which
; mygo build includes after its own languages (windows.installerInclude in
; mygo.json). NSIS shows the language of the system, English elsewhere.
!insertmacro MUI_LANGUAGE "SimpChinese"
LangString MYGO_DESKTOP_SHORTCUT ${LANG_SIMPCHINESE} "创建桌面快捷方式"
