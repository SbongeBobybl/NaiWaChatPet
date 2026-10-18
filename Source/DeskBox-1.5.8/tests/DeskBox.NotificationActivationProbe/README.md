# Packaged notification activation probe

The probe links the production notification service, registration guard and
activation envelope store. It uses a unique temporary MSIX identity, the already
installed Windows App Runtime 2 framework, and an existing trusted development
certificate. It does not replace DeskBox or install certificates.

Publish with an x64 Visual Studio developer environment:

```powershell
dotnet publish .\tests\DeskBox.NotificationActivationProbe\DeskBox.NotificationActivationProbe.csproj -c Release -p:Platform=x64 -r win-x64 -p:PublishAot=true -p:IlcUseEnvironmentalTools=true -o D:\temp\notification-probe-publish
```

Run `Run-MSIX.ps1` with `-PublishDirectory`, an existing
`-CertificateThumbprint`, and a fresh `-OutputDirectory`.

The verifier uses the real COM notification activation interface to launch the
packaged process, sends a second callback to that same process, and checks that
both payloads and their selection input survive forwarding to another process.
The temporary package is removed in `finally`. Evidence is retained under the
unique probe directory in Common Documents, outside MSIX file virtualization.

This checks native activation and forwarding, not a human click in Notification
Center, the application's complete Todo UI, or a Microsoft Store flight upgrade.
