Unicode true

####
## Please note: Template replacements don't work in this file. They are provided with default defines like
## mentioned underneath.
## If the keyword is not defined, "wails_tools.nsh" will populate them.
## If they are defined here, "wails_tools.nsh" will not touch them. This allows you to use this project.nsi manually
## from outside of Wails for debugging and development of the installer.
## 
## For development first make a wails nsis build to populate the "wails_tools.nsh":
## > wails build --target windows/amd64 --nsis
## Then you can call makensis on this file with specifying the path to your binary:
## For a AMD64 only installer:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app.exe
## For a ARM64 only installer:
## > makensis -DARG_WAILS_ARM64_BINARY=..\..\bin\app.exe
## For a installer with both architectures:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app-amd64.exe -DARG_WAILS_ARM64_BINARY=..\..\bin\app-arm64.exe
####
## The following information is taken from the wails_tools.nsh file, but they can be overwritten here.
####
## !define INFO_PROJECTNAME    "my-project" # Default "sonde"
## !define INFO_COMPANYNAME    "The Sonde Authors" # Default "The Sonde Authors"
## !define INFO_PRODUCTNAME    "My Product Name" # Default "Sonde"
## !define INFO_PRODUCTVERSION "1.0.0"     # Default "0.1.0"
## !define INFO_COPYRIGHT      "(c) Now, The Sonde Authors" # Default "© 2026, The Sonde Authors"
###
## !define PRODUCT_EXECUTABLE  "Application.exe"      # Default "${INFO_PROJECTNAME}.exe"
## !define UNINST_KEY_NAME     "UninstKeyInRegistry"  # Default "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}"
####
## !define REQUEST_EXECUTION_LEVEL "admin"            # Default "admin"  see also https://nsis.sourceforge.io/Docs/Chapter4.html
## !define WAILS_INSTALL_SCOPE     "user"             # Default "machine" - set to "user" for per-user install ($LOCALAPPDATA) without UAC prompt
####
## Include the wails tools
####
!include "wails_tools.nsh"

# The version information for this two must consist of 4 parts
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

# Enable HiDPI support. https://nsis.sourceforge.io/Reference/ManifestDPIAware
ManifestDPIAware true

!include "MUI.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
# !define MUI_WELCOMEFINISHPAGE_BITMAP "resources\leftimage.bmp" #Include this to add a bitmap on the left side of the Welcome Page. Must be a size of 164x314
!define MUI_FINISHPAGE_NOAUTOCLOSE # Wait on the INSTFILES page so the user can take a look into the details of the installation steps
!define MUI_ABORTWARNING # This will warn the user if they exit from the installer.

!insertmacro MUI_PAGE_WELCOME # Welcome to the installer page.
# !insertmacro MUI_PAGE_LICENSE "resources\eula.txt" # Adds a EULA page to the installer
!insertmacro MUI_PAGE_DIRECTORY # In which folder install page.
!insertmacro MUI_PAGE_INSTFILES # Installing page.
!insertmacro MUI_PAGE_FINISH # Finished installation page.

!insertmacro MUI_UNPAGE_INSTFILES # Uninstalling page

!insertmacro MUI_LANGUAGE "English" # Set the Language of the installer

## The following two statements can be used to sign the installer and the uninstaller. The path to the binaries are provided in %1
#!uninstfinalize 'signtool --file "%1"'
#!finalize 'signtool --file "%1"'

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe" # Name of the installer's file.
!if "${WAILS_INSTALL_SCOPE}" == "user"
    InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
!else
    InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
!endif
ShowInstDetails show # This will always show the installation details.

# Sonde's silent update mode, started by the app: /S /UPDATE /WAITPID=<pid>.
# It installs into the folder the machine-wide install recorded (never a
# folder from the command line), waits for the app to quit, and refuses to
# touch a Sonde.exe that still runs.
Var Updating

# relaunch starts Sonde again through Explorer: as the user, not with the
# installer's elevation. An update that stops also starts it again, since
# the app quit for it. Exit codes: 2, no install recorded; 3, Sonde.exe
# still running.
!macro relaunch
    Exec '"$WINDIR\explorer.exe" "$INSTDIR\${PRODUCT_EXECUTABLE}"'
!macroend

Function .onInit
   !insertmacro wails.checkArchitecture

   ${GetParameters} $R0
   ClearErrors
   ${GetOptions} $R0 "/UPDATE" $R1
   ${If} ${Errors}
       Return
   ${EndIf}
   StrCpy $Updating 1
   # Never a page: the folder is the recorded one.
   SetSilent silent

   # The installed folder, from the machine-wide key only (64-bit view):
   # InstallLocation, or for a 0.1.0 install the folder of DisplayIcon.
   SetRegView 64
   ReadRegStr $R2 HKLM "${UNINST_KEY}" "InstallLocation"
   ${If} $R2 == ""
       ReadRegStr $R3 HKLM "${UNINST_KEY}" "DisplayIcon"
       # A quoted value: without its quotes.
       StrCpy $R8 $R3 1
       ${If} $R8 == '"'
           StrCpy $R3 $R3 "" 1
           StrCpy $R3 $R3 -1
       ${EndIf}
       ${If} $R3 != ""
           ${GetParent} $R3 $R2
       ${EndIf}
   ${EndIf}
   ${If} $R2 == ""
       SetErrorLevel 2
       Abort
   ${EndIf}
   StrCpy $INSTDIR $R2

   # Wait up to 60 s for the app to quit.
   ClearErrors
   ${GetOptions} $R0 "/WAITPID=" $R4
   ${IfNot} ${Errors}
       # A number, whatever the command line held.
       IntOp $R4 $R4 + 0
       System::Call 'kernel32::OpenProcess(i 0x00100000, i 0, i $R4) p .R5'
       ${If} $R5 P<> 0
           System::Call 'kernel32::WaitForSingleObject(p R5, i 60000) i .R6'
           System::Call 'kernel32::CloseHandle(p R5)'
       ${EndIf}
   ${EndIf}

   # A Sonde.exe that still runs (another window) cannot be written:
   # stop before anything changes.
   ${If} ${FileExists} "$INSTDIR\${PRODUCT_EXECUTABLE}"
       ClearErrors
       FileOpen $R7 "$INSTDIR\${PRODUCT_EXECUTABLE}" a
       ${If} ${Errors}
           SetErrorLevel 3
           !insertmacro relaunch
           Abort
       ${EndIf}
       FileClose $R7
   ${EndIf}
FunctionEnd

# After an update, start Sonde again.
Function .onInstSuccess
   ${If} $Updating == 1
       !insertmacro relaunch
   ${EndIf}
FunctionEnd

Section
    !insertmacro wails.setShellContext

    # The app already ran on this machine: an update never runs the
    # WebView2 bootstrapper elevated.
    ${If} $Updating != 1
        !insertmacro wails.webview2runtime
    ${EndIf}

    SetOutPath $INSTDIR
    
    !insertmacro wails.files

    # An update keeps the shortcuts the user kept (or deleted).
    ${If} $Updating != 1
        CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
        CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    ${EndIf}

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols
    
    !insertmacro wails.writeUninstaller

    # Where it is installed: the app checks it before updating itself in
    # place, and /UPDATE installs there.
    !if "${WAILS_INSTALL_SCOPE}" != "user"
        SetRegView 64
        WriteRegStr HKLM "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
    !endif
SectionEnd

Section "uninstall" 
    !insertmacro wails.setShellContext

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd
