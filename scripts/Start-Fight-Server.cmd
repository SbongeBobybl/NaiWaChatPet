@echo off
cd /d "%~dp0"
title NaiWa Fight Room Server
echo Keep this window open while playing. Press Ctrl+C to stop.
echo Local test: 127.0.0.1:28760
echo LAN players must use this computer's LAN IP, not 127.0.0.1.
echo.
"%~dp0NaiWaFightServer.exe" -listen :28760
echo.
echo Server stopped. If startup failed, check whether port 28760 is already in use.
pause
